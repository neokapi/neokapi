package odf_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test ODF file builders ---

// makeODFZip creates an ODF ZIP archive with the given mimetype and content.xml.
func makeODFZip(mimetype, contentXML string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// mimetype must be first, uncompressed
	fh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	w, _ := zw.CreateHeader(fh)
	_, _ = w.Write([]byte(mimetype))

	// content.xml
	w, _ = zw.Create("content.xml")
	_, _ = w.Write([]byte(contentXML))

	// META-INF/manifest.xml (minimal)
	w, _ = zw.Create("META-INF/manifest.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">
  <manifest:file-entry manifest:full-path="/" manifest:media-type="` + mimetype + `"/>
  <manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>
</manifest:manifest>`))

	zw.Close()
	return buf.Bytes()
}

// makeODFZipWithStyles creates an ODF ZIP with content.xml and styles.xml.
func makeODFZipWithStyles(mimetype, contentXML, stylesXML string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	w, _ := zw.CreateHeader(fh)
	_, _ = w.Write([]byte(mimetype))

	w, _ = zw.Create("content.xml")
	_, _ = w.Write([]byte(contentXML))

	w, _ = zw.Create("styles.xml")
	_, _ = w.Write([]byte(stylesXML))

	w, _ = zw.Create("META-INF/manifest.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0">
  <manifest:file-entry manifest:full-path="/" manifest:media-type="` + mimetype + `"/>
</manifest:manifest>`))

	zw.Close()
	return buf.Bytes()
}

// --- Mime type constants ---
const (
	mimeODT = "application/vnd.oasis.opendocument.text"
	mimeODS = "application/vnd.oasis.opendocument.spreadsheet"
	mimeODP = "application/vnd.oasis.opendocument.presentation"
)

// --- Simple ODT content XML templates ---

func simpleODTContent(paragraphs ...string) string {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0">
<office:body><office:text>`)
	for _, p := range paragraphs {
		body.WriteString(`<text:p>` + p + `</text:p>`)
	}
	body.WriteString(`</office:text></office:body></office:document-content>`)
	return body.String()
}

func simpleODSContent(cells [][]string) string {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0">
<office:body><office:spreadsheet>
<table:table table:name="Sheet1">`)
	for _, row := range cells {
		body.WriteString(`<table:table-row>`)
		for _, cell := range row {
			body.WriteString(`<table:table-cell><text:p>` + cell + `</text:p></table:table-cell>`)
		}
		body.WriteString(`</table:table-row>`)
	}
	body.WriteString(`</table:table></office:spreadsheet></office:body></office:document-content>`)
	return body.String()
}

func simpleODPContent(slides []string) string {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0"
  xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0">
<office:body><office:presentation>`)
	for _, slide := range slides {
		body.WriteString(`<draw:page><draw:frame><draw:text-box><text:p>` + slide + `</text:p></draw:text-box></draw:frame></draw:page>`)
	}
	body.WriteString(`</office:presentation></office:body></office:document-content>`)
	return body.String()
}

// --- Helper functions ---

func openReader(t *testing.T, data []byte) *odf.Reader {
	t.Helper()
	ctx := t.Context()
	reader := odf.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	return reader
}

func readParts(t *testing.T, data []byte) []*model.Part {
	t.Helper()
	reader := openReader(t, data)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(t.Context()))
}

// --- Tests ---

func TestReaderMetadata(t *testing.T) {
	reader := odf.NewReader()
	assert.Equal(t, "odf", reader.Name())
	assert.Equal(t, "Open Document Format", reader.DisplayName())
}

// okapi: OpenOfficeFilterTest#testDefaultInfo — verifies ODF MIME types and file extensions.
func TestReaderSignature(t *testing.T) {
	reader := odf.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/vnd.oasis.opendocument.text")
	assert.Contains(t, sig.MIMETypes, "application/vnd.oasis.opendocument.spreadsheet")
	assert.Contains(t, sig.MIMETypes, "application/vnd.oasis.opendocument.presentation")
	assert.Contains(t, sig.Extensions, ".odt")
	assert.Contains(t, sig.Extensions, ".ods")
	assert.Contains(t, sig.Extensions, ".odp")
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := odf.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// okapi: OpenOfficeFilterTest#testFirstTextUnit — extracts paragraphs from ODT content.
func TestReadSimpleODT(t *testing.T) {
	data := makeODFZip(mimeODT, simpleODTContent("Hello, World!", "Second paragraph"))
	parts := readParts(t, data)

	// Should have layer start, blocks, layer end
	require.True(t, len(parts) >= 3, "expected at least 3 parts, got %d", len(parts))

	// First part should be root layer start
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	rootLayer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "odf", rootLayer.Format)
	assert.Equal(t, "doc1", rootLayer.ID)

	// Last part should be root layer end
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// Extract blocks
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello, World!", blocks[0].SourceText())
	assert.Equal(t, "Second paragraph", blocks[1].SourceText())
}

func TestReadODTBlockProperties(t *testing.T) {
	data := makeODFZip(mimeODT, simpleODTContent("Test paragraph"))
	parts := readParts(t, data)

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)

	block := blocks[0]
	assert.Equal(t, "content.xml", block.Properties["partPath"])
	assert.Equal(t, "p", block.Properties["element"])
	assert.True(t, block.Translatable)
}

func TestReadODTHeadings(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text>
<text:h text:outline-level="1">Chapter One</text:h>
<text:p>Body text here.</text:p>
</office:text></office:body></office:document-content>`

	data := makeODFZip(mimeODT, content)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)
	assert.Equal(t, "Chapter One", blocks[0].SourceText())
	assert.Equal(t, "h", blocks[0].Properties["element"])
	assert.Equal(t, "Body text here.", blocks[1].SourceText())
}

// okapi: OpenOfficeFilterTest#testFormulaResultExtraction — extracts spreadsheet cell content.
func TestReadODSSpreadsheet(t *testing.T) {
	cells := [][]string{
		{"Name", "Value"},
		{"Item A", "100"},
		{"Item B", "200"},
	}
	data := makeODFZip(mimeODS, simpleODSContent(cells))
	parts := readParts(t, data)

	// Check doc type in layer properties
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	rootLayer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "ods", rootLayer.Properties["docType"])

	blocks := testutil.FilterBlocks(parts)
	// 6 cell text:p values + 1 translatable table:name="Sheet1" attribute
	// (the simpleODSContent helper wraps cells in <table:table table:name="Sheet1">).
	require.Len(t, blocks, 7)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Sheet1")
	assert.Contains(t, texts, "Name")
	assert.Contains(t, texts, "Value")
	assert.Contains(t, texts, "Item A")
	assert.Contains(t, texts, "100")
}

func TestReadODPPresentation(t *testing.T) {
	slides := []string{"Title Slide", "Content Slide"}
	data := makeODFZip(mimeODP, simpleODPContent(slides))
	parts := readParts(t, data)

	rootLayer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "odp", rootLayer.Properties["docType"])

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Title Slide", blocks[0].SourceText())
	assert.Equal(t, "Content Slide", blocks[1].SourceText())
}

// okapi: ODFFilterTest#testFirstTextUnit — inline formatting produces spans within blocks.
func TestReadInlineFormatting(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text>
<text:p>Normal <text:span text:style-name="Bold">bold</text:span> text</text:p>
</office:text></office:body></office:document-content>`

	data := makeODFZip(mimeODT, content)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)

	runs := blocks[0].SourceRuns()
	hasInline := false
	for _, r := range runs {
		if r.Text == nil {
			hasInline = true
			break
		}
	}
	assert.True(t, hasInline, "inline formatting should produce inline-code runs")
	assert.Equal(t, "Normal bold text", model.RunsPlainText(runs))
}

func TestReadHyperlink(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:xlink="http://www.w3.org/1999/xlink">
<office:body><office:text>
<text:p>Click <text:a xlink:href="https://example.com">here</text:a> for more</text:p>
</office:text></office:body></office:document-content>`

	data := makeODFZip(mimeODT, content)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	runs := blocks[0].SourceRuns()
	hasInline := false
	for _, r := range runs {
		if r.Text == nil {
			hasInline = true
			break
		}
	}
	assert.True(t, hasInline, "hyperlink should produce inline-code runs")
	assert.Equal(t, "Click here for more", model.RunsPlainText(runs))
}

func TestReadEmptyParagraphs(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text>
<text:p></text:p>
<text:p>Non-empty</text:p>
<text:p>   </text:p>
</office:text></office:body></office:document-content>`

	data := makeODFZip(mimeODT, content)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	// Empty and whitespace-only paragraphs should be skipped
	require.Len(t, blocks, 1)
	assert.Equal(t, "Non-empty", blocks[0].SourceText())
}

func TestReadWithStylesXML(t *testing.T) {
	contentXML := simpleODTContent("Content text")
	stylesXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:master-styles>
<text:p>Header text</text:p>
</office:master-styles>
</office:document-styles>`

	data := makeODFZipWithStyles(mimeODT, contentXML, stylesXML)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Content text")
	assert.Contains(t, texts, "Header text")
}

func TestReadInvalidZip(t *testing.T) {
	ctx := t.Context()
	reader := odf.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader([]byte("not a zip file")), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err) // Open succeeds, error comes from Read
	defer reader.Close()

	var readErr error
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			readErr = result.Error
			break
		}
	}
	require.Error(t, readErr)
	assert.Contains(t, readErr.Error(), "not a valid ZIP archive")
}

// okapi: OpenOfficeFilterTest#testDoubleExtraction — roundtrip read/write/re-read preserves ODF content.
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()
	originalContent := simpleODTContent("Hello, World!", "Second paragraph")
	data := makeODFZip(mimeODT, originalContent)

	// Read
	reader := odf.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := odf.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalContent(data)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read and verify blocks match
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, 2)
	assert.Equal(t, "Hello, World!", blocks2[0].SourceText())
	assert.Equal(t, "Second paragraph", blocks2[1].SourceText())
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Hello", "World"))

	// Read
	reader := odf.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set target translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			switch block.SourceText() {
			case "Hello":
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			case "World":
				block.SetTargetText(model.LocaleFrench, "Monde")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := odf.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)
	writer.SetOriginalContent(data)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read and verify translations applied
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, 2)
	assert.Equal(t, "Bonjour", blocks2[0].SourceText())
	assert.Equal(t, "Monde", blocks2[1].SourceText())
}

func TestConfig(t *testing.T) {
	cfg := &odf.Config{}
	assert.Equal(t, "odf", cfg.FormatName())

	cfg.Reset()
	assert.True(t, cfg.TranslateNotes)
	assert.False(t, cfg.TranslateHiddenContent)

	err := cfg.Validate()
	require.NoError(t, err)

	err = cfg.ApplyMap(map[string]any{
		"translateNotes": false,
	})
	require.NoError(t, err)
	assert.False(t, cfg.TranslateNotes)

	err = cfg.ApplyMap(map[string]any{
		"unknownKey": true,
	})
	require.Error(t, err)
}

func TestReaderConfig(t *testing.T) {
	reader := odf.NewReader()
	cfg := reader.Config()
	assert.NotNil(t, cfg)
	assert.Equal(t, "odf", cfg.FormatName())
}

func TestWriterNoOriginalContent(t *testing.T) {
	ctx := t.Context()
	writer := odf.NewWriter()
	var buf bytes.Buffer
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	close(ch)
	err = writer.Write(ctx, ch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires original content")
}

func TestReadODTWithLineBreak(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text>
<text:p>Line one<text:line-break/>Line two</text:p>
</office:text></office:body></office:document-content>`

	data := makeODFZip(mimeODT, content)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	// Upstream Okapi emits text:line-break as a PLACEHOLDER inline code
	// carrying the original <text:line-break/> markup, NOT as a literal
	// '\n' in the translatable text (ODFFilter.java:619-622). SourceText
	// returns only TextRun content; the line-break placeholder shows up
	// in SourceRuns as a Ph run carrying the original markup so the
	// writer can splice it back into the reconstructed XML.
	runs := blocks[0].SourceRuns()
	require.Len(t, runs, 3)
	require.NotNil(t, runs[0].Text)
	assert.Equal(t, "Line one", runs[0].Text.Text)
	require.NotNil(t, runs[1].Ph)
	assert.Equal(t, "lb", runs[1].Ph.Type)
	assert.Equal(t, "<text:line-break/>", runs[1].Ph.Data)
	require.NotNil(t, runs[2].Text)
	assert.Equal(t, "Line two", runs[2].Text.Text)
	assert.Equal(t, "Line oneLine two", blocks[0].SourceText())
}

func TestReadODTWithTab(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text>
<text:p>Before<text:tab/>After</text:p>
</office:text></office:body></office:document-content>`

	data := makeODFZip(mimeODT, content)
	parts := readParts(t, data)
	blocks := testutil.FilterBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Before\tAfter", blocks[0].SourceText())
}

func TestWriterName(t *testing.T) {
	writer := odf.NewWriter()
	assert.Equal(t, "odf", writer.Name())
}

// --- Skeleton Store Tests ---

func TestSkeletonRoundTrip(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Hello, World!", "Second paragraph"))

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := odf.NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err = reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks1 := testutil.FilterBlocks(parts)
	require.Len(t, blocks1, 2)

	// Write with skeleton store (no translation — source roundtrip)
	var buf bytes.Buffer
	writer := odf.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	require.True(t, buf.Len() > 0, "output should not be empty")

	// Re-read and compare blocks
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, len(blocks1))
	for i, b := range blocks1 {
		assert.Equal(t, b.SourceText(), blocks2[i].SourceText(),
			"block %d text mismatch", i)
	}
}

func TestSkeletonRoundTripWithTranslation(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Hello", "World"))

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := odf.NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err = reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translations on all blocks
	frFR := model.LocaleID("fr-FR")
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				b.SetTargetText(frFR, "FR: "+b.SourceText())
			}
		}
	}

	// Write with locale
	var buf bytes.Buffer
	writer := odf.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(frFR)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	require.True(t, buf.Len() > 0, "output should not be empty")

	// Re-read and verify translations appear
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, 2)
	assert.Equal(t, "FR: Hello", blocks2[0].SourceText())
	assert.Equal(t, "FR: World", blocks2[1].SourceText())
}

func TestSkeletonRoundTripWithStyles(t *testing.T) {
	ctx := t.Context()
	contentXML := simpleODTContent("Content text")
	stylesXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:master-styles>
<text:p>Header text</text:p>
</office:master-styles>
</office:document-styles>`
	data := makeODFZipWithStyles(mimeODT, contentXML, stylesXML)

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := odf.NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err = reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks1 := testutil.FilterBlocks(parts)
	require.Len(t, blocks1, 2)

	// Write with skeleton store
	var buf bytes.Buffer
	writer := odf.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read and compare
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, 2)
	texts := testutil.BlockTexts(blocks2)
	assert.Contains(t, texts, "Content text")
	assert.Contains(t, texts, "Header text")
}

func TestSkeletonRoundTripODS(t *testing.T) {
	ctx := t.Context()
	cells := [][]string{
		{"Name", "Value"},
		{"Item A", "100"},
	}
	data := makeODFZip(mimeODS, simpleODSContent(cells))

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := odf.NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.ods", model.LocaleEnglish)
	err = reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks1 := testutil.FilterBlocks(parts)

	// Write with skeleton store
	var buf bytes.Buffer
	writer := odf.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read and compare
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.ods", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, len(blocks1))
	for i, b := range blocks1 {
		assert.Equal(t, b.SourceText(), blocks2[i].SourceText(),
			"block %d text mismatch", i)
	}
}

func TestSkeletonRoundTripEmptyParagraphs(t *testing.T) {
	ctx := t.Context()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:body><office:text>
<text:p></text:p>
<text:p>Non-empty</text:p>
<text:p>   </text:p>
</office:text></office:body></office:document-content>`
	data := makeODFZip(mimeODT, content)

	// Read with skeleton store
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := odf.NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err = reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	blocks1 := testutil.FilterBlocks(parts)
	require.Len(t, blocks1, 1)
	assert.Equal(t, "Non-empty", blocks1[0].SourceText())

	// Write with skeleton store
	var buf bytes.Buffer
	writer := odf.NewWriter()
	writer.SetOriginalContent(data)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read and verify
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)

	blocks2 := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks2, 1)
	assert.Equal(t, "Non-empty", blocks2[0].SourceText())
}

func TestTempFileCleanup(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Test"))

	reader := odf.NewReader()
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	_ = testutil.CollectParts(t, reader.Read(ctx))

	// Close should clean up temp file without error
	err = reader.Close()
	require.NoError(t, err)
}
