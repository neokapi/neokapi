package localblob

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadAndDownload(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := t.Context()
	data := []byte("hello blob storage")

	ref, err := s.Upload(ctx, data, storage.UploadOptions{ContentType: "text/plain"})
	require.NoError(t, err)
	assert.NotEmpty(t, ref.Key)
	assert.Equal(t, int64(len(data)), ref.Size)
	assert.Equal(t, "text/plain", ref.ContentType)

	// Download and verify content.
	rc, err := s.Download(ctx, ref.Key)
	require.NoError(t, err)
	defer rc.Close()

	got := make([]byte, 1024)
	n, _ := rc.Read(got)
	assert.Equal(t, data, got[:n])
}

func TestDedup(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := t.Context()
	data := []byte("dedup test data")

	ref1, err := s.Upload(ctx, data, storage.UploadOptions{})
	require.NoError(t, err)

	ref2, err := s.Upload(ctx, data, storage.UploadOptions{})
	require.NoError(t, err)

	assert.Equal(t, ref1.Key, ref2.Key, "same data should produce same key")
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := t.Context()

	exists, err := s.Exists(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	ref, err := s.Upload(ctx, []byte("exists test"), storage.UploadOptions{})
	require.NoError(t, err)

	exists, err = s.Exists(ctx, ref.Key)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := t.Context()

	ref, err := s.Upload(ctx, []byte("delete me"), storage.UploadOptions{})
	require.NoError(t, err)

	err = s.Delete(ctx, ref.Key)
	require.NoError(t, err)

	exists, err := s.Exists(ctx, ref.Key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDownloadNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	_, err = s.Download(t.Context(), "nonexistent")
	assert.ErrorIs(t, err, storage.ErrBlobNotFound)
}

func TestPreSignedURLsNotSupported(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := t.Context()

	_, err = s.GenerateUploadURL(ctx, "key", storage.SignOptions{})
	require.ErrorIs(t, err, storage.ErrNotSupported)

	_, err = s.GenerateDownloadURL(ctx, "key", storage.SignOptions{})
	assert.ErrorIs(t, err, storage.ErrNotSupported)
}

// TestKeysOutsideTheStoreAreNotResolved pins the rule that a storage key names
// a blob this store issued and nothing else. Keys reach the store from client
// input — a sync manifest's chunk hashes, an asset-existence probe — so a key
// that is not a SHA-256 digest must never be resolved into a path and tried.
func TestKeysOutsideTheStoreAreNotResolved(t *testing.T) {
	root := t.TempDir()
	// A file next to the store root: the kind of neighbour a traversal key
	// would try to reach.
	outside := filepath.Join(root, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("not a blob"), 0o600))

	s, err := New(filepath.Join(root, "blobs"))
	require.NoError(t, err)
	ctx := t.Context()

	keys := []struct {
		name string
		key  string
	}{
		{"parent traversal", "../outside.txt"},
		{"nested traversal", "../../etc/passwd"},
		{"traversal through the shard split", "..%2f..%2foutside.txt"},
		{"absolute path", "/etc/passwd"},
		{"absolute path into the temp dir", outside},
		{"empty", ""},
		{"short non-hex", "ab"},
		{"non-hex of digest length", strings.Repeat("z", 64)},
		{"uppercase hex", strings.ToUpper(strings.Repeat("ab", 32))},
		{"digest with a trailing segment", strings.Repeat("ab", 32) + "/../../outside.txt"},
		{"digest with a null byte", strings.Repeat("ab", 32) + "\x00"},
	}

	for _, tc := range keys {
		t.Run(tc.name, func(t *testing.T) {
			// Download reports a miss rather than opening the file.
			rc, err := s.Download(ctx, tc.key)
			if err == nil {
				rc.Close()
				t.Fatalf("Download(%q) resolved to a readable file", tc.key)
			}
			require.ErrorIs(t, err, storage.ErrBlobNotFound)

			// Exists answers false — callers use it to validate input, so a
			// malformed key is an answer, not a failure.
			exists, err := s.Exists(ctx, tc.key)
			require.NoError(t, err)
			assert.False(t, exists)

			// Delete is a no-op and leaves the neighbouring file alone.
			require.NoError(t, s.Delete(ctx, tc.key))
			assert.FileExists(t, outside, "Delete(%q) must not reach outside the store", tc.key)
		})
	}
}

// TestUploadSessionsStayInsideTheStore covers the staging directory, whose ID
// is a caller-chosen label rather than a digest and so is held to the weaker
// rule that it must be one ordinary path segment.
func TestUploadSessionsStayInsideTheStore(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	require.NoError(t, os.MkdirAll(victim, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(victim, "keep.txt"), []byte("keep"), 0o600))

	s, err := New(filepath.Join(root, "blobs"))
	require.NoError(t, err)
	ctx := t.Context()

	for _, id := range []string{"../victim", "../../victim", "/tmp", "", ".", ".."} {
		t.Run("id="+id, func(t *testing.T) {
			_, err := s.InitUpload(ctx, id)
			require.Error(t, err)
			require.Error(t, s.StageChunk(ctx, id, 0, []byte("x")))
			require.Error(t, s.CommitUpload(ctx, id, 1))
			// AbortUpload is the dangerous one: it removes a tree.
			require.Error(t, s.AbortUpload(ctx, id))
			assert.DirExists(t, victim)
			assert.FileExists(t, filepath.Join(victim, "keep.txt"))
		})
	}
}

// Verify Store implements BlobStore interface.
var _ storage.BlobStore = (*Store)(nil)
