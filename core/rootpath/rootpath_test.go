//go:build unix

package rootpath_test

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/rootpath"
)

// tree makes a root and, beside it, an outside directory holding one file.
func tree(t *testing.T) (root, outside, secret string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "root")
	outside = filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	secret = filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside"), 0o644))
	return root, outside, secret
}

func requireEscape(t *testing.T, err error, link string) {
	t.Helper()
	var esc *rootpath.EscapeError
	require.ErrorAs(t, err, &esc)
	assert.Equal(t, link, esc.Link)
	assert.NotContains(t, err.Error(), "outside/secret", "the error never names where the link points")
}

func TestContain(t *testing.T) {
	t.Run("a file link out of the root is refused", func(t *testing.T) {
		root, _, secret := tree(t)
		link := filepath.Join(root, "docs", "fr.txt")
		require.NoError(t, os.Symlink(secret, link))
		_, err := rootpath.Contain(root, link)
		requireEscape(t, err, "docs/fr.txt")
	})

	t.Run("a new file beneath a directory link out of the root is refused", func(t *testing.T) {
		root, outside, _ := tree(t)
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "locales")))
		_, err := rootpath.Contain(root, filepath.Join(root, "locales", "fr", "new.json"))
		requireEscape(t, err, "locales")
	})

	t.Run("a link that stays inside the root is allowed", func(t *testing.T) {
		root, _, _ := tree(t)
		target := filepath.Join(root, "docs", "en.txt")
		require.NoError(t, os.WriteFile(target, []byte("en"), 0o644))
		link := filepath.Join(root, "docs", "fr.txt")
		require.NoError(t, os.Symlink("en.txt", link))
		real, err := rootpath.Contain(root, link)
		require.NoError(t, err)
		want, _ := filepath.EvalSymlinks(target)
		assert.Equal(t, want, real)
	})

	t.Run("a root reached through a link is allowed", func(t *testing.T) {
		root, _, _ := tree(t)
		via := filepath.Join(t.TempDir(), "via")
		require.NoError(t, os.Symlink(root, via))
		_, err := rootpath.Contain(via, filepath.Join(via, "docs", "new.txt"))
		require.NoError(t, err)
	})

	t.Run("a link to a missing file is refused", func(t *testing.T) {
		root, outside, _ := tree(t)
		link := filepath.Join(root, "docs", "keys")
		require.NoError(t, os.Symlink(filepath.Join(outside, "missing"), link))
		_, err := rootpath.Contain(root, link)
		requireEscape(t, err, "docs/keys")
	})

	t.Run("a new file in a real directory is allowed", func(t *testing.T) {
		root, _, _ := tree(t)
		_, err := rootpath.Contain(root, filepath.Join(root, "docs", "a", "b.txt"))
		require.NoError(t, err)
	})
}

func TestOpenRegular(t *testing.T) {
	t.Run("a file link out of the root is never opened", func(t *testing.T) {
		root, _, secret := tree(t)
		link := filepath.Join(root, "docs", "readme.md")
		require.NoError(t, os.Symlink(secret, link))
		_, err := rootpath.OpenRegular(root, link)
		requireEscape(t, err, "docs/readme.md")
	})

	t.Run("a FIFO is refused without blocking", func(t *testing.T) {
		root, _, _ := tree(t)
		fifo := filepath.Join(root, "docs", "pipe.md")
		require.NoError(t, syscall.Mkfifo(fifo, 0o600))
		_, err := rootpath.OpenRegular(root, fifo)
		require.ErrorIs(t, err, rootpath.ErrNotRegular)
	})

	t.Run("a regular file inside the root is read", func(t *testing.T) {
		root, _, _ := tree(t)
		p := filepath.Join(root, "docs", "en.md")
		require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))
		f, err := rootpath.OpenRegular(root, p)
		require.NoError(t, err)
		defer f.Close()
		b, err := io.ReadAll(f)
		require.NoError(t, err)
		assert.Equal(t, "hello", string(b))
	})
}

// An absolute link that leads inside the root is followed by a read, as
// Contain follows it.
func TestAnAbsoluteLinkInsideTheRoot(t *testing.T) {
	root, _, _ := tree(t)
	real := filepath.Join(root, "docs", "en.md")
	require.NoError(t, os.WriteFile(real, []byte("hello"), 0o644))
	link := filepath.Join(root, "docs", "abs.md")
	require.NoError(t, os.Symlink(real, link))

	f, err := rootpath.OpenRegular(root, link)
	require.NoError(t, err)
	b, _ := io.ReadAll(f)
	require.NoError(t, f.Close())
	assert.Equal(t, "hello", string(b))
}
