// Package localblob provides a local filesystem implementation of BlobStore.
// It stores blobs as files using git-like sharding: {rootDir}/{key[0:2]}/{key[2:4]}/{key}.
// Pre-signed URL methods return ErrNotSupported; the CLI falls back to direct
// Upload/Download through the server proxy.
package localblob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/storage"
)

// Store implements storage.BlobStore using the local filesystem.
type Store struct {
	rootDir string
}

// New creates a local blob store rooted at the given directory.
// The directory is created if it does not exist.
func New(rootDir string) (*Store, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create blob root: %w", err)
	}
	return &Store{rootDir: rootDir}, nil
}

func (s *Store) blobPath(key string) string {
	if len(key) < 4 {
		return filepath.Join(s.rootDir, key)
	}
	return filepath.Join(s.rootDir, key[0:2], key[2:4], key)
}

// Upload stores binary content and returns a content-addressed BlobRef.
// If a blob with the same SHA-256 key already exists, the existing ref is returned.
func (s *Store) Upload(_ context.Context, data []byte, opts storage.UploadOptions) (*storage.BlobRef, error) {
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])

	path := s.blobPath(key)

	// Dedup: if the file already exists, return without writing.
	if info, err := os.Stat(path); err == nil {
		return &storage.BlobRef{
			Key:         key,
			Size:        info.Size(),
			ContentType: opts.ContentType,
		}, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create blob directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write blob: %w", err)
	}

	return &storage.BlobRef{
		Key:         key,
		Size:        int64(len(data)),
		ContentType: opts.ContentType,
	}, nil
}

// Download retrieves binary content by storage key.
func (s *Store) Download(_ context.Context, key string) (io.ReadCloser, error) {
	f, err := os.Open(s.blobPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrBlobNotFound
		}
		return nil, fmt.Errorf("open blob: %w", err)
	}
	return f, nil
}

// GenerateUploadURL is not supported by the local filesystem backend.
func (s *Store) GenerateUploadURL(_ context.Context, _ string, _ storage.SignOptions) (string, error) {
	return "", storage.ErrNotSupported
}

// GenerateDownloadURL is not supported by the local filesystem backend.
func (s *Store) GenerateDownloadURL(_ context.Context, _ string, _ storage.SignOptions) (string, error) {
	return "", storage.ErrNotSupported
}

// Delete removes a blob by storage key.
func (s *Store) Delete(_ context.Context, key string) error {
	err := os.Remove(s.blobPath(key))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// Exists checks whether a blob with the given key exists.
func (s *Store) Exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(s.blobPath(key))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat blob: %w", err)
}

// ---------------------------------------------------------------------------
// ChunkedBlobStore implementation (AD-038)
// ---------------------------------------------------------------------------

func (s *Store) InitUpload(_ context.Context, uploadKey string) (string, error) {
	uploadID := uploadKey // use the key as the upload ID for simplicity
	dir := filepath.Join(s.rootDir, "_uploads", uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}
	return uploadID, nil
}

func (s *Store) StageChunk(_ context.Context, uploadID string, chunkIndex int, data []byte) error {
	path := filepath.Join(s.rootDir, "_uploads", uploadID, fmt.Sprintf("chunk-%04d", chunkIndex))
	return os.WriteFile(path, data, 0o644)
}

func (s *Store) CommitUpload(ctx context.Context, uploadID string, totalChunks int) error {
	dir := filepath.Join(s.rootDir, "_uploads", uploadID)

	// Assemble chunks into final blob.
	var assembled []byte
	for i := range totalChunks {
		path := filepath.Join(dir, fmt.Sprintf("chunk-%04d", i))
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read chunk %d: %w", i, err)
		}
		assembled = append(assembled, data...)
	}

	// Store as a regular blob.
	h := sha256.Sum256(assembled)
	key := hex.EncodeToString(h[:])
	blobPath := s.blobPath(key)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(blobPath, assembled, 0o644); err != nil {
		return err
	}

	// Clean up upload dir.
	_ = os.RemoveAll(dir)
	return nil
}

func (s *Store) AbortUpload(_ context.Context, uploadID string) error {
	dir := filepath.Join(s.rootDir, "_uploads", uploadID)
	return os.RemoveAll(dir)
}

func (s *Store) GenerateChunkUploadURLs(_ context.Context, _ string, _ int, _ storage.SignOptions) ([]string, error) {
	return nil, storage.ErrNotSupported
}
