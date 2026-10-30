//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShare_FreshFolder drives `kapi share` on a folder kapi has not been run
// in: the ladder, then --via folder writing the recipe and syncing, then a
// plain share syncing again with the recipe kept.
func TestShare_FreshFolder(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("# Guide\n\nWe utilise the widget.\n"), 0o644))
	shared, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// The isolation env sets KAPI_NO_PROJECT, so the folder is named with -p.
	recipePath := filepath.Join(dir, "kapi.yaml")

	ladder := kapiIn(t, dir, "share", "-p", recipePath)
	assert.True(t, strings.HasPrefix(ladder, "How far your rules reach\n"), ladder)
	assert.Contains(t, ladder, "With your team       kapi share --via git | folder <path>")
	assert.NoFileExists(t, filepath.Join(dir, "kapi.yaml"))

	first := kapiIn(t, dir, "share", "-p", recipePath, "--via", "folder", shared)
	assert.True(t, strings.HasPrefix(first, "kapi.yaml  created\nWith your team · through a shared folder ("+shared+")\n"), first)
	recipe, err := os.ReadFile(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(recipe), "context:\n  backend: file\n  path: "+shared+"\n")

	again := kapiIn(t, dir, "share", "-p", recipePath, "--via", "folder", shared)
	assert.True(t, strings.HasPrefix(again, "kapi.yaml  kept\n"), again)

	synced := kapiIn(t, dir, "share", "-p", recipePath)
	assert.Equal(t, "With your team · through a shared folder ("+shared+")\n"+
		"  pushed none · pulled none · last sync just now\n", synced)
}
