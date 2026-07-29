// Package localblob provides a local filesystem implementation of BlobStore.
// It stores blobs as files using git-like sharding: {rootDir}/{key[0:2]}/{key[2:4]}/{key}.
// Pre-signed URL methods return ErrNotSupported; the CLI falls back to direct
// Upload/Download through the server proxy.
package localblob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/storage"
)

// errBadKey marks a storage key that cannot name a blob in this store. Keys
// arrive from clients (a sync manifest's chunk hashes, an asset lookup), and a
// key is only ever a SHA-256 digest this store itself produced — so anything
// else is rejected before it reaches the filesystem rather than resolved into a
// path and tried.
var errBadKey = errors.New("invalid blob key")

// blobKeyPattern is the exact shape of a key this store issues: the lowercase
// hex encoding of a SHA-256 digest, as written by Upload and CommitUpload.
var blobKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Store implements storage.BlobStore using the local filesystem.
type Store struct {
	// rootDir is absolute, so every derived path can be checked for
	// containment against it without re-resolving.
	rootDir string
}

// New creates a local blob store rooted at the given directory.
// The directory is created if it does not exist.
func New(rootDir string) (*Store, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create blob root: %w", err)
	}
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve blob root: %w", err)
	}
	return &Store{rootDir: abs}, nil
}

// contain joins rel onto the store root and returns the result only if it stays
// inside that root. The shape checks in blobPath and uploadDir already make
// escape impossible; this is the backstop that keeps it impossible if a future
// caller relaxes one of them.
func (s *Store) contain(rel ...string) (string, error) {
	p := filepath.Join(append([]string{s.rootDir}, rel...)...)
	if p != s.rootDir && !strings.HasPrefix(p, s.rootDir+string(filepath.Separator)) {
		return "", errBadKey
	}
	return p, nil
}

// blobPath maps a storage key to its sharded on-disk path:
// {rootDir}/{key[0:2]}/{key[2:4]}/{key}. The key is validated first — it is a
// path component, so a value like "../../etc/passwd" would otherwise select a
// file outside the store.
func (s *Store) blobPath(key string) (string, error) {
	if !blobKeyPattern.MatchString(key) {
		return "", errBadKey
	}
	return s.contain(key[0:2], key[2:4], key)
}

// uploadDir maps a chunked-upload session ID to its staging directory. An
// upload ID is a caller-chosen label rather than a digest, so it is held to the
// weaker rule that it must be one ordinary path segment.
func (s *Store) uploadDir(uploadID string) (string, error) {
	if uploadID == "" || uploadID == "." || uploadID == ".." ||
		strings.ContainsAny(uploadID, `/\`) || strings.Contains(uploadID, "\x00") {
		return "", errBadKey
	}
	return s.contain("_uploads", uploadID)
}

// Upload stores binary content and returns a content-addressed BlobRef.
// If a blob with the same SHA-256 key already exists, the existing ref is returned.
func (s *Store) Upload(_ context.Context, data []byte, opts storage.UploadOptions) (*storage.BlobRef, error) {
	hash := sha256.Sum256(data)
	key := hex.EncodeToString(hash[:])

	path, err := s.blobPath(key)
	if err != nil {
		return nil, err
	}

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

// Download retrieves binary content by storage key. A key that this store could
// never have issued is reported as a miss, exactly like a key that is well
// formed but absent — the caller learns nothing about the filesystem from the
// difference.
func (s *Store) Download(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.blobPath(key)
	if err != nil {
		return nil, storage.ErrBlobNotFound
	}
	f, err := os.Open(path)
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

// Delete removes a blob by storage key. Deleting a key this store could never
// have issued is a no-op rather than an error: there is nothing to remove, and
// the request must not be allowed to name a file elsewhere on disk.
func (s *Store) Delete(_ context.Context, key string) error {
	path, err := s.blobPath(key)
	if err != nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// Exists checks whether a blob with the given key exists. A key this store
// could never have issued does not exist, which is an answer rather than a
// failure — callers use Exists to validate client input.
func (s *Store) Exists(_ context.Context, key string) (bool, error) {
	path, err := s.blobPath(key)
	if err != nil {
		return false, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat blob: %w", err)
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// ChunkedBlobStore implementation (Bowrain AD-009)
// ---------------------------------------------------------------------------

func (s *Store) InitUpload(_ context.Context, uploadKey string) (string, error) {
	uploadID := uploadKey // use the key as the upload ID for simplicity
	dir, err := s.uploadDir(uploadID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}
	return uploadID, nil
}

func (s *Store) StageChunk(_ context.Context, uploadID string, chunkIndex int, data []byte) error {
	dir, err := s.uploadDir(uploadID)
	if err != nil {
		return err
	}
	// chunkIndex is formatted, not concatenated, so it contributes no separators.
	return os.WriteFile(filepath.Join(dir, fmt.Sprintf("chunk-%04d", chunkIndex)), data, 0o644)
}

func (s *Store) CommitUpload(_ context.Context, uploadID string, totalChunks int) error {
	dir, err := s.uploadDir(uploadID)
	if err != nil {
		return err
	}

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
	blobPath, err := s.blobPath(key)
	if err != nil {
		return err
	}
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
	dir, err := s.uploadDir(uploadID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (s *Store) GenerateChunkUploadURLs(_ context.Context, _ string, _ int, _ storage.SignOptions) ([]string, error) {
	return nil, storage.ErrNotSupported
}
