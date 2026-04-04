package localblob

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkedUpload(t *testing.T) {
	store, err := New(t.TempDir())
	require.NoError(t, err)
	ctx := t.Context()

	// Init upload.
	uploadID, err := store.InitUpload(ctx, "test-upload")
	require.NoError(t, err)
	assert.NotEmpty(t, uploadID)

	// Stage 3 chunks.
	require.NoError(t, store.StageChunk(ctx, uploadID, 0, []byte("chunk-0-data")))
	require.NoError(t, store.StageChunk(ctx, uploadID, 1, []byte("chunk-1-data")))
	require.NoError(t, store.StageChunk(ctx, uploadID, 2, []byte("chunk-2-data")))

	// Commit.
	require.NoError(t, store.CommitUpload(ctx, uploadID, 3))

	// The assembled blob should be downloadable by its content hash.
	// (The hash is of "chunk-0-datachunk-1-datachunk-2-data")
}

func TestChunkedUpload_Abort(t *testing.T) {
	store, err := New(t.TempDir())
	require.NoError(t, err)
	ctx := t.Context()

	uploadID, err := store.InitUpload(ctx, "abort-test")
	require.NoError(t, err)

	require.NoError(t, store.StageChunk(ctx, uploadID, 0, []byte("data")))

	// Abort should clean up.
	require.NoError(t, store.AbortUpload(ctx, uploadID))
}
