//go:build unix

package dataroot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCacheEnv = "KAPI_TEST_CACHE_DIR"

// Each condition PrepareCacheDir holds a named directory to is refused with a
// message that names the condition and the path.
func TestPrepareCacheDir_Refusals(t *testing.T) {
	checkout := t.TempDir()
	for _, tc := range []struct {
		name string
		dir  func(t *testing.T) string
		says string
	}{
		{"relative", func(*testing.T) string { return "parse" }, "must be an absolute path"},
		{"inside the checkout", func(*testing.T) string { return filepath.Join(checkout, ".cache") }, "inside the checkout"},
		{"a link", func(t *testing.T) string {
			link := filepath.Join(t.TempDir(), "parse")
			require.NoError(t, os.Symlink(t.TempDir(), link))
			return link
		}, "is a symbolic link"},
		{"a file", func(t *testing.T) string {
			f := filepath.Join(t.TempDir(), "parse")
			require.NoError(t, os.WriteFile(f, nil, 0o600))
			return f
		}, "is not a directory"},
		{"open to the group", func(t *testing.T) string {
			d := filepath.Join(t.TempDir(), "parse")
			require.NoError(t, os.Mkdir(d, 0o700))
			require.NoError(t, os.Chmod(d, 0o750))
			return d
		}, "gives group or other users access"},
		{"open to others", func(t *testing.T) string {
			d := filepath.Join(t.TempDir(), "parse")
			require.NoError(t, os.Mkdir(d, 0o700))
			require.NoError(t, os.Chmod(d, 0o703))
			return d
		}, "gives group or other users access"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.dir(t)
			_, err := PrepareCacheDir(testCacheEnv, dir, checkout)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.Contains(t, err.Error(), testCacheEnv)
		})
	}
}

// A directory someone else prepared is refused and left as it was.
func TestPrepareCacheDir_DoesNotRepairADirectory(t *testing.T) {
	d := filepath.Join(t.TempDir(), "parse")
	require.NoError(t, os.Mkdir(d, 0o700))
	require.NoError(t, os.Chmod(d, 0o755))
	_, err := PrepareCacheDir(testCacheEnv, d, "")
	require.Error(t, err)
	st, err := os.Stat(d)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), st.Mode().Perm())
}

// A directory kapi creates is private, and a private one is accepted.
func TestPrepareCacheDir_CreatesAndAcceptsAPrivateDirectory(t *testing.T) {
	d := filepath.Join(t.TempDir(), "a", "parse")
	got, err := PrepareCacheDir(testCacheEnv, d, "")
	require.NoError(t, err)
	assert.Equal(t, d, got)
	st, err := os.Stat(d)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), st.Mode().Perm())
	_, err = PrepareCacheDir(testCacheEnv, d, "")
	require.NoError(t, err, "an existing private directory is accepted")
}

// A test binary keeps the directory beneath the temporary directory.
func TestPrepareCacheDir_StaysInTheTemporaryDirectoryUnderTest(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	outside := filepath.Join(wd, "testdata-cache-never-created")
	_, err = PrepareCacheDir(testCacheEnv, outside, "")
	require.Error(t, err)
	assert.NoDirExists(t, outside)
}
