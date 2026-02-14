package kapiproject

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewState(t *testing.T) {
	state := NewState()
	require.NotNil(t, state)
	assert.NotNil(t, state.Files)
	assert.NotNil(t, state.RemoteItems)
	assert.Len(t, state.Files, 0)
	assert.Len(t, state.RemoteItems, 0)
}

func TestLoadStateFunction(t *testing.T) {
	t.Run("load creates new state if file not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		state, err := LoadState(kapiDir)
		require.NoError(t, err)
		require.NotNil(t, state)
		assert.NotNil(t, state.Files)
		assert.NotNil(t, state.RemoteItems)
	})

	t.Run("load existing state file", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		// Create state with data
		pullTime, _ := time.Parse(time.RFC3339, "2026-01-01T10:00:00Z")
		pushTime, _ := time.Parse(time.RFC3339, "2026-01-01T11:00:00Z")
		modTime, _ := time.Parse(time.RFC3339, "2026-01-01T12:00:00Z")

		originalState := &State{
			LastPull: pullTime,
			LastPush: pushTime,
			Files: map[string]*FileState{
				"test.txt": {
					ContentHash: "abc123",
					Modified:    modTime,
				},
			},
			RemoteItems: map[string]*RemoteItemState{
				"remote-1": {
					ContentHash: "def456",
					Modified:    modTime,
					LocalPath:   "test.txt",
				},
			},
		}

		require.NoError(t, SaveState(kapiDir, originalState))

		// Load it back
		loaded, err := LoadState(kapiDir)
		require.NoError(t, err)
		require.NotNil(t, loaded)

		assert.Equal(t, pullTime.Unix(), loaded.LastPull.Unix())
		assert.Equal(t, pushTime.Unix(), loaded.LastPush.Unix())
		assert.Len(t, loaded.Files, 1)
		assert.Equal(t, "abc123", loaded.Files["test.txt"].ContentHash)
		assert.Len(t, loaded.RemoteItems, 1)
		assert.Equal(t, "def456", loaded.RemoteItems["remote-1"].ContentHash)
	})

	t.Run("load invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		// Write invalid JSON
		statePath := filepath.Join(kapiDir, StateFile)
		require.NoError(t, os.WriteFile(statePath, []byte("{invalid json}"), 0600))

		_, err := LoadState(kapiDir)
		require.Error(t, err)
	})
}

func TestSaveStateFunction(t *testing.T) {
	t.Run("save and reload state", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		modTime := time.Now().Truncate(time.Second)
		state := &State{
			LastPull: modTime,
			LastPush: modTime,
			Files: map[string]*FileState{
				"file1.txt": {
					ContentHash: "hash1",
					Modified:    modTime,
				},
				"file2.txt": {
					ContentHash: "hash2",
					Modified:    modTime,
				},
			},
			RemoteItems: map[string]*RemoteItemState{
				"item1": {
					ContentHash: "rhash1",
					Modified:    modTime,
					LocalPath:   "file1.txt",
				},
			},
		}

		err := SaveState(kapiDir, state)
		require.NoError(t, err)

		// Verify file was created
		statePath := filepath.Join(kapiDir, StateFile)
		_, err = os.Stat(statePath)
		require.NoError(t, err)

		// Reload and verify
		loaded, err := LoadState(kapiDir)
		require.NoError(t, err)

		assert.Equal(t, state.LastPull.Unix(), loaded.LastPull.Unix())
		assert.Equal(t, state.LastPush.Unix(), loaded.LastPush.Unix())
		assert.Len(t, loaded.Files, 2)
		assert.Equal(t, "hash1", loaded.Files["file1.txt"].ContentHash)
		assert.Equal(t, "hash2", loaded.Files["file2.txt"].ContentHash)
		assert.Len(t, loaded.RemoteItems, 1)
		assert.Equal(t, "rhash1", loaded.RemoteItems["item1"].ContentHash)
	})
}

func TestUpdateFileState(t *testing.T) {
	state := NewState()

	modTime := time.Now().Truncate(time.Second)
	state.UpdateFileState("test.txt", "abc123", modTime)

	require.Len(t, state.Files, 1)
	require.NotNil(t, state.Files["test.txt"])
	assert.Equal(t, "abc123", state.Files["test.txt"].ContentHash)
	assert.Equal(t, modTime.Unix(), state.Files["test.txt"].Modified.Unix())
}

func TestUpdateRemoteItemState(t *testing.T) {
	state := NewState()

	modTime := time.Now().Truncate(time.Second)
	state.UpdateRemoteItemState("item123", "def456", modTime, "local/path.txt")

	require.Len(t, state.RemoteItems, 1)
	require.NotNil(t, state.RemoteItems["item123"])
	assert.Equal(t, "def456", state.RemoteItems["item123"].ContentHash)
	assert.Equal(t, modTime.Unix(), state.RemoteItems["item123"].Modified.Unix())
	assert.Equal(t, "local/path.txt", state.RemoteItems["item123"].LocalPath)
}

func TestUpdateFileStateWithNilMap(t *testing.T) {
	state := &State{
		Files: nil,
	}

	modTime := time.Now()
	state.UpdateFileState("test.txt", "hash", modTime)

	require.NotNil(t, state.Files)
	assert.Len(t, state.Files, 1)
}

func TestUpdateRemoteItemStateWithNilMap(t *testing.T) {
	state := &State{
		RemoteItems: nil,
	}

	modTime := time.Now()
	state.UpdateRemoteItemState("item1", "hash", modTime, "path.txt")

	require.NotNil(t, state.RemoteItems)
	assert.Len(t, state.RemoteItems, 1)
}
