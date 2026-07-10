package brandscan

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const docxRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

// makeDocx builds a minimal in-memory .docx with one paragraph per argument.
// Paragraph text must be XML-safe (no markup characters).
func makeDocx(t *testing.T, paragraphs ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writePart := func(name, content string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	writePart("[Content_Types].xml", docxContentTypes)
	writePart("_rels/.rels", docxRels)
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString("<w:p><w:r><w:t>")
		body.WriteString(p)
		body.WriteString("</w:t></w:r></w:p>")
	}
	writePart("word/document.xml",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
			`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">`+
			`<w:body>`+body.String()+`</w:body></w:document>`)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestExtractFileHappyPaths(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		filename    string
		contentType string
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "markdown",
			data:        []byte("# Acme\n\nWe build **rockets** for everyone.\n"),
			filename:    "brand.md",
			wantContain: []string{"Acme", "We build rockets for everyone."},
		},
		{
			name: "html strips script and style",
			data: []byte(`<html><head><title>Acme</title>` +
				`<script>var secret = "SCRIPT_SECRET";</script>` +
				`<style>.x{color:red}</style></head>` +
				`<body><p>Bold, direct, human.</p></body></html>`),
			filename:    "brand.html",
			wantContain: []string{"Acme", "Bold, direct, human."},
			wantAbsent:  []string{"SCRIPT_SECRET", "color:red"},
		},
		{
			name:        "plain text",
			data:        []byte("Acme voice: bold and direct.\nAlways lowercase product names.\n"),
			filename:    "voice.txt",
			wantContain: []string{"Acme voice: bold and direct."},
		},
		{
			name:        "csv",
			data:        []byte("term,definition\nrocket,A launch vehicle built by Acme\n"),
			filename:    "glossary.csv",
			wantContain: []string{"A launch vehicle built by Acme"},
		},
		{
			name:        "content type fallback without extension",
			data:        []byte(`<html><body><p>Fallback via content type.</p></body></html>`),
			filename:    "brandpage",
			contentType: "text/html; charset=utf-8",
			wantContain: []string{"Fallback via content type."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := ExtractFile(tt.data, tt.filename, tt.contentType)
			require.NoError(t, err)
			for _, want := range tt.wantContain {
				assert.Contains(t, text, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, text, absent)
			}
		})
	}
}

func TestExtractFileDocx(t *testing.T) {
	data := makeDocx(t, "Hello from Acme.", "We make orbital delivery boringly reliable.")
	text, err := ExtractFile(data, "brand.docx", "")
	require.NoError(t, err)
	assert.Contains(t, text, "Hello from Acme.")
	assert.Contains(t, text, "We make orbital delivery boringly reliable.")
}

func TestExtractFileRejections(t *testing.T) {
	oversize := make([]byte, MaxFileBytes+1)
	tests := []struct {
		name        string
		data        []byte
		filename    string
		contentType string
		wantErr     string
	}{
		{"oversize file", oversize, "big.txt", "", "too large"},
		{"unknown extension", []byte("MZ"), "installer.exe", "", "unsupported file type"},
		{"unknown content type without extension", []byte("data"), "blob", "application/octet-stream", "unsupported file type"},
		{"pdf deferred", []byte("%PDF-1.7"), "brand.pdf", "", "deferred"},
		{"pptx deferred", []byte("PK"), "deck.pptx", "", "deferred"},
		{"pdf deferred by content type", []byte("%PDF-1.7"), "brand", "application/pdf", "deferred"},
		{"pptx deferred by content type", []byte("PK"), "deck",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation", "deferred"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := ExtractFile(tt.data, tt.filename, tt.contentType)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, text)
		})
	}
}

func TestExtractFileDeferredMessageIsExplicit(t *testing.T) {
	_, err := ExtractFile([]byte("%PDF-1.7"), "brand.pdf", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported (deferred: needs pdf/pptx text extractor)")
}
