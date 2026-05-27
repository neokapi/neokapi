package pluginhost_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Host.Remove is the single uninstall path: it deletes a plugin from the exact
// directory it was discovered in (Plugin.Dir), so install, discovery, and
// removal can never disagree on the location (regression for the desktop
// uninstall-dir mismatch, #741).
func TestHostRemove(t *testing.T) {
	mkPluginDir := func(t *testing.T) string {
		t.Helper()
		dir := filepath.Join(t.TempDir(), "okapi-bridge")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o644))
		return dir
	}
	plugin := func(dir string, order int) *pluginhost.Plugin {
		return &pluginhost.Plugin{
			Dir:      dir,
			Source:   pluginhost.Source{Order: order, Label: dir, Path: dir},
			Manifest: &manifest.Manifest{Plugin: "okapi-bridge", Version: "1.0.0"},
		}
	}

	t.Run("removes from its own dir and drops from the host", func(t *testing.T) {
		dir := mkPluginDir(t)
		h := pluginhost.NewHost([]*pluginhost.Plugin{plugin(dir, 1)}, nil)
		require.NoError(t, h.Remove("okapi-bridge"))
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "plugin dir should be deleted")
		assert.Nil(t, h.Plugin("okapi-bridge"), "plugin should be dropped from the host")
	})

	t.Run("not installed", func(t *testing.T) {
		assert.Error(t, pluginhost.NewHost(nil, nil).Remove("nope"))
	})

	t.Run("refuses system installs, leaving files intact", func(t *testing.T) {
		dir := mkPluginDir(t)
		h := pluginhost.NewHost([]*pluginhost.Plugin{plugin(dir, 3)}, nil)
		err := h.Remove("okapi-bridge")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "system install")
		_, statErr := os.Stat(dir)
		assert.NoError(t, statErr, "system-install files must be left intact")
	})
}
