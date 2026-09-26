package workspace

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBlobIsBounded covers the two ways a blob could reach memory unbounded:
// a write over the bound, and a stored row whose bytes inflate past the size
// it records, which is what a row merged in from another log could hold.
func TestBlobIsBounded(t *testing.T) {
	ctx := t.Context()
	b := Local(t.TempDir())
	t.Cleanup(func() { _ = b.Close() })

	_, err := b.PutBlob(ctx, make([]byte, MaxBlobSize+1))
	require.ErrorIs(t, err, ErrBlobTooLarge)

	// A row that records 4 bytes and inflates to a megabyte of zeros.
	var packed bytes.Buffer
	zw := gzip.NewWriter(&packed)
	_, err = zw.Write(make([]byte, 1<<20))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	db, err := b.Registry(ctx)
	require.NoError(t, err)
	address := BlobAddress([]byte("tiny"))
	_, err = db.ExecContext(ctx,
		`INSERT INTO workspace_blobs (digest, size, data) VALUES (?, ?, ?)`, address, 4, packed.Bytes())
	require.NoError(t, err)
	_, err = b.Blob(ctx, address)
	require.ErrorContains(t, err, "does not inflate to the 4 bytes")

	// A row that records a size over the bound is refused before its bytes
	// are read.
	huge := BlobAddress([]byte("huge"))
	_, err = db.ExecContext(ctx,
		`INSERT INTO workspace_blobs (digest, size, data) VALUES (?, ?, ?)`, huge, MaxBlobSize+1, packed.Bytes())
	require.NoError(t, err)
	_, err = b.Blob(ctx, huge)
	require.ErrorIs(t, err, ErrBlobTooLarge)
}
