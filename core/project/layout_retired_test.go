package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsureLayout_SweepsRetiredStateFiles pins the footgun-kill: the retired
// tm.db / termbase.db (and their WAL/SHM sidecars) are removed on project
// open, while the live stores are untouched.
func TestEnsureLayout_SweepsRetiredStateFiles(t *testing.T) {
	root := t.TempDir()
	layout := Layout{Root: root, StateDir: filepath.Join(root, StateDirName)}
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))

	for _, name := range []string{"tm.db", "tm.db-wal", "termbase.db", MemoryFileName, TermsFileName} {
		require.NoError(t, os.WriteFile(filepath.Join(layout.StateDir, name), []byte("x"), 0o644))
	}

	require.NoError(t, EnsureLayout(layout))

	for _, gone := range []string{"tm.db", "tm.db-wal", "termbase.db"} {
		_, err := os.Stat(filepath.Join(layout.StateDir, gone))
		assert.True(t, os.IsNotExist(err), "%s must be swept", gone)
	}
	for _, kept := range []string{MemoryFileName, TermsFileName} {
		_, err := os.Stat(filepath.Join(layout.StateDir, kept))
		assert.NoError(t, err, "%s must survive", kept)
	}
}
