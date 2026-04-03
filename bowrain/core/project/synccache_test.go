package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncCacheAssetRoundTrip(t *testing.T) {
	dir := t.TempDir()

	cache := &SyncCache{
		ProjectID:     "proj-1",
		StreamCursors: map[string]int64{"main": 42},
		Files: map[string]*FileCache{
			"docs/manual.docx": {
				Blocks: map[string]string{"b1": "hash1"},
				Assets: map[string]string{
					"image1.png": "sha256:aabbccdd",
					"chart.emf":  "sha256:11223344",
				},
			},
		},
	}

	require.NoError(t, cache.Save(dir))

	loaded := LoadSyncCache(dir)
	assert.Equal(t, "proj-1", loaded.ProjectID)
	assert.Equal(t, int64(42), loaded.GetStreamCursor("main"))

	fc := loaded.Files["docs/manual.docx"]
	require.NotNil(t, fc)
	assert.Equal(t, "hash1", fc.Blocks["b1"])
	assert.Equal(t, "sha256:aabbccdd", fc.Assets["image1.png"])
	assert.Equal(t, "sha256:11223344", fc.Assets["chart.emf"])
}

func TestSyncCacheAssetNilSafety(t *testing.T) {
	dir := t.TempDir()

	// Cache without assets field should load fine.
	cache := &SyncCache{
		Files: map[string]*FileCache{
			"file.json": {Blocks: map[string]string{"b1": "h1"}},
		},
		StreamCursors: map[string]int64{},
	}
	require.NoError(t, cache.Save(dir))

	loaded := LoadSyncCache(dir)
	fc := loaded.Files["file.json"]
	require.NotNil(t, fc)
	assert.Nil(t, fc.Assets) // omitempty: not present in JSON
}
