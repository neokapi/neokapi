//go:build !windows

package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stageDesktopPlugin writes a minimal plugin named name under root, with a
// marker file saying which copy it is.
func stageDesktopPlugin(t *testing.T, root, name, marker string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"),
		[]byte(`{"manifest_version":"1","plugin":"`+name+`","version":"1.0.0","binary":"kapi-`+name+`"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi-"+name), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "copy"), []byte(marker), 0o644))
}

// privateDir is a temporary directory no other user can write.
func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	return dir
}

// On first start, the plugins an earlier desktop installed under its config
// directory move into the shared root once. A plugin the shared root already
// holds keeps that copy. The old cache file and the empty directory go.
func TestMigrateLegacyPlugins_MovesThreePluginsOnce(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", privateDir(t))
	legacy := legacyPluginDir()
	for _, name := range []string{"bowrain", "okapi", "okapi-bridge"} {
		stageDesktopPlugin(t, legacy, name, "legacy")
	}
	require.NoError(t, os.WriteFile(filepath.Join(legacy, legacyCacheFile), []byte("{}"), 0o644))

	app := NewApp()
	app.pluginDir = filepath.Join(privateDir(t), "plugins")
	stageDesktopPlugin(t, app.pluginDir, "okapi", "shared")

	moved := app.migrateLegacyPlugins()
	assert.Equal(t, []string{"bowrain", "okapi-bridge"}, moved)
	for _, name := range []string{"bowrain", "okapi", "okapi-bridge"} {
		assert.FileExists(t, filepath.Join(app.pluginDir, name, "kapi-"+name))
	}
	got, err := os.ReadFile(filepath.Join(app.pluginDir, "okapi", "copy"))
	require.NoError(t, err)
	assert.Equal(t, "shared", string(got), "the shared copy wins")
	assert.NoDirExists(t, legacy, "the old directory, cache file included, is gone")

	assert.Empty(t, app.migrateLegacyPlugins(), "a second start moves nothing")
}

// A legacy plugin that fails the install checks stays where it is.
func TestMigrateLegacyPlugins_LeavesARefusedPlugin(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", privateDir(t))
	legacy := legacyPluginDir()
	stageDesktopPlugin(t, legacy, "bowrain", "legacy")
	require.NoError(t, os.Chmod(filepath.Join(legacy, "bowrain"), 0o777))

	app := NewApp()
	app.pluginDir = filepath.Join(privateDir(t), "plugins")
	assert.Empty(t, app.migrateLegacyPlugins())
	assert.DirExists(t, filepath.Join(legacy, "bowrain"))
	assert.NoDirExists(t, filepath.Join(app.pluginDir, "bowrain"))
}

// When the old directory is a link to the shared one, the copy "there" is the
// plugin just installed, and nothing is removed.
func TestRemoveLegacyPlugin_LegacyLinkedToShared(t *testing.T) {
	config := privateDir(t)
	t.Setenv("KAPI_CONFIG_DIR", config)
	shared := filepath.Join(privateDir(t), "plugins")
	stageDesktopPlugin(t, shared, "vision", "shared")
	require.NoError(t, os.Symlink(shared, legacyPluginDir()))

	app := NewApp()
	app.pluginDir = shared
	app.removeLegacyPlugin("vision")
	assert.FileExists(t, filepath.Join(shared, "vision", "kapi-vision"), "the installed plugin stays")
	assert.Empty(t, app.migrateLegacyPlugins())
	assert.FileExists(t, filepath.Join(shared, "vision", "kapi-vision"))
}

// Every change to the plugin set refreshes the media engines: a removal from
// the Plugins page here, through the one function the installs, updates and
// the watcher also call.
func TestRemovePlugin_RefreshesTheMediaEngines(t *testing.T) {
	root := privateDir(t)
	t.Setenv("KAPI_PLUGINS_DIR", root)
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	stageDesktopPlugin(t, root, "demo", "shared")
	refreshes := 0
	real := refreshMediaEngines
	refreshMediaEngines = func() { refreshes++ }
	t.Cleanup(func() { refreshMediaEngines = real })

	app := NewApp()
	app.rescanPlugins()
	require.NotNil(t, app.host().Plugin("demo"))
	require.NoError(t, app.RemovePlugin("demo"))
	assert.Equal(t, 1, refreshes)
	assert.NoDirExists(t, filepath.Join(root, "demo"))

	app.pluginsChanged(false) // the watcher's path
	assert.Equal(t, 2, refreshes)
}
