package runtimedirs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCreatesAMissingDirectoryAndLeavesNothingInIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blobs")
	require.NoError(t, Check(Dir{Purpose: "blob", Env: "BLOB_STORAGE_LOCAL_DIR", Path: dir}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file is removed")
}

func TestCheckNamesEveryDirectoryThatIsNotWritable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only directory is writable for root and on Windows")
	}
	locked := t.TempDir()
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	err := Check(
		Dir{Purpose: "temporary", Env: "TMPDIR", Path: locked},
		Dir{Purpose: "blob", Env: "BLOB_STORAGE_LOCAL_DIR", Path: filepath.Join(locked, "blobs")},
		Dir{Purpose: "fine", Env: "FINE_DIR", Path: t.TempDir()},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "temporary directory "+locked+" (set by TMPDIR) is not writable")
	assert.Contains(t, err.Error(), "blob directory "+filepath.Join(locked, "blobs")+" (set by BLOB_STORAGE_LOCAL_DIR)")
	assert.NotContains(t, err.Error(), "FINE_DIR")
}

func TestCheckRefusesAnUnsetDirectory(t *testing.T) {
	err := Check(Dir{Purpose: "blob", Env: "BLOB_STORAGE_LOCAL_DIR"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no directory is set")
}

func TestTempFollowsTMPDIR(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	assert.Equal(t, Dir{Purpose: "temporary", Env: "TMPDIR", Path: dir}, Temp())
}
