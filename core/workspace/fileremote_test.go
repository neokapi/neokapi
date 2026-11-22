//go:build !js && !windows

package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

// segment is a name in the layout's log directory.
var segment = workspace.SegmentName("w1", strings.Repeat("0", 24))

// A link another writer plants inside the shared folder leads neither a read
// nor a write out of it.
func TestFileRemote_ALinkInsideTheFolderStaysInside(t *testing.T) {
	outside := t.TempDir()
	share := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0o600))

	t.Run("a read through a planted link", func(t *testing.T) {
		require.NoError(t, os.MkdirAll(filepath.Join(share, "log", "w1"), 0o755))
		planted := filepath.Join(share, filepath.FromSlash(segment))
		require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), planted))
		t.Cleanup(func() { _ = os.Remove(planted) })

		r := workspace.NewFileRemote(share)
		names, err := r.List(t.Context(), workspace.RemoteLogDir)
		require.NoError(t, err)
		assert.Empty(t, names, "a link is not an object")
		_, err = r.Get(t.Context(), segment)
		require.Error(t, err, "the link is not followed")
	})

	t.Run("a write through a planted directory link", func(t *testing.T) {
		require.NoError(t, os.RemoveAll(filepath.Join(share, "log")))
		require.NoError(t, os.Symlink(outside, filepath.Join(share, "log")))

		r := workspace.NewFileRemote(share)
		err := r.Put(t.Context(), workspace.Object{Name: segment, Data: []byte("{}\n")})
		require.Error(t, err)
		assert.NoDirExists(t, filepath.Join(outside, "w1"), "nothing is written outside the folder")
	})
}

// The folder is opened once: replacing its path with a link afterwards does
// not move the remote.
func TestFileRemote_AFolderSwappedAfterTheOpenIsNotFollowed(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	share := filepath.Join(parent, "share")
	require.NoError(t, os.Mkdir(share, 0o755))

	r := workspace.NewFileRemote(share)
	t.Cleanup(func() { _ = r.Close() })
	_, err := r.List(t.Context(), workspace.RemoteLogDir)
	require.NoError(t, err)

	require.NoError(t, os.Rename(share, filepath.Join(parent, "moved")))
	require.NoError(t, os.Symlink(outside, share))
	require.NoError(t, r.Put(t.Context(), workspace.Object{Name: segment, Data: []byte("{}\n")}))

	assert.NoDirExists(t, filepath.Join(outside, "log"), "the write went to the folder that was opened")
	assert.FileExists(t, filepath.Join(parent, "moved", filepath.FromSlash(segment)))
}
