package epub_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/epub"
)

// TestWrittenMimetypeIsFirstStoredWithNoExtraField reads the raw local file
// header at the start of a written EPUB, on both the skeleton and the
// original-reparse write paths. OCF requires the mimetype entry to come first,
// uncompressed, with no extra field; epubcheck reports an extra field there as
// PKG-005.
func TestWrittenMimetypeIsFirstStoredWithNoExtraField(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "minimal.epub"))
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		skeleton bool
	}{
		{name: "skeleton", skeleton: true},
		{name: "original", skeleton: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, writer := epub.NewReader(), epub.NewWriter()
			if tc.skeleton {
				store, err := format.NewWiredSkeleton(reader, writer)
				require.NoError(t, err)
				if store != nil {
					defer store.Close()
				}
			}
			parts, err := spec.ReadParts(reader, src)
			require.NoError(t, err)
			out, err := spec.WriteParts(writer, parts, src)
			require.NoError(t, err)

			require.GreaterOrEqual(t, len(out), 30)
			require.Equal(t, []byte("PK\x03\x04"), out[:4], "a local file header opens the archive")
			method := binary.LittleEndian.Uint16(out[8:10])
			nameLen := binary.LittleEndian.Uint16(out[26:28])
			extraLen := binary.LittleEndian.Uint16(out[28:30])
			require.GreaterOrEqual(t, len(out), 30+int(nameLen))
			assert.Equal(t, "mimetype", string(out[30:30+int(nameLen)]), "mimetype is the first entry")
			assert.Zero(t, method, "mimetype is stored, not compressed")
			assert.Zero(t, extraLen, "mimetype carries no extra field")
			body := out[30+int(nameLen)+int(extraLen):]
			assert.Equal(t, "application/epub+zip", string(body[:len("application/epub+zip")]))
		})
	}
}
