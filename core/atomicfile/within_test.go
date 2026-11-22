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

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written in %s", dir)
}

// A directory link that leads out of the root, put in place after the caller
// checked the path and before the staging, refuses the staging itself: the
// temporary file is never created outside the root.
func TestStageWithinRefusesALinkBeforeStaging(t *testing.T) {
	root, outside := withinTree(t)
	require.NoError(t, os.Remove(filepath.Join(root, "site")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "site")))

	_, err := atomicfile.StageWithin(root, filepath.Join(root, "site", "app.json"), write)
	require.Error(t, err)
	assertEmpty(t, outside)
}

// A directory link put where the target's new directory goes, after the file
// was staged, is refused by the rename, which resolves its names inside the
// root.
func TestStageWithinRefusesALinkSwappedInAfterStaging(t *testing.T) {
	root, outside := withinTree(t)
	s, err := atomicfile.StageWithin(root, filepath.Join(root, "site", "fr", "app.json"), write)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "site", "fr")))

	require.Error(t, s.Commit())
	assertEmpty(t, outside)
}

// The same swap under a file staged by path is followed: the reason a check
// on the path followed by staging and a rename by path is not enough.
func TestStageByPathFollowsALinkSwappedInAfterStaging(t *testing.T) {
	root, outside := withinTree(t)
	s, err := atomicfile.StageWithParents(filepath.Join(root, "site", "fr", "app.json"), write)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "site", "fr")))
	require.NoError(t, s.Commit())
	assert.FileExists(t, filepath.Join(outside, "app.json"))
}

func TestStageWithin(t *testing.T) {
	t.Run("a new file and its directories inside the root", func(t *testing.T) {
		root, _ := withinTree(t)
		target := filepath.Join(root, "site", "fr", "app.json")
		s, err := atomicfile.StageWithin(root, target, write)
		require.NoError(t, err)
		assert.Equal(t, target, s.Target())
		got, err := s.ReadAll()
		require.NoError(t, err)
		assert.Equal(t, "body", string(got))
		require.NoError(t, s.Commit())
		body, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "body", string(body))
		assert.Equal(t, []string{"app.json"}, names(t, filepath.Join(root, "site", "fr")))
	})

	t.Run("a file reached through a link inside the root keeps its mode", func(t *testing.T) {
		root, _ := withinTree(t)
		real := filepath.Join(root, "site", "real.json")
		require.NoError(t, os.WriteFile(real, []byte("old"), 0o640))
		link := filepath.Join(root, "site", "link.json")
		require.NoError(t, os.Symlink("real.json", link))
		s, err := atomicfile.StageWithin(root, link, write)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		body, _ := os.ReadFile(real)
		assert.Equal(t, "body", string(body))
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
		perm, _ := os.Stat(real)
		assert.Equal(t, os.FileMode(0o640), perm.Mode().Perm())
	})

	t.Run("a file link that leads out of the root is refused", func(t *testing.T) {
		root, outside := withinTree(t)
		secret := filepath.Join(outside, "secret.json")
		require.NoError(t, os.WriteFile(secret, []byte("outside"), 0o644))
		require.NoError(t, os.Symlink(secret, filepath.Join(root, "site", "fr.json")))
		_, err := atomicfile.StageWithin(root, filepath.Join(root, "site", "fr.json"), write)
		require.Error(t, err)
		body, _ := os.ReadFile(secret)
		assert.Equal(t, "outside", string(body))
	})

	t.Run("a root reached through a link", func(t *testing.T) {
		root, _ := withinTree(t)
		via := filepath.Join(t.TempDir(), "via")
		require.NoError(t, os.Symlink(root, via))
		s, err := atomicfile.StageWithin(via, filepath.Join(via, "site", "de.json"), write)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		assert.FileExists(t, filepath.Join(root, "site", "de.json"))
	})

	t.Run("a relative target under a root named as the working directory", func(t *testing.T) {
		root, _ := withinTree(t)
		t.Chdir(root)
		s, err := atomicfile.StageWithin(root, filepath.Join("site", "rel.json"), write)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		assert.FileExists(t, filepath.Join(root, "site", "rel.json"))
	})

	t.Run("a target outside the root is refused", func(t *testing.T) {
		root, outside := withinTree(t)
		_, err := atomicfile.StageWithin(root, filepath.Join(outside, "x.json"), write)
		require.Error(t, err)
		assertEmpty(t, outside)
	})

	t.Run("a discarded file leaves nothing", func(t *testing.T) {
		root, _ := withinTree(t)
		s, err := atomicfile.StageWithin(root, filepath.Join(root, "site", "new", "x.json"), write)
		require.NoError(t, err)
		require.NoError(t, s.Discard())
		assert.Empty(t, names(t, filepath.Join(root, "site")))
	})
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
