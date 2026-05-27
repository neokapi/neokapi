package pluginhost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveInstalledFrom(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "okapi-bridge")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte("{}"), 0o644))

	// Removes the plugin from the given dir.
	require.NoError(t, pluginhost.RemoveInstalledFrom(dir, "okapi-bridge"))
	_, err := os.Stat(pluginDir)
	assert.True(t, os.IsNotExist(err), "plugin dir should be gone")

	// A plugin absent from the given dir reports "not installed" (mentioning the dir),
	// not a different default location — this is the bug behind the desktop Uninstall
	// failure (install dir kapiConfigDir()/plugins ≠ remove dir InstallTarget()).
	err = pluginhost.RemoveInstalledFrom(dir, "okapi-bridge")
	require.Error(t, err)
	assert.Contains(t, err.Error(), dir)
}
