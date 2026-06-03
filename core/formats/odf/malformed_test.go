package odf_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainMalformed opens the ODF reader on raw bytes and drains its result
// channel, reporting whether any clean PartResult.Error surfaced and how many
// translatable blocks were emitted. ODF is a binary ZIP package, so unlike the
// JSON malformed helper this feeds raw bytes via a bytes.Reader (matching the
// reader_test.go convention) rather than RawDocFromString.
//
// Both Open and the channel drain run inside require.NotPanics so a panic in
// the reader goroutine fails the test with a clear message instead of crashing
// the run. Run under -race to surface any data race in that goroutine.
func drainMalformed(t *testing.T, data []byte) (foundError bool, blocks int) {
	t.Helper()
	ctx := t.Context()
	reader := odf.NewReader()

	require.NotPanics(t, func() {
		// Open only validates the document/reader is non-nil; the ZIP/XML
		// parse errors surface later, during Read.
		err := reader.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish))
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

// makeBareZip builds a valid ZIP archive from the given name->contents entries
// without any of the ODF scaffolding makeODFZip adds. Used to construct ZIPs
// that are structurally valid but malformed as ODF packages (missing
// content.xml, broken content.xml, ...).
func makeBareZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, contents := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// TestReadMalformedZipSurfacesError feeds inputs that cannot be opened as a ZIP
// archive at all and asserts the error surfaces cleanly on the result channel
// (PartResult.Error) — never a panic or a hang. zip.NewReader is the gate here:
// it rejects non-ZIP and truncated/empty input before any ODF parsing begins.
//
// Run with -race to catch any data race in the reader goroutine.
func TestReadMalformedZipSurfacesError(t *testing.T) {
	t.Parallel()

	// A real, valid ODF ZIP whose tail we truncate to produce a corrupt
	// archive (the central directory at the end of the file is lost).
	validODF := makeODFZip(mimeODT, simpleODTContent("Hello"))
	truncated := validODF[:len(validODF)/2]

	tests := []struct {
		name string
		data []byte
	}{
		{
			// Pure garbage bytes: no PK ZIP local-file header, so
			// zip.NewReader fails immediately.
			name: "non-zip garbage",
			data: []byte("@@@ this is not a zip %%% ^^^ \x00\x01\x02"),
		},
		{
			// Truncated ZIP: a genuine ODF archive cut in half, so the
			// central directory record is missing.
			name: "truncated zip",
			data: truncated,
		},
		{
			// Fake PK magic followed by garbage: starts with the ZIP
			// signature but is not a valid archive.
			name: "fake pk magic then garbage",
			data: append([]byte{0x50, 0x4B, 0x03, 0x04}, []byte("garbage not a real zip entry")...),
		},
		{
			// Empty input: zero-length archive, no central directory.
			name: "empty input",
			data: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			foundError, blocks := drainMalformed(t, tt.data)
			assert.True(t, foundError, "expected a clean error for un-openable ZIP input")
			assert.Zero(t, blocks, "no blocks should be emitted when the ZIP cannot be opened")
		})
	}
}

// TestReadValidZipMissingContentXMLIsGracefulEmpty feeds a structurally valid
// ZIP that lacks content.xml (and meta.xml / styles.xml). The reader skips the
// absent entries and emits only the root layer — no error, no panic, no blocks.
// This proves robustness against an empty/incomplete package rather than
// treating a missing entry as a crash.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadValidZipMissingContentXMLIsGracefulEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
	}{
		{
			// A valid ZIP with only an unrelated entry — none of the ODF
			// XML parts (content/meta/styles) are present.
			name: "only unrelated entry",
			data: makeBareZip(t, map[string]string{"README.txt": "hello"}),
		},
		{
			// A valid ZIP with the ODF mimetype marker but no content.xml.
			name: "mimetype but no content.xml",
			data: makeBareZip(t, map[string]string{"mimetype": mimeODT}),
		},
		{
			// A completely empty (but valid) ZIP archive.
			name: "empty zip archive",
			data: makeBareZip(t, map[string]string{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var foundError bool
			var blocks int
			require.NotPanics(t, func() {
				foundError, blocks = drainMalformed(t, tt.data)
			})
			assert.False(t, foundError, "a valid ZIP missing content.xml should not error")
			assert.Zero(t, blocks, "no translatable blocks without content.xml")
		})
	}
}

// TestReadBrokenContentXMLDoesNotPanic feeds valid ZIPs whose content.xml /
// styles.xml is broken XML. The ODF parser drives an xml.Decoder token loop
// that stops cleanly at the first decode error (reader.go's parseODFContent
// breaks out of the loop on err), so malformed XML must not panic or hang —
// whether it yields partial blocks or none is an implementation detail we do
// not over-assert. The single contract is robustness: no panic, no hang.
//
// Run with -race to surface any data race in the reader goroutine.
func TestReadBrokenContentXMLDoesNotPanic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
	}{
		{
			// Unclosed root element: the decoder reaches EOF mid-document.
			name:    "unclosed root element",
			content: `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"><office:body><office:text><text:p`,
		},
		{
			// Garbage that is not XML at all.
			name:    "not xml at all",
			content: "@@@ %%% not xml ^^^",
		},
		{
			// Mismatched / unbalanced tags.
			name:    "mismatched tags",
			content: `<a xmlns="x"><b></c></a>`,
		},
		{
			// Truncated mid declaration.
			name:    "truncated declaration",
			content: `<?xml version="1.0"`,
		},
		{
			// Empty content.xml.
			name:    "empty content",
			content: "",
		},
		{
			// Well-formed XML envelope wrapping a broken inline span inside
			// a translatable paragraph, exercising the inline-code path.
			name: "broken inline inside paragraph",
			content: `<?xml version="1.0"?><office:document-content ` +
				`xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
				`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">` +
				`<office:body><office:text><text:p>Hello <text:span>world`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := makeODFZip(mimeODT, tt.content)
			require.NotPanics(t, func() {
				drainMalformed(t, data)
			})
		})
	}
}

// TestReadNilDocumentAndReader verifies Open rejects a nil document and a
// document with a nil Reader without panicking — the cheap guard in Open
// (reader.go: "nil document or reader") must fire before Read is ever reached.
func TestReadNilDocumentAndReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	require.NotPanics(t, func() {
		reader := odf.NewReader()
		err := reader.Open(ctx, nil)
		require.Error(t, err)
	})

	require.NotPanics(t, func() {
		reader := odf.NewReader()
		err := reader.Open(ctx, &model.RawDocument{SourceLocale: model.LocaleEnglish})
		require.Error(t, err)
	})
}
