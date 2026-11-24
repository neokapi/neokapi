//go:build unix

package dataroot

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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

// Writers of one checkout's record writing at once leave a whole record each
// time, and no temporary file behind. An edit can be lost to a concurrent
// writer (updateRecord takes no lock), so the test asserts the record holds
// only fields one of the writers wrote.
func TestConcurrentRecordWritesLeaveAWholeRecord(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	dir, _, err := EnsureCheckoutDir(root)
	require.NoError(t, err)

	const n = 24
	done := make(chan error, n+1)
	for i := range n {
		go func() {
			_, err := MarkTold(dir, "notice-"+strconv.Itoa(i))
			done <- err
		}()
	}
	go func() { done <- SetRecordProject(dir, CheckoutProject{ID: "prj_x", Name: "x"}) }()
	for range n + 1 {
		require.NoError(t, <-done)
	}
	rec, err := ReadRecord(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, rec.Told, "the record is whole and holds notices")
	for name := range rec.Told {
		assert.True(t, strings.HasPrefix(name, "notice-"), name)
	}
	if rec.Project != nil {
		assert.Equal(t, "prj_x", rec.Project.ID)
	}
	assert.Equal(t, NormalizePath(root), rec.Path)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"), "a temporary file was left: %s", e.Name())
	}
}

// A record's temporary file a stopped write left is removed when the record is
// next written, once it is a day old; a fresh one may be another writer's.
func TestARecordWriteRemovesStaleTemporaryFiles(t *testing.T) {
	isolate(t)
	dir, _, err := EnsureCheckoutDir(t.TempDir())
	require.NoError(t, err)
	stale := filepath.Join(dir, ".checkout.json.111.tmp")
	fresh := filepath.Join(dir, ".checkout.json.222.tmp")
	for _, p := range []string{stale, fresh} {
		require.NoError(t, os.WriteFile(p, []byte("{"), 0o600))
	}
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))
	_, err = MarkTold(dir, "n")
	require.NoError(t, err)
	assert.NoFileExists(t, stale)
	assert.FileExists(t, fresh)
	assert.True(t, IsRecordTemp(".checkout.json.222.tmp"))
	assert.False(t, IsRecordTemp("checkout.json"))
}
