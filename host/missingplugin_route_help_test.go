package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// A verb run right after the missing-plugin install goes to the plugin's daemon
// route outside cobra, so the route's help and its flag refusal happen here
// too. The plugin has no binary, so a route that tried to reach its daemon
// fails to start one.
func TestDispatchPluginVerb_RouteHelp(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	const name = "verbhelp"
	plugin := &pluginhost.Plugin{
		Dir:        t.TempDir(),
		BinaryPath: filepath.Join(t.TempDir(), "absent"),
		Manifest: &manifest.Manifest{
			ManifestVersion: "1",
			Plugin:          name,
			Version:         "0.0.0",
			Binary:          "absent",
			Daemon:          &manifest.DaemonConfig{StartupTimeoutSeconds: 1},
			Capabilities: manifest.Capabilities{
				SourceConnectors: []manifest.SourceConnector{{ID: name + "-source"}},
				Commands:         []manifest.Command{{Name: "push", Short: "Upload local changes to the server"}},
			},
		},
	}
	pluginhost.RegisterSourceConnectorDispatcher(pluginhost.NewGenericSourceConnectorDispatcher(name), pluginhost.SourceConnectorOpsClaimed...)
	a := &App{Quiet: true, PluginHost: pluginhost.NewHost([]*pluginhost.Plugin{plugin}, nil)}
	t.Cleanup(func() { a.DaemonPool().Shutdown() })

	dispatch := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		r, w, err := os.Pipe()
		require.NoError(t, err)
		stdout := os.Stdout
		os.Stdout = w
		runErr := a.dispatchPluginVerb(context.Background(), "push", args)
		os.Stdout = stdout
		require.NoError(t, w.Close())
		out, err := io.ReadAll(r)
		require.NoError(t, err)
		return string(out), runErr
	}

	t.Run("must fail: push --help prints the route's help", func(t *testing.T) {
		out, err := dispatch(t, "--help")
		require.NoError(t, err)
		assert.Contains(t, out, "Upload local changes to the server")
		assert.Contains(t, out, "kapi push [paths...] [flags]")
		assert.Contains(t, out, "--dry-run")
	})

	t.Run("must fail: an unknown flag is refused before a daemon starts", func(t *testing.T) {
		_, err := dispatch(t, "--concepts")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--concepts")
		assert.NotContains(t, err.Error(), "acquire daemon")
	})
}
