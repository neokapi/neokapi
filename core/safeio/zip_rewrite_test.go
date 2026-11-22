package safeio_test

import (
	"archive/zip"
	"bytes"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rewriteArchive copies every entry of src through RewriteHeader, the way a
// format writer rewrites the entries it changed.
func rewriteArchive(t *testing.T, src []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(src), int64(len(src)))
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		data, err := safeio.DefaultZipLimits.ReadEntry(f)
		require.NoError(t, err)
		w, err := zw.CreateHeader(safeio.RewriteHeader(&f.FileHeader))
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// TestRewriteHeaderIsAFixedPoint checks that rewriting an archive written the
// same way changes nothing. A copied header keeps the extended-timestamp field
// CreateHeader added, and CreateHeader adds another after it, so a plain copy
// grows each entry by one field on every rewrite.
func TestRewriteHeaderIsAFixedPoint(t *testing.T) {
	t.Parallel()
	modified := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	// An unrelated extra field, which must survive.
	other := []byte{0x0a, 0x00, 0x02, 0x00, 0xab, 0xcd}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "word/document.xml",
		Method:   zip.Deflate,
		Modified: modified,
		Extra:    other,
	})
	require.NoError(t, err)
	_, err = w.Write([]byte("<w:document/>"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	once := rewriteArchive(t, buf.Bytes())
	twice := rewriteArchive(t, once)
	assert.Equal(t, once, twice)

	zr, err := zip.NewReader(bytes.NewReader(twice), int64(len(twice)))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.True(t, bytes.HasPrefix(zr.File[0].Extra, other), "the unrelated extra field is kept")
	assert.True(t, zr.File[0].Modified.Equal(modified), "the modification time is kept")
}
