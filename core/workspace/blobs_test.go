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

func TestBlobRefsReadsTheBlobAndTheBlobsAPayloadNames(t *testing.T) {
	a, b, c := BlobAddress([]byte("a")), BlobAddress([]byte("b")), BlobAddress([]byte("c"))
	tests := []struct {
		name    string
		payload string
		want    []string
	}{
		{name: "no payload", payload: ``, want: nil},
		{name: "no blob", payload: `{"steps":[]}`, want: nil},
		{name: "one blob", payload: `{"blob":"` + a + `"}`, want: []string{a}},
		{name: "the blobs a record keeps", payload: `{"doc":{"key":"d"},"blobs":["` + b + `","` + c + `"]}`, want: []string{b, c}},
		{name: "a blob and the blobs it names, once each", payload: `{"blob":"` + a + `","blobs":["` + b + `","` + a + `"]}`, want: []string{a, b}},
		{name: "not an address", payload: `{"blob":"x","blobs":["sha256:zz"]}`, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, BlobRefs(Op{Payload: []byte(tc.payload)}))
		})
	}
}
