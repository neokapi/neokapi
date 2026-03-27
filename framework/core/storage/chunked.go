package storage

import (
	"context"
)

// ChunkedBlobStore extends BlobStore with chunked upload support.
// Backends that support chunked uploads (Azure Block Blobs, S3 multipart)
// implement this interface. Backends that don't can fall back to assembling
// chunks in memory and calling Upload.
type ChunkedBlobStore interface {
	BlobStore

	// InitUpload prepares a chunked upload session and returns an upload ID.
	// The uploadKey is the final blob key after commit.
	InitUpload(ctx context.Context, uploadKey string) (uploadID string, err error)

	// StageChunk uploads one chunk of a multi-part upload.
	// chunkIndex determines the order in the final blob.
	StageChunk(ctx context.Context, uploadID string, chunkIndex int, data []byte) error

	// CommitUpload finalizes the chunked upload, assembling all staged chunks
	// into the final blob at the uploadKey.
	CommitUpload(ctx context.Context, uploadID string, totalChunks int) error

	// AbortUpload cancels a chunked upload and cleans up staged chunks.
	AbortUpload(ctx context.Context, uploadID string) error

	// GenerateChunkUploadURLs returns pre-signed URLs for direct client upload
	// of each chunk. Returns ErrNotSupported for local backends.
	GenerateChunkUploadURLs(ctx context.Context, uploadID string, chunkCount int, opts SignOptions) ([]string, error)
}
