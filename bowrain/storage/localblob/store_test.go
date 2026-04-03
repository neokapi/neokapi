package localblob

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadAndDownload(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := context.Background()
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

	ctx := context.Background()
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

	ctx := context.Background()

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

	ctx := context.Background()

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

	_, err = s.Download(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, storage.ErrBlobNotFound)
}

func TestPreSignedURLsNotSupported(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = s.GenerateUploadURL(ctx, "key", storage.SignOptions{})
	assert.ErrorIs(t, err, storage.ErrNotSupported)

	_, err = s.GenerateDownloadURL(ctx, "key", storage.SignOptions{})
	assert.ErrorIs(t, err, storage.ErrNotSupported)
}

// Verify Store implements BlobStore interface.
var _ storage.BlobStore = (*Store)(nil)
