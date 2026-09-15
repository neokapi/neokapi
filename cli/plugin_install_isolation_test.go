package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An isolated kapi confines plugin discovery to $KAPI_PLUGINS_DIR
// (KAPI_PLUGINS_DIR_ONLY). These tests pin that `kapi plugin install`, `update`
// and `remove` act on the directory discovery reads, that an empty
// $KAPI_PLUGINS_DIR is refused, and that nothing changes without the flag.

// runPluginSubcommand drives `kapi plugin <args...>` through the parent plugin
// command and returns the captured output and the RunE error.
func runPluginSubcommand(t *testing.T, app *App, args ...string) (stdout, stderr bytes.Buffer, err error) {
	t.Helper()
	cmd := NewPluginCmd(app)
	cmd.SetContext(context.Background())
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return stdout, stderr, err
}

func TestPluginInstall_OnlyEnvDir_InstallsWhereDiscoveryReads(t *testing.T) {
	dataHome := withIsolatedXDG(t)
	pluginsDir := t.TempDir()
	// A list: the install lands in the first entry.
	t.Setenv("KAPI_PLUGINS_DIR", pluginsDir+string(os.PathListSeparator)+t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	server, _ := servePluginRegistry(t, "demo", map[string]string{"1.0.0": "stable", "1.1.0": "stable"})
	index := server.URL + "/plugins.json"

	stdout, stderr, err := runPluginSubcommand(t, &App{}, "install", "demo@1.0.0", "--index", index, "--unsafe")
	require.NoErrorf(t, err, "install failed; stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), filepath.Join(pluginsDir, "demo"))

	plugins := pluginhost.Discover(pluginhost.DiscoverOptions{EnvPluginsDir: os.Getenv("KAPI_PLUGINS_DIR")})
	require.Len(t, plugins, 1, "discovery under KAPI_PLUGINS_DIR_ONLY sees the plugin just installed")
	assert.Equal(t, filepath.Join(pluginsDir, "demo"), plugins[0].Dir)
	assert.NoDirExists(t, filepath.Join(dataHome, "kapi", "plugins", "demo"))

	stdout, stderr, err = runPluginSubcommand(t, &App{}, "update", "demo", "--index", index, "--constraint", "^1.0.0", "--unsafe")
	require.NoErrorf(t, err, "update failed; stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "Updated demo 1.0.0 → 1.1.0")
	meta, err := pluginhost.ReadInstalledMetadata(filepath.Join(pluginsDir, "demo"))
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", meta.Version)

	stdout, stderr, err = runPluginSubcommand(t, &App{}, "remove", "demo")
	require.NoErrorf(t, err, "remove failed; stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "Removed demo")
	assert.NoDirExists(t, filepath.Join(pluginsDir, "demo"))
}

func TestPluginInstall_OnlyEnvDir_FollowsPluginDirFlag(t *testing.T) {
	dataHome := withIsolatedXDG(t)
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	server, _ := servePluginRegistry(t, "demo", map[string]string{"1.0.0": "stable", "1.1.0": "stable"})
	index := server.URL + "/plugins.json"

	// --plugin-dir replaces $KAPI_PLUGINS_DIR for discovery, so it is the
	// directory an isolated install and update write to.
	flagDir := t.TempDir()
	app := &App{}
	app.PluginDir = flagDir
	_, stderr, err := runPluginSubcommand(t, app, "install", "demo@1.0.0", "--index", index, "--unsafe")
	require.NoErrorf(t, err, "install failed; stderr=%s", stderr.String())

	assert.DirExists(t, filepath.Join(flagDir, "demo"))
	assert.NoDirExists(t, filepath.Join(os.Getenv("KAPI_PLUGINS_DIR"), "demo"))
	assert.NoDirExists(t, filepath.Join(dataHome, "kapi", "plugins", "demo"))

	stdout, stderr, err := runPluginSubcommand(t, app, "update", "demo", "--index", index, "--constraint", "^1.0.0", "--unsafe")
	require.NoErrorf(t, err, "update failed; stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "Updated demo 1.0.0 → 1.1.0")
	meta, err := pluginhost.ReadInstalledMetadata(filepath.Join(flagDir, "demo"))
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", meta.Version)
	assert.NoDirExists(t, filepath.Join(os.Getenv("KAPI_PLUGINS_DIR"), "demo"))
}

func TestPluginInstall_OnlyEnvDir_EmptyPluginsDirRefuses(t *testing.T) {
	server, _ := servePluginRegistry(t, "demo", map[string]string{"1.0.0": "stable"})
	index := server.URL + "/plugins.json"

	for _, tc := range []struct {
		name       string
		pluginsDir string
		args       []string
	}{
		{"install", "", []string{"install", "demo", "--index", index, "--unsafe"}},
		{"update", "", []string{"update", "demo", "--index", index, "--unsafe"}},
		{"install with only list separators", string(os.PathListSeparator), []string{"install", "demo", "--index", index, "--unsafe"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataHome := withIsolatedXDG(t)
			t.Setenv("KAPI_PLUGINS_DIR", tc.pluginsDir)
			t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")

			_, _, err := runPluginSubcommand(t, &App{}, tc.args...)
			require.Error(t, err, "discovery would never see a plugin installed outside $KAPI_PLUGINS_DIR")
			assert.Contains(t, err.Error(), "KAPI_PLUGINS_DIR_ONLY")
			assert.Contains(t, err.Error(), "KAPI_PLUGINS_DIR is empty")
			assert.NoDirExists(t, filepath.Join(dataHome, "kapi", "plugins", "demo"), "no fallback to the data dir")
		})
	}
}

func TestPluginInstall_WithoutOnlyEnvDir_TargetUnchanged(t *testing.T) {
	server, _ := servePluginRegistry(t, "demo", map[string]string{"1.0.0": "stable"})
	index := server.URL + "/plugins.json"

	for _, tc := range []struct {
		name          string
		setPluginsDir bool
	}{
		{"no plugins dir", false},
		// $KAPI_PLUGINS_DIR on its own adds a discovery root and leaves the
		// install target at the data dir.
		{"plugins dir set on its own", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataHome := withIsolatedXDG(t)
			t.Setenv("KAPI_PLUGINS_DIR_ONLY", "")
			pluginsDir := t.TempDir()
			if tc.setPluginsDir {
				t.Setenv("KAPI_PLUGINS_DIR", pluginsDir)
			}

			stdout, stderr, err := runPluginSubcommand(t, &App{}, "install", "demo", "--index", index, "--unsafe")
			require.NoErrorf(t, err, "install failed; stderr=%s", stderr.String())
			want := filepath.Join(dataHome, "kapi", "plugins", "demo")
			assert.Contains(t, stdout.String(), want)
			assert.DirExists(t, want)
			assert.NoDirExists(t, filepath.Join(pluginsDir, "demo"))
		})
	}
}

func TestInstallFromRegistry_OnlyEnvDir_ExplicitTargetDirUnaffected(t *testing.T) {
	// Kapi Desktop passes its plugin dir as TargetDir, so the environment never
	// chooses its install target.
	withIsolatedXDG(t)
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	server, _ := servePluginRegistry(t, "demo", map[string]string{"1.0.0": "stable"})

	target := t.TempDir()
	res, err := InstallPluginFromRegistry(context.Background(), pluginhost.InstallOptions{
		IndexURL:   server.URL + "/plugins.json",
		PluginName: "demo",
		TargetDir:  target,
		Unsafe:     true,
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(target, "demo"), res.InstallDir)
}
