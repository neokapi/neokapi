package workspace

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// BlobPrefix starts every blob address: the digest algorithm, so an address
// says how it was computed.
const BlobPrefix = "sha256:"

// MaxBlobSize bounds a blob, in bytes before compression. A blob holds one
// imported bundle or one batch of approved wording, and a writer with more
// than this splits it across operations. The bound is also what keeps a read
// from inflating a stored row, including one merged in from another log, into
// more memory than any legitimate write could have produced.
const MaxBlobSize = 64 << 20

// ErrBlobTooLarge reports a blob over MaxBlobSize.
var ErrBlobTooLarge = fmt.Errorf("workspace: a blob holds at most %d bytes", MaxBlobSize)

// BlobAddress is the address bytes are stored under.
func BlobAddress(data []byte) string {
	sum := sha256.Sum256(data)
	return BlobPrefix + hex.EncodeToString(sum[:])
}

// validBlobAddress reports whether s is an address BlobAddress could return.
func validBlobAddress(s string) bool {
	hexPart, ok := strings.CutPrefix(s, BlobPrefix)
	if !ok || len(hexPart) != 2*sha256.Size || strings.ToLower(hexPart) != hexPart {
		return false
	}
	_, err := hex.DecodeString(hexPart)
	return err == nil
}

// PutBlob stores bytes an operation names, compressed, under the digest of the
// bytes themselves.
func (b *LocalBackend) PutBlob(ctx context.Context, data []byte) (string, error) {
	if len(data) > MaxBlobSize {
		return "", ErrBlobTooLarge
	}
	db, err := b.Registry(ctx)
	if err != nil {
		return "", err
	}
	if b.Describe().ReadOnly {
		return "", ErrReadOnly
	}
	address := BlobAddress(data)
	var packed bytes.Buffer
	zw := gzip.NewWriter(&packed)
	if _, err := zw.Write(data); err != nil {
		return "", fmt.Errorf("workspace: compress blob: %w", err)
	}
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("workspace: compress blob: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO workspace_blobs (digest, size, data) VALUES (?, ?, ?) ON CONFLICT(digest) DO NOTHING`,
		address, len(data), packed.Bytes()); err != nil {
		return "", fmt.Errorf("workspace: store blob: %w", err)
	}
	return address, nil
}

// Blob returns the bytes stored under an address, after checking they are the
// bytes the address names.
//
// The size a row records is checked before its bytes are read, the compressed
// bytes are read only when they fit the bound, and the inflated bytes are read
// through a limit of the recorded size. A row that lies about its size, or
// compressed bytes that inflate past it, are reported rather than read whole.
func (b *LocalBackend) Blob(ctx context.Context, digest string) ([]byte, error) {
	if !validBlobAddress(digest) {
		return nil, fmt.Errorf("workspace: %q is not a blob address", digest)
	}
	db, err := b.Registry(ctx)
	if err != nil {
		return nil, err
	}
	var size, stored int64
	switch err := db.QueryRowContext(ctx,
		`SELECT size, length(data) FROM workspace_blobs WHERE digest = ?`, digest).Scan(&size, &stored); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%w: %s", ErrNoBlob, digest)
	case err != nil:
		return nil, fmt.Errorf("workspace: read blob %s: %w", digest, err)
	}
	if size < 0 || size > MaxBlobSize || stored > MaxBlobSize {
		return nil, fmt.Errorf("%w: %s records %d bytes", ErrBlobTooLarge, digest, size)
	}
	var packed []byte
	if err := db.QueryRowContext(ctx,
		`SELECT data FROM workspace_blobs WHERE digest = ?`, digest).Scan(&packed); err != nil {
		return nil, fmt.Errorf("workspace: read blob %s: %w", digest, err)
	}
	data, err := inflate(packed, size)
	if err != nil {
		return nil, fmt.Errorf("workspace: read blob %s: %w", digest, err)
	}
	if BlobAddress(data) != digest {
		return nil, fmt.Errorf("workspace: blob %s does not hold the bytes its address names", digest)
	}
	return data, nil
}

// inflate decompresses a stored blob, reading one byte past the size it
// records so a blob that inflates further is caught without being read whole.
func inflate(packed []byte, size int64) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(zr, size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != size {
		return nil, fmt.Errorf("the blob does not inflate to the %d bytes it records", size)
	}
	return data, nil
}

// PutBlob stores bytes an operation names and returns their address.
func (w *Workspace) PutBlob(ctx context.Context, data []byte) (string, error) {
	return w.backend.PutBlob(ctx, data)
}

// Blob returns the bytes stored under an address.
func (w *Workspace) Blob(ctx context.Context, digest string) ([]byte, error) {
	return w.backend.Blob(ctx, digest)
}
