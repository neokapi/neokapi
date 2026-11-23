//go:build unix

package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A state directory, or a directory kapi keeps in it, that a symbolic link
// leads out of the project is refused before anything is read or written
// there; one that stays inside passes.
func TestLayoutCheckState(t *testing.T) {
	newRoot := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, RecipeFileName), []byte("version: v1\nname: p\n"), 0o644))
		return root
	}

	t.Run("a state directory that links out is refused everywhere a layout is made", func(t *testing.T) {
		root := newRoot(t)
		outside := t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(root, StateDirName)))

		_, err := ResolveLayout(root)
		require.ErrorContains(t, err, "leads outside the project")
		_, err = LayoutFor(filepath.Join(root, RecipeFileName))
		require.ErrorContains(t, err, "leads outside the project")
		require.ErrorContains(t, EnsureLayout(LayoutAt(root)), "leads outside the project")
		entries, rerr := os.ReadDir(outside)
		require.NoError(t, rerr)
		assert.Empty(t, entries, "nothing is created through the link")
	})

	t.Run("a directory inside the state directory that is a link is refused", func(t *testing.T) {
		root := newRoot(t)
		outside := t.TempDir()
		l := LayoutAt(root)
		require.NoError(t, os.MkdirAll(l.StateDir, 0o755))
		require.NoError(t, os.Symlink(outside, filepath.Join(l.StateDir, WorkDirName)))
		require.ErrorContains(t, l.CheckState(), ".kapi/work is a symbolic link")
	})

	t.Run("a file kapi keeps that is a link is refused, wherever it leads", func(t *testing.T) {
		for _, rel := range []string{"work/store.db", "work/store.db-wal", "work/cache/index.db", "sync/sync-cache.json"} {
			root := newRoot(t)
			l := LayoutAt(root)
			other := filepath.Join(t.TempDir(), "store.db")
			require.NoError(t, os.WriteFile(other, []byte("outside"), 0o644))
			link := filepath.Join(l.StateDir, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
			require.NoError(t, os.Symlink(other, link))
			require.ErrorContains(t, l.CheckState(), ".kapi/"+rel+" is a symbolic link", rel)
			_, err := ResolveLayout(root)
			require.Error(t, err, rel)

			inside := filepath.Join(root, "kept.db")
			require.NoError(t, os.WriteFile(inside, nil, 0o644))
			require.NoError(t, os.Remove(link))
			require.NoError(t, os.Symlink(inside, link))
			require.ErrorContains(t, l.CheckState(), "is a symbolic link", "a link inside the project is refused too: %s", rel)
		}
	})

	t.Run("a fresh project and a state directory linked inside it pass", func(t *testing.T) {
		root := newRoot(t)
		require.NoError(t, EnsureLayout(LayoutAt(root)))
		require.NoError(t, LayoutAt(root).CheckState())

		other := newRoot(t)
		require.NoError(t, os.MkdirAll(filepath.Join(other, "state"), 0o755))
		require.NoError(t, os.Symlink("state", filepath.Join(other, StateDirName)))
		require.NoError(t, LayoutAt(other).CheckState())
	})
}
