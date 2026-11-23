//go:build unix

package dataroot

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsurePrivateDir(t *testing.T) {
	t.Run("a new directory is created with mode 0700", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "root")
		require.NoError(t, ensurePrivateDir(dir))
		info, err := os.Lstat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	})

	t.Run("an existing private directory of this user is accepted", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "root")
		require.NoError(t, os.Mkdir(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "kept"), []byte("x"), 0o600))
		require.NoError(t, ensurePrivateDir(dir))
		assert.FileExists(t, filepath.Join(dir, "kept"))
	})

	t.Run("an existing link is refused", func(t *testing.T) {
		tmp := t.TempDir()
		target := filepath.Join(tmp, "elsewhere")
		require.NoError(t, os.Mkdir(target, 0o700))
		link := filepath.Join(tmp, "root")
		require.NoError(t, os.Symlink(target, link))
		err := ensurePrivateDir(link)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symbolic link")
	})

	t.Run("an existing directory open to everyone is refused", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "root")
		require.NoError(t, os.Mkdir(dir, 0o700))
		require.NoError(t, os.Chmod(dir, 0o777))
		err := ensurePrivateDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "group or other")
	})

	t.Run("an existing file is refused", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "root")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
		require.Error(t, ensurePrivateDir(path))
	})
}

// A link named like a dead binary's root is left alone, and so is what it
// points at: the sweep removes only real directories this user owns.
func TestSweepRefusesALink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(t.TempDir(), "precious")
	require.NoError(t, os.Mkdir(target, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(target, "file"), []byte("x"), 0o600))

	const dead = 100
	link := filepath.Join(tmp, testDataDirPrefix+strconv.Itoa(dead))
	require.NoError(t, os.Symlink(target, link))

	removed := sweepTestDataDirs(tmp, 4242, func(int) bool { return false },
		func(int) (time.Time, bool) { return time.Time{}, false })
	assert.Empty(t, removed)
	assert.FileExists(t, filepath.Join(target, "file"))
	_, err := os.Lstat(link)
	assert.NoError(t, err, "the link itself is left for its owner")
}

// The test binary's own root and the per-user directories under it are
// private to this user.
func TestTheTestRootAndUserDirsArePrivate(t *testing.T) {
	root := testDataDir()
	require.NoError(t, checkPrivateDir(root))
	for _, kind := range []string{"config", "home"} {
		dir, ok := TestUserDir(kind)
		require.True(t, ok)
		require.NoError(t, checkPrivateDir(dir), kind)
		require.NoError(t, checkPrivateDir(filepath.Dir(dir)))
	}
}
