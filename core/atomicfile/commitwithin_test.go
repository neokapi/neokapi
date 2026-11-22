//go:build unix

package atomicfile_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/atomicfile"
)

func write(w io.Writer) error {
	_, err := io.WriteString(w, "body")
	return err
}

func withinTree(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "root")
	outside = filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "site"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	return root, outside
}

// A directory link put where the target's new directory goes, after the file
// was staged, is refused by the rename itself, which resolves its names
// inside the root.
func TestCommitWithinRefusesALinkSwappedInAfterStaging(t *testing.T) {
	root, outside := withinTree(t)
	target := filepath.Join(root, "site", "fr", "app.json")
	s, err := atomicfile.StageWithParents(target, write)
	require.NoError(t, err)

	require.NoError(t, os.Symlink(outside, filepath.Join(root, "site", "fr")))

	require.Error(t, s.CommitWithin(root))
	assert.NoFileExists(t, filepath.Join(outside, "app.json"), "nothing is written through the link")
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing lands outside the root")
}

// The same swap under a plain Commit is followed: the reason a check on the
// path followed by a rename by path is not enough.
func TestCommitFollowsALinkSwappedInAfterStaging(t *testing.T) {
	root, outside := withinTree(t)
	target := filepath.Join(root, "site", "fr", "app.json")
	s, err := atomicfile.StageWithParents(target, write)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "site", "fr")))
	require.NoError(t, s.Commit())
	assert.FileExists(t, filepath.Join(outside, "app.json"))
}

func TestCommitWithin(t *testing.T) {
	t.Run("a new file and its directories inside the root", func(t *testing.T) {
		root, _ := withinTree(t)
		target := filepath.Join(root, "site", "fr", "app.json")
		s, err := atomicfile.StageWithParents(target, write)
		require.NoError(t, err)
		require.NoError(t, s.CommitWithin(root))
		body, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "body", string(body))
	})

	t.Run("a file reached through a link inside the root", func(t *testing.T) {
		root, _ := withinTree(t)
		real := filepath.Join(root, "site", "real.json")
		require.NoError(t, os.WriteFile(real, []byte("old"), 0o640))
		link := filepath.Join(root, "site", "link.json")
		require.NoError(t, os.Symlink("real.json", link))
		s, err := atomicfile.Stage(link, write)
		require.NoError(t, err)
		require.NoError(t, s.CommitWithin(root))
		body, _ := os.ReadFile(real)
		assert.Equal(t, "body", string(body))
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink)
	})

	t.Run("a root reached through a link", func(t *testing.T) {
		root, _ := withinTree(t)
		via := filepath.Join(t.TempDir(), "via")
		require.NoError(t, os.Symlink(root, via))
		target := filepath.Join(via, "site", "de.json")
		s, err := atomicfile.Stage(target, write)
		require.NoError(t, err)
		require.NoError(t, s.CommitWithin(via))
		assert.FileExists(t, filepath.Join(root, "site", "de.json"))
	})

	t.Run("a relative target under a root named as the working directory", func(t *testing.T) {
		root, _ := withinTree(t)
		t.Chdir(root)
		s, err := atomicfile.Stage(filepath.Join("site", "rel.json"), write)
		require.NoError(t, err)
		require.NoError(t, s.CommitWithin(root))
		assert.FileExists(t, filepath.Join(root, "site", "rel.json"))
	})

	t.Run("a target outside the root is refused and the staged file removed", func(t *testing.T) {
		root, outside := withinTree(t)
		target := filepath.Join(outside, "x.json")
		s, err := atomicfile.Stage(target, write)
		require.NoError(t, err)
		require.Error(t, s.CommitWithin(root))
		entries, _ := os.ReadDir(outside)
		assert.Empty(t, entries)
	})
}
