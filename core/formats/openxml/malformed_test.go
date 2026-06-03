package openxml

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAllOpenXML drains the reader's result channel for the given raw bytes,
// returning whether any clean PartResult.Error surfaced and the number of
// translatable blocks emitted. OpenXML is a binary ZIP package, so — unlike the
// JSON malformed test — we feed raw bytes through a bytes.Reader rather than
// RawDocFromString. The Open+Read calls are wrapped in require.NotPanics so a
// panic fails the test with a clear message instead of crashing the run.
//
// Run with -race to surface any data race in the reader goroutine that drives
// the channel.
func readAllOpenXML(t *testing.T, raw []byte) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader; ZIP and part-parse
		// errors surface later, during Read.
		err := reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(raw), "test://input.docx", model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
			if result.Part != nil && result.Part.Type == model.PartBlock {
				blocks++
			}
		}
	})
	return foundError, blocks
}

// zipBytes builds an in-memory ZIP archive from the given name→content entries,
// preserving order. It returns the raw bytes so callers can hand them to the
// reader the same way a real .docx would arrive on a stream.
func zipBytes(t *testing.T, entries [][2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		writeZipEntry(t, zw, e[0], e[1])
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

const validContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const validRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

// TestReadMalformedSurfacesError feeds inputs the reader genuinely cannot
// process and asserts the failure surfaces cleanly on the result channel
// (PartResult.Error) rather than panicking or hanging. These exercise the
// reader's guarded paths: the io.ReadAll/zip.NewReader/parseContainer error
// returns and the per-part XML parser error returns in readContent.
//
// Run with -race to catch any data race in the reader goroutine.
func TestReadMalformedSurfacesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			// Non-ZIP garbage: the first bytes are not the PK signature, so
			// zip.NewReader rejects the archive ("not a valid ZIP archive").
			name: "non-zip garbage bytes",
			raw:  []byte("@@@ this is not a zip file %%% ^^^ \x00\x01\x02\x03"),
		},
		{
			// Truncated ZIP: a real .docx prefix cut off mid-stream. The PK
			// local-file header is present but the central directory the ZIP
			// format places at the tail is gone, so zip.NewReader fails.
			name: "truncated zip",
			raw:  truncatedZip(t),
		},
		{
			// Valid ZIP but no [Content_Types].xml: parseContentTypes returns
			// "missing [Content_Types].xml", wrapped by parseContainer.
			name: "zip missing content types",
			raw: zipBytes(t, [][2]string{
				{"_rels/.rels", validRootRels},
				{"word/document.xml", `<w:document/>`},
			}),
		},
		{
			// Valid ZIP, valid [Content_Types].xml, but [Content_Types].xml
			// declares no recognizable main part, so detectDocType returns
			// unknown → "unable to determine document type".
			name: "zip unknown document type",
			raw: zipBytes(t, [][2]string{
				{"[Content_Types].xml", `<?xml version="1.0"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="xml" ContentType="application/xml"/>
</Types>`},
				{"_rels/.rels", validRootRels},
			}),
		},
		{
			// Valid DOCX shell whose word/document.xml is broken XML (an
			// unclosed element). The wmlParser's xml.Decoder reports a syntax
			// error, surfaced as "wml: parsing word/document.xml: ...".
			name: "broken document xml",
			raw: zipBytes(t, [][2]string{
				{"[Content_Types].xml", validContentTypes},
				{"_rels/.rels", validRootRels},
				{"word/document.xml", `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>unterminated`},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, _ := readAllOpenXML(t, tt.raw)
			assert.True(t, foundError, "expected a clean PartResult.Error for malformed OpenXML input")
		})
	}
}

// TestReadGracefulInputsDoNotPanic feeds inputs the reader tolerates rather
// than rejects: it must neither panic nor hang, and whether it surfaces an
// error or simply emits no translatable blocks is an implementation detail we
// do not over-assert. The single contract is robustness.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadGracefulInputsDoNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			// Empty input: io.ReadAll yields zero bytes, zip.NewReader on an
			// empty archive fails cleanly with an error.
			name: "empty input",
			raw:  []byte{},
		},
		{
			// A bare PK signature with nothing after it.
			name: "bare pk signature",
			raw:  []byte{0x50, 0x4B},
		},
		{
			// A valid, complete DOCX whose declared main part is absent from
			// the archive. readContent skips missing parts (zf == nil →
			// continue), so this parses to zero translatable blocks without an
			// error — it must not panic on the missing word/document.xml.
			name: "valid shell missing document part",
			raw: zipBytes(t, [][2]string{
				{"[Content_Types].xml", validContentTypes},
				{"_rels/.rels", validRootRels},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				readAllOpenXML(t, tt.raw)
			})
		})
	}
}

// truncatedZip builds a valid one-entry ZIP, then returns only its leading
// portion — enough to retain the local-file header but drop the central
// directory the ZIP reader needs, simulating a download cut off mid-stream.
func truncatedZip(t *testing.T) []byte {
	t.Helper()
	full := zipBytes(t, [][2]string{
		{"[Content_Types].xml", validContentTypes},
		{"_rels/.rels", validRootRels},
		{"word/document.xml", `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>Hello World</w:t></w:r></w:p></w:body>
</w:document>`},
	})
	require.Greater(t, len(full), 32, "expected a non-trivial ZIP to truncate")
	// Keep the first half: the PK\x03\x04 local header survives, the trailing
	// central directory / end-of-central-directory record does not.
	return full[:len(full)/2]
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
