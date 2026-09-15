package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
)

// A source-connector route reaches a daemon that acts on a project, and push
// and pull write to a server. These tests dispatch routes to the fake daemon's
// SourceConnectorService, which records every call it receives, so each case
// shows whether the route ran, whether a daemon started, and which project the
// daemon was given.

// loggedConnectorCall is one call the fake daemon's connector recorded.
type loggedConnectorCall struct {
	RPC     string   `json:"rpc"`
	Root    string   `json:"root"`
	Paths   []string `json:"paths,omitempty"`
	Force   bool     `json:"force,omitempty"`
	DryRun  bool     `json:"dry_run,omitempty"`
	Locales []string `json:"locales,omitempty"`
}

// connectorRoute is a Mode-C plugin with the generic source-connector
// dispatcher registered for it, served by a fake daemon that logs its calls
// and its starts.
type connectorRoute struct {
	plugin   *Plugin
	pool     *DaemonPool
	callLog  string
	spawnLog string
}

var connectorRouteSeq atomic.Int32

func newConnectorRoute(t *testing.T) *connectorRoute {
	t.Helper()
	bin := buildFakeDaemon(t)
	name := fmt.Sprintf("connector%d", connectorRouteSeq.Add(1))
	logs := t.TempDir()
	r := &connectorRoute{
		plugin: makePlugin(t, name, bin, &manifest.DaemonConfig{
			StartupTimeoutSeconds: 10,
			IdleTimeoutSeconds:    300,
		}),
		pool:     NewDaemonPool(DaemonPoolOptions{MaxDaemons: 2}),
		callLog:  filepath.Join(logs, "calls.jsonl"),
		spawnLog: filepath.Join(logs, "spawns"),
	}
	t.Cleanup(r.pool.Shutdown)
	t.Setenv("FAKE_DAEMON_CONNECTOR_LOG", r.callLog)
	t.Setenv("FAKE_DAEMON_SPAWN_LOG", r.spawnLog)
	RegisterSourceConnectorDispatcher(NewGenericSourceConnectorDispatcher(name), SourceConnectorOpsClaimed...)
	return r
}

func (r *connectorRoute) dispatch(t *testing.T, op string, args ...string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return DispatchViaDaemon(ctx, r.pool, r.plugin, op, args)
}

// lines returns the lines of path, or none when nothing created it.
func lines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	require.NoError(t, sc.Err())
	return out
}

func (r *connectorRoute) calls(t *testing.T) []loggedConnectorCall {
	t.Helper()
	var calls []loggedConnectorCall
	for _, line := range lines(t, r.callLog) {
		var c loggedConnectorCall
		require.NoError(t, json.Unmarshal([]byte(line), &c))
		calls = append(calls, c)
	}
	return calls
}

// writeConnectorProject writes a kapi.yaml recipe and its state directory at
// dir and returns dir.
func writeConnectorProject(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	require.NoError(t, project.Save(filepath.Join(dir, project.RecipeFileName), &project.KapiProject{Version: "v1", Name: filepath.Base(dir)}))
	return dir
}

func TestSourceConnectorDispatch(t *testing.T) {
	// The shared fake daemon builds from this package's directory, so it is
	// built here, before any case changes the working directory.
	buildFakeDaemon(t)
	base := t.TempDir()
	proj := writeConnectorProject(t, filepath.Join(base, "proj"))
	other := writeConnectorProject(t, filepath.Join(base, "other"))
	missing := filepath.Join(base, "missing")

	for _, tc := range []struct {
		name string
		env  map[string]string
		cwd  string
		op   string
		args []string
		// want is the one call the daemon receives. When nil, the route runs
		// nothing and starts no daemon, and the dispatch returns pflag.ErrHelp
		// (help) or an error containing wantErr.
		want    *loggedConnectorCall
		help    bool
		wantErr string
	}{
		{
			name: "a push runs with its paths and flags",
			cwd:  other, op: "push", args: []string{"-p", proj, "--force", "docs/a.md"},
			want: &loggedConnectorCall{RPC: "Push", Root: proj, Paths: []string{"docs/a.md"}, Force: true},
		},
		{
			name: "a pull runs with its locales",
			cwd:  other, op: "pull", args: []string{"-p", proj, "--locale", "fr,de", "--dry-run"},
			want: &loggedConnectorCall{RPC: "Pull", Root: proj, Locales: []string{"fr", "de"}, DryRun: true},
		},
		{
			name: "ls runs with its paths",
			cwd:  other, op: "ls", args: []string{"-p", proj, "docs"},
			want: &loggedConnectorCall{RPC: "ListFiles", Root: proj, Paths: []string{"docs"}},
		},
		{
			name: "with no -p the project is the one the working directory is in",
			cwd:  proj, op: "status",
			want: &loggedConnectorCall{RPC: "Status", Root: proj},
		},
		{
			name: "-- ends the flags",
			cwd:  other, op: "push", args: []string{"-p", proj, "--", "--odd.md"},
			want: &loggedConnectorCall{RPC: "Push", Root: proj, Paths: []string{"--odd.md"}},
		},
		{
			name: "kapi's persistent flags are taken",
			cwd:  other, op: "push", args: []string{"-p", proj, "docs/a.md", "-v", "--yes", "-c", "kapi.config.yaml", "--color", "never"},
			want: &loggedConnectorCall{RPC: "Push", Root: proj, Paths: []string{"docs/a.md"}},
		},
		{
			name: "must fail: a boolean flag kapi defines leaves the path after it a path",
			cwd:  other, op: "push", args: []string{"-p", proj, "--json", "docs/a.md"},
			want: &loggedConnectorCall{RPC: "Push", Root: proj, Paths: []string{"docs/a.md"}},
		},
		{name: "must fail: push --help runs nothing", cwd: proj, op: "push", args: []string{"--help"}, help: true},
		{name: "must fail: push -h runs nothing", cwd: proj, op: "push", args: []string{"-h"}, help: true},
		{name: "must fail: pull --help runs nothing", cwd: proj, op: "pull", args: []string{"--help"}, help: true},
		{name: "must fail: pull -h runs nothing", cwd: proj, op: "pull", args: []string{"-h"}, help: true},
		{name: "must fail: status --help runs nothing", cwd: proj, op: "status", args: []string{"--help"}, help: true},
		{name: "must fail: ls --help runs nothing", cwd: proj, op: "ls", args: []string{"docs", "--help"}, help: true},
		{
			name: "must fail: an unknown flag on push runs nothing",
			cwd:  proj, op: "push", args: []string{"--concepts", "docs/a.md"}, wantErr: "--concepts",
		},
		{
			name: "must fail: an unknown flag on pull runs nothing",
			cwd:  proj, op: "pull", args: []string{"--no-brand"}, wantErr: "--no-brand",
		},
		{
			name: "must fail: an unknown shorthand flag on push runs nothing",
			cwd:  proj, op: "push", args: []string{"-x"}, wantErr: "-x",
		},
		{
			name: "must fail: an unknown flag on ls runs nothing",
			cwd:  proj, op: "ls", args: []string{"--bogus"}, wantErr: "--bogus",
		},
		{
			name: "must fail: pull given a path runs nothing",
			cwd:  proj, op: "pull", args: []string{"fr"}, wantErr: "takes no paths",
		},
		{
			name: "must fail: KAPI_NO_PROJECT without -p binds no project for push, inside a project directory",
			env:  map[string]string{"KAPI_NO_PROJECT": "1"}, cwd: proj, op: "push", wantErr: "KAPI_NO_PROJECT",
		},
		{
			name: "must fail: KAPI_NO_PROJECT without -p binds no project for pull, inside a project directory",
			env:  map[string]string{"KAPI_NO_PROJECT": "1"}, cwd: proj, op: "pull", wantErr: "KAPI_NO_PROJECT",
		},
		{
			name: "must fail: KAPI_NO_PROJECT without -p binds no project for status, inside a project directory",
			env:  map[string]string{"KAPI_NO_PROJECT": "1"}, cwd: proj, op: "status", wantErr: "KAPI_NO_PROJECT",
		},
		{
			name: "KAPI_NO_PROJECT leaves an explicit -p in force",
			env:  map[string]string{"KAPI_NO_PROJECT": "1"}, cwd: other, op: "push", args: []string{"-p", proj},
			want: &loggedConnectorCall{RPC: "Push", Root: proj},
		},
		{
			name: "must fail: KAPI_PROJECT names the project when no -p is given",
			env:  map[string]string{"KAPI_PROJECT": proj}, cwd: other, op: "push",
			want: &loggedConnectorCall{RPC: "Push", Root: proj},
		},
		{
			name: "must fail: a relative -p names the project from kapi's working directory",
			cwd:  base, op: "push", args: []string{"-p", "proj"},
			want: &loggedConnectorCall{RPC: "Push", Root: proj},
		},
		{
			name: "must fail: a -p naming no project runs nothing",
			cwd:  proj, op: "push", args: []string{"-p", missing}, wantErr: missing,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KAPI_NO_PROJECT", "")
			t.Setenv("KAPI_PROJECT", "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			t.Chdir(tc.cwd)
			r := newConnectorRoute(t)

			err := r.dispatch(t, tc.op, tc.args...)
			calls := r.calls(t)

			if tc.want != nil {
				require.NoError(t, err)
				require.Len(t, calls, 1, "the daemon receives one call")
				got := calls[0]
				require.True(t, filepath.IsAbs(got.Root), "the daemon receives an absolute project root, not %q", got.Root)
				wantInfo, err := os.Stat(tc.want.Root)
				require.NoError(t, err)
				gotInfo, err := os.Stat(got.Root)
				require.NoError(t, err)
				assert.True(t, os.SameFile(wantInfo, gotInfo), "the daemon receives the project %s, not %s", tc.want.Root, got.Root)
				got.Root = tc.want.Root
				assert.Equal(t, *tc.want, got)
				return
			}

			assert.Empty(t, calls, "the route reached the daemon")
			assert.Empty(t, lines(t, r.spawnLog), "a daemon started")
			if tc.help {
				assert.ErrorIs(t, err, pflag.ErrHelp)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
