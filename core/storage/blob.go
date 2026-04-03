package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotSupported is returned by BlobStore methods that are not supported
// by a particular backend (e.g., pre-signed URLs on the local filesystem).
var ErrNotSupported = errors.New("operation not supported by this backend")

// ErrBlobNotFound is returned when a blob with the given key does not exist.
var ErrBlobNotFound = errors.New("blob not found")

// BlobStore provides content-addressed binary storage.
// Implementations must be safe for concurrent use.
type BlobStore interface {
	// Upload stores binary content and returns a storage key.
	// The key is content-addressed (SHA-256 of data) to enable deduplication.
	Upload(ctx context.Context, data []byte, opts UploadOptions) (*BlobRef, error)

	// Download retrieves binary content by storage key.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// GenerateUploadURL returns a pre-signed URL for direct client upload.
	// Returns ErrNotSupported for backends that don't support pre-signed URLs.
	GenerateUploadURL(ctx context.Context, key string, opts SignOptions) (string, error)

	// GenerateDownloadURL returns a pre-signed URL for direct client download.
	// Returns ErrNotSupported for backends that don't support pre-signed URLs.
	GenerateDownloadURL(ctx context.Context, key string, opts SignOptions) (string, error)

	// Delete removes a blob by storage key.
	Delete(ctx context.Context, key string) error

	// Exists checks whether a blob with the given key exists.
	Exists(ctx context.Context, key string) (bool, error)
}

// UploadOptions configures a blob upload.
type UploadOptions struct {
	ContentType string // MIME type (e.g., "image/png")
	Filename    string // Original filename for Content-Disposition
}

// SignOptions configures pre-signed URL generation.
type SignOptions struct {
	ExpiresIn time.Duration // SAS token / pre-signed URL TTL (default: 1 hour)
}

// BlobRef is the result of a successful upload.
type BlobRef struct {
	Key         string // SHA-256 hex of content
	Size        int64
	ContentType string
}
