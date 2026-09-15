package pluginattach

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A source-connector route that runs over a daemon answers --help and refuses a
// flag it does not take before any daemon is acquired. The plugin here has no
// binary, so a route that tried to reach its daemon fails to start one.
func TestSourceConnectorRouteHelp(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Chdir(t.TempDir())

	const name = "connectorhelp"
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
				Commands: []manifest.Command{
					{Name: "push", Short: "Upload local changes to the server"},
					{Name: "pull", Short: "Download translations from the server"},
				},
			},
		},
	}
	pluginhost.RegisterSourceConnectorDispatcher(pluginhost.NewGenericSourceConnectorDispatcher(name), pluginhost.SourceConnectorOpsClaimed...)
	pool := pluginhost.NewDaemonPool(pluginhost.DaemonPoolOptions{MaxDaemons: 1})
	t.Cleanup(pool.Shutdown)
	host := pluginhost.NewHost([]*pluginhost.Plugin{plugin}, nil)

	execute := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		root := &cobra.Command{Use: "kapi", SilenceUsage: true, SilenceErrors: true}
		AttachCommandsWithOptions(root, host, AttachOptions{DaemonPool: pool})
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		return out.String(), err
	}

	t.Run("must fail: push --help prints the route's help", func(t *testing.T) {
		out, err := execute(t, "push", "--help")
		require.NoError(t, err)
		assert.Contains(t, out, "Upload local changes to the server")
		assert.Contains(t, out, "kapi push [paths...] [flags]")
		for _, flag := range []string{"--force", "--dry-run", "-p, --project"} {
			assert.Contains(t, out, flag)
		}
	})

	t.Run("must fail: pull -h prints the route's help", func(t *testing.T) {
		out, err := execute(t, "pull", "-h")
		require.NoError(t, err)
		assert.Contains(t, out, "Download translations from the server")
		assert.Contains(t, out, "--locale")
	})

	t.Run("must fail: the group spelling prints the same help", func(t *testing.T) {
		out, err := execute(t, name, "push", "--help")
		require.NoError(t, err)
		assert.Contains(t, out, "kapi "+name+" push [paths...] [flags]")
		assert.Contains(t, out, "--dry-run")
	})

	t.Run("must fail: an unknown flag is refused before a daemon starts", func(t *testing.T) {
		_, err := execute(t, "push", "--concepts")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--concepts")
		assert.NotContains(t, err.Error(), "acquire daemon")
	})
}
