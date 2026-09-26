package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/cli"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/venue/project"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every plugin command that reads a recipe resolves it the way kapi's own
// verbs do: -p, then KAPI_NO_PROJECT, then KAPI_PROJECT, then the upward walk.
// A plugin route that walked from the working directory on its own once pushed
// this repository's dogfood recipe to production from a run that named another
// project, so these tests stand a connected project in the working directory,
// point it at a server that counts requests, and require that server to hear
// nothing.

// countingServer answers every request with 500 and counts them.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// connectedProject writes a project connected to srv and returns its root.
func connectedProject(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	_, err = project.InitProject(root, &project.Recipe{
		Version:  coreproj.CurrentVersion,
		Name:     "Connected",
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Server:   &project.ServerSpec{URL: srv.URL + "/acme/proj-1", Stream: "main"},
	})
	require.NoError(t, err)
	return root
}

// runPluginCommand runs args through a fresh root carrying the plugin's
// command tree, the way `kapi-bowrain command …` does. The commands are package
// state, so the -p and --help a run sets are cleared once the test ends.
func runPluginCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "command", SilenceUsage: true, SilenceErrors: true}
	cli.AddPersistentFlags(&cli.App{}, root)
	for _, c := range []*cobra.Command{pushCmd, pullCmd, serverStatusCmd, serverLsCmd, diffCmd, streamCmd, authCmd, upCmd} {
		root.AddCommand(c)
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	t.Cleanup(func() {
		for _, c := range []*cobra.Command{pushCmd, pullCmd, serverStatusCmd, serverLsCmd, diffCmd, streamCmd, streamListCmd, streamStatusCmd, authCmd, authTokenCmd, authTokenListCmd, upCmd} {
			for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
				_ = fs.Set("project", "")
				_ = fs.Set("help", "false")
			}
		}
	})
	err := root.Execute()
	return out.String(), err
}

// pluginRoutes are the plugin commands that act on a project.
var pluginRoutes = [][]string{
	{"push"},
	{"pull"},
	{"server-status", "--json"},
	{"server-ls", "--json"},
	{"diff"},
	{"stream", "list"},
	{"stream", "status"},
	{"auth", "token", "list"},
	{"server-up", "--json"},
}

func TestPluginRoutes_RefuseUnderNoProjectBeforeAnyRequest(t *testing.T) {
	isolateKapi(t)
	automationApp(t)
	srv, hits := countingServer(t)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Setenv("BOWRAIN_SERVER_URL", srv.URL)
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	t.Chdir(connectedProject(t, srv))

	for _, args := range pluginRoutes {
		t.Run(args[0]+" "+args[len(args)-1], func(t *testing.T) {
			_, err := runPluginCommand(t, args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), coreproj.NoProjectEnvVar)
		})
	}
	assert.Zero(t, hits.Load(), "a route under KAPI_NO_PROJECT reached the server of the project in the working directory")
}

func TestPluginRoutes_HelpRunsNothing(t *testing.T) {
	isolateKapi(t)
	srv, hits := countingServer(t)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Chdir(connectedProject(t, srv))

	for _, args := range pluginRoutes {
		t.Run(args[0], func(t *testing.T) {
			out, err := runPluginCommand(t, append(args[:len(args):len(args)], "--help")...)
			require.NoError(t, err)
			assert.Contains(t, out, "--project")
		})
	}
	assert.Zero(t, hits.Load())
}

// A route given -p acts on that project alone, even run from inside another.
// server-up is the route that once resolved -p for the recipe and then pushed
// the working directory's project.
func TestPluginRoutes_ExplicitProjectWinsOverTheWorkingDirectory(t *testing.T) {
	isolateKapi(t)
	automationApp(t)
	t.Setenv(coreproj.NoProjectEnvVar, "")
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Setenv("BOWRAIN_SERVER_URL", "")
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	here, hereHits := countingServer(t)
	there, thereHits := countingServer(t)
	t.Chdir(connectedProject(t, here))
	named := connectedProject(t, there)

	for _, args := range pluginRoutes {
		t.Run(args[0]+" "+args[len(args)-1], func(t *testing.T) {
			_, _ = runPluginCommand(t, append(args[:len(args):len(args)], "-p", named)...)
		})
	}
	assert.Zero(t, hereHits.Load(), "a route given -p reached the server of the project in the working directory")
	assert.NotZero(t, thereHits.Load(), "no route reached the server of the project -p named")
}

func TestRequireProject(t *testing.T) {
	isolateKapi(t)
	srv, _ := countingServer(t)
	root := connectedProject(t, srv)

	cmd := &cobra.Command{Use: "x"}
	cli.AddProjectFlag(cmd)

	_, err := requireProject(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), coreproj.NoProjectEnvVar)

	require.NoError(t, cmd.Flags().Set("project", root))
	proj, err := requireProject(cmd)
	require.NoError(t, err)
	assert.Equal(t, root, proj.Root)
}
