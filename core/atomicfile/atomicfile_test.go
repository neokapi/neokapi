package atomicfile_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/atomicfile"
)

// realPath resolves every symlink in path, which is what Replace returns: on
// macOS even t.TempDir() sits under one.
func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

func skipWithoutSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture makes a symlink")
	}
}

func TestReplace(t *testing.T) {
	t.Run("a symlink stays a link and its file takes the bytes", func(t *testing.T) {
		skipWithoutSymlinks(t)
		dir := t.TempDir()
		target := filepath.Join(dir, "real.txt")
		require.NoError(t, os.WriteFile(target, []byte("old"), 0o640))
		link := filepath.Join(t.TempDir(), "link.txt")
		require.NoError(t, os.Symlink(target, link))

		written, err := atomicfile.ReplaceBytes(link, []byte("new"))
		require.NoError(t, err)
		assert.Equal(t, realPath(t, target), written)

		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link is still a link")
		body, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "new", string(body))
		perm, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), perm.Mode().Perm(), "the file keeps its mode")
	})

	t.Run("a link to a link resolves to the file at the end", func(t *testing.T) {
		skipWithoutSymlinks(t)
		dir := t.TempDir()
		target := filepath.Join(dir, "real.txt")
		require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))
		first := filepath.Join(dir, "first.txt")
		require.NoError(t, os.Symlink(target, first))
		second := filepath.Join(dir, "second.txt")
		require.NoError(t, os.Symlink(first, second))

		written, err := atomicfile.ReplaceBytes(second, []byte("new"))
		require.NoError(t, err)
		assert.Equal(t, realPath(t, target), written)
		body, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "new", string(body))
	})

	t.Run("an existing file keeps its mode", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))

		_, err := atomicfile.ReplaceBytes(path, []byte("new"))
		require.NoError(t, err)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("a file that did not exist is created the way os.Create creates one", func(t *testing.T) {
		dir := t.TempDir()
		made := filepath.Join(dir, "made.txt")
		f, err := os.Create(made)
		require.NoError(t, err)
		require.NoError(t, f.Close())
		want, err := os.Stat(made)
		require.NoError(t, err)

		path := filepath.Join(dir, "new.txt")
		written, err := atomicfile.ReplaceBytes(path, []byte("new"))
		require.NoError(t, err)
		assert.Equal(t, path, written)
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, want.Mode().Perm(), info.Mode().Perm())
	})

	t.Run("a failed write leaves the file as it was and no temporary file behind", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "file.txt")
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
		fail := errors.New("the writer gave up")

		_, err := atomicfile.Replace(path, func(w io.Writer) error {
			_, _ = w.Write([]byte("half"))
			return fail
		})
		require.ErrorIs(t, err, fail)
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "old", string(body))
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Len(t, entries, 1, "the temporary file is gone")
	})

	t.Run("a failure at the destination is an atomicfile.Error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "missing", "file.txt")

		_, err := atomicfile.ReplaceBytes(path, []byte("new"))
		var ferr *atomicfile.Error
		require.ErrorAs(t, err, &ferr)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestResolve(t *testing.T) {
	skipWithoutSymlinks(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	require.NoError(t, os.WriteFile(target, []byte("body"), 0o640))
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink(target, link))

	got, mode, exists, err := atomicfile.Resolve(link)
	require.NoError(t, err)
	assert.Equal(t, realPath(t, target), got)
	assert.Equal(t, os.FileMode(0o640), mode)
	assert.True(t, exists)

	absent := filepath.Join(dir, "absent.txt")
	got, _, exists, err = atomicfile.Resolve(absent)
	require.NoError(t, err)
	assert.Equal(t, absent, got)
	assert.False(t, exists)
}
