//go:build unix

package dataroot

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolate gives the test a data root of its own and forgets what earlier
// tests resolved.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(EnvDir, t.TempDir())
	followed.Clear()
	t.Cleanup(followed.Clear)
}

// One directory typed two ways, on a volume that does not tell the spellings
// apart, is one checkout with one state directory.
func TestOneDirectoryTypedTwoWaysIsOneCheckout(t *testing.T) {
	isolate(t)
	upper := filepath.Join(t.TempDir(), "V4")
	require.NoError(t, os.Mkdir(upper, 0o755))
	lower := filepath.Join(filepath.Dir(upper), "v4")
	if _, err := os.Stat(lower); err != nil {
		t.Skip("this volume tells V4 and v4 apart, so they are two directories")
	}
	dir, _, err := EnsureCheckoutDir(upper)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vault.json"), []byte("{}"), 0o600))

	followed.Clear()
	assert.Equal(t, dir, CheckoutDir(lower))
	again, earlier, err := EnsureCheckoutDir(lower)
	require.NoError(t, err)
	assert.Equal(t, dir, again)
	assert.Empty(t, earlier)
	assert.Len(t, Checkouts(), 1, "one record")
}

// Where the file system records no birth time, a rename is not followed: a new
// directory that reuses a deleted checkout's inode would look the same.
func TestARenameIsNotFollowedWithoutABirthTime(t *testing.T) {
	isolate(t)
	prev := birthTimeOf
	birthTimeOf = func(string, *syscall.Stat_t) string { return "" }
	t.Cleanup(func() { birthTimeOf = prev })

	a := filepath.Join(t.TempDir(), "a")
	require.NoError(t, os.Mkdir(a, 0o755))
	dir, _, err := EnsureCheckoutDir(a)
	require.NoError(t, err)
	b := filepath.Join(t.TempDir(), "b")
	require.NoError(t, os.Rename(a, b))

	followed.Clear()
	assert.NotEqual(t, dir, CheckoutDir(b), "the state does not follow")
	assert.DirExists(t, dir, "and stays where it was, for a relink")

	birthTimeOf = prev
	followed.Clear()
	c := filepath.Join(t.TempDir(), "c")
	require.NoError(t, os.Mkdir(c, 0o755))
	cdir, _, err := EnsureCheckoutDir(c)
	require.NoError(t, err)
	d := filepath.Join(t.TempDir(), "d")
	require.NoError(t, os.Rename(c, d))
	followed.Clear()
	if strings.HasSuffix(mustIdentity(t, d), ":") {
		t.Skip("this file system records no birth time")
	}
	assert.Equal(t, filepath.Join(CheckoutsDir(), checkoutKey(NormalizePath(d), mustIdentity(t, d))), CheckoutDir(d))
	assert.NoDirExists(t, cdir, "with a birth time, the state follows the rename")
}

func mustIdentity(t *testing.T, p string) string {
	t.Helper()
	id, err := identity(NormalizePath(p))
	require.NoError(t, err)
	return id
}

// A state directory with no record, as a CI cache restores one into the
// directory `kapi state path` named, is the checkout's by its key, and gets
// its record on first use.
func TestARestoredStateDirectoryGetsItsRecord(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	dir := CheckoutDir(root)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "work", "cache", "docs"), 0o700))

	got, _, err := EnsureCheckoutDir(root)
	require.NoError(t, err)
	assert.Equal(t, dir, got)
	rec, err := ReadRecord(dir)
	require.NoError(t, err)
	assert.Equal(t, NormalizePath(root), rec.Path)
	assert.DirExists(t, filepath.Join(dir, "work", "cache", "docs"), "what was restored is kept")
}
