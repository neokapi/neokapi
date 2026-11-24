//go:build windows

package filelock_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage/filelock"
)

// TestSweep_LeavesAFileAnotherHandleHoldsOpen pins what the sweep relies on
// under Windows, where it gives the lock back before it removes the file:
// Windows refuses to remove a file another handle holds open without
// FILE_SHARE_DELETE, which Go's os.Open never asks for. A stale lock file a
// locker has opened, holding the lock or waiting for it, stays; once the
// handle is closed, the next sweep removes it.
func TestSweep_LeavesAFileAnotherHandleHoldsOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.lock")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))

	open, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	n, _, err := filelock.Sweep(dir, 24*time.Hour, 0, 100)
	require.NoError(t, err)
	assert.Zero(t, n, "a file another handle holds open is not removed")
	assert.FileExists(t, path)

	require.NoError(t, open.Close())
	n, _, err = filelock.Sweep(dir, 24*time.Hour, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "once the handle is closed, the sweep removes it")
	assert.NoFileExists(t, path)
}
