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
func (b *LocalBackend) Blob(ctx context.Context, digest string) ([]byte, error) {
	if !validBlobAddress(digest) {
		return nil, fmt.Errorf("workspace: %q is not a blob address", digest)
	}
	db, err := b.Registry(ctx)
	if err != nil {
		return nil, err
	}
	var packed []byte
	switch err := db.QueryRowContext(ctx,
		`SELECT data FROM workspace_blobs WHERE digest = ?`, digest).Scan(&packed); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%w: %s", ErrNoBlob, digest)
	case err != nil:
		return nil, fmt.Errorf("workspace: read blob %s: %w", digest, err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, fmt.Errorf("workspace: read blob %s: %w", digest, err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("workspace: read blob %s: %w", digest, err)
	}
	if BlobAddress(data) != digest {
		return nil, fmt.Errorf("workspace: blob %s does not hold the bytes its address names", digest)
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
