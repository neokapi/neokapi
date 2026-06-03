package epub_test

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeZip builds a ZIP archive from the given name->content entries. Used to
// construct deliberately broken EPUB structures (an EPUB is just a ZIP).
func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// drainForError reads every PartResult from the channel, asserting the drain
// itself never panics, and reports whether any result carried an error.
func drainForError(t *testing.T, ch <-chan model.PartResult) bool {
	t.Helper()
	var foundError bool
	require.NotPanics(t, func() {
		for result := range ch {
			if result.Error != nil {
				foundError = true
			}
		}
	})
	return foundError
}

// TestReadMalformedEPUB feeds structurally broken EPUB containers and asserts
// that Read surfaces a clean error on its result channel rather than panicking.
// Open only spools the bytes to a temp file, so the structural parse errors
// surface during Read (after the root layer-start part has been emitted).
func TestReadMalformedEPUB(t *testing.T) {
	t.Parallel()

	const validContainer = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	tests := []struct {
		name  string
		input []byte
	}{
		{
			// Not a ZIP at all — zip.OpenReader cannot find the central directory.
			name:  "garbage not a zip",
			input: []byte("this is definitely not a zip archive {[<"),
		},
		{
			// A valid ZIP local file header prefix followed by junk, so the
			// archive is truncated and the central directory is missing.
			name:  "truncated zip",
			input: append([]byte{0x50, 0x4B, 0x03, 0x04}, []byte("truncated junk payload")...),
		},
		{
			// A well-formed ZIP, but it lacks the mandatory container manifest.
			name: "missing container.xml",
			input: makeZip(t, map[string]string{
				"mimetype":          "application/epub+zip",
				"OEBPS/content.opf": "<package/>",
			}),
		},
		{
			// container.xml is present but declares no <rootfile>, so the OPF
			// location cannot be resolved.
			name: "container without rootfile",
			input: makeZip(t, map[string]string{
				"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles></rootfiles>
</container>`,
			}),
		},
		{
			// container.xml points at an OPF path that does not exist in the ZIP.
			name: "missing opf file",
			input: makeZip(t, map[string]string{
				"META-INF/container.xml": validContainer,
			}),
		},
		{
			// The OPF file exists but is not parseable XML.
			name: "unparseable opf",
			input: makeZip(t, map[string]string{
				"META-INF/container.xml": validContainer,
				"OEBPS/content.opf":      `<package><manifest><item id="ch1"`,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := epub.NewReader()

			// Open only spools bytes to a temp file; it should not panic and
			// should not yet surface the structural error.
			require.NotPanics(t, func() {
				err := reader.Open(ctx, rawDocFromBytes(tt.input, model.LocaleEnglish))
				require.NoError(t, err)
			})
			defer reader.Close()

			foundError := drainForError(t, reader.Read(ctx))
			assert.True(t, foundError, "expected a clean error for malformed EPUB input")
		})
	}
}

// TestReadSpineItemrefMissingManifestEntry verifies that a spine <itemref>
// whose idref has no matching manifest <item> (and, separately, a manifest item
// whose href points at a missing ZIP entry) is skipped gracefully rather than
// panicking. EPUB readers are deliberately lenient here: a dangling spine
// reference yields no block, but the document still reads cleanly to completion.
func TestReadSpineItemrefMissingManifestEntry(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	const container = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	// Spine references "ghost" (no manifest item) and "broken" (manifest item
	// whose href targets a non-existent ZIP entry). Neither must cause a panic.
	const opf = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="broken" href="does-not-exist.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ghost"/>
    <itemref idref="broken"/>
  </spine>
</package>`

	data := makeZip(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": container,
		"OEBPS/content.opf":      opf,
	})

	reader := epub.NewReader()
	require.NotPanics(t, func() {
		err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
		require.NoError(t, err)
	})
	defer reader.Close()

	// The dangling references are skipped; the read completes without error or
	// panic.
	foundError := drainForError(t, reader.Read(ctx))
	assert.False(t, foundError, "dangling spine references should be skipped, not error")
}

// TestReadNilReader verifies Open rejects a document with a nil reader without
// panicking (the nil-document case is covered by TestReadNilDocument in
// reader_test.go).
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := epub.NewReader()
	err := reader.Open(ctx, &model.RawDocument{
		URI:          "test://book.epub",
		SourceLocale: model.LocaleEnglish,
		Reader:       nil,
	})
	require.Error(t, err)
}
