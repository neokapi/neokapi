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

	t.Run("a directory inside the state directory that links out is refused", func(t *testing.T) {
		root := newRoot(t)
		outside := t.TempDir()
		l := LayoutAt(root)
		require.NoError(t, os.MkdirAll(l.StateDir, 0o755))
		require.NoError(t, os.Symlink(outside, l.WorkDir()))
		require.ErrorContains(t, l.CheckState(), "leads outside the project")
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
