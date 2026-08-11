package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MCP `up` tool ran the loop in-process whatever the recipe said. In a
// server-connected project a human's `kapi up` pushes, converges on the org's
// keys and pulls the results; the agent's `up` did none of that — it spent the
// user's own provider on a local loop and left the server holding the state the
// team reviews from. The venue is one decision now, and both surfaces make it.

// newVenueTestProject writes a recipe that binds a convergence venue (or does
// not) and returns the recipe path. The venue is registered as an extension,
// exactly as a platform plugin registers it, so this exercises the registered
// venue rather than a key name the framework would have to know.
func newVenueTestProject(t *testing.T, connected bool) string {
	t.Helper()
	// Dogfood isolation contract (CLAUDE.md): pin every root this could inherit.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	projecttest.ResetExtensions()
	t.Cleanup(projecttest.ResetExtensions)
	project.RegisterExtensionGroup("platform", []project.Extension{
		{Name: "bowrain", Scope: project.ScopeProject, Venue: true},
	})

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world"}`), 0o644))

	recipe := filepath.Join(dir, project.RecipeFileName)
	// The local venue really runs: the flow translates with the deterministic
	// demo provider, so a local fallback costs no key, no network and no spend.
	body := `version: v1
name: VenueTest
defaults:
  source_language: en
  target_languages: [nb]
  source_gate: none
  flow: translate
collections:
  - name: app
    path: src/en.json
    target: src/{lang}.json
flows:
  translate:
    steps:
      - tool: translate
        config:
          provider: demo
`
	if connected {
		body += `bowrain:
  url: https://bowrain.example/ws/venue-test
  converge: manual
`
	}
	require.NoError(t, os.WriteFile(recipe, []byte(body), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	return recipe
}

// serverUpStub is a stub plugin owning the `server-up` plumbing. It records the
// argv kapi dispatched and answers with the NDJSON document the real plumbing
// writes under --json, so a caller that dispatches gets a result and a caller
// that does not leaves no trace.
type serverUpStub struct {
	host     *pluginhost.Host
	argsFile string
}

func newServerUpStub(t *testing.T) *serverUpStub {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "argv")
	binPath := filepath.Join(dir, "kapi-venue-stub")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argsFile + "\n" +
		`echo '{"type":"locale","locale":"nb"}'` + "\n" +
		`echo '{"type":"result","flow":"server-venue","passes":2,"converged":true,"materializedFiles":3}'` + "\n"
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "venue-stub",
		Version:         "0.0.1",
		Binary:          filepath.Base(binPath),
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: serverUpCommand, Hidden: true}},
		},
	}
	require.NoError(t, m.Validate())

	p := &pluginhost.Plugin{
		Dir:        dir,
		BinaryPath: binPath,
		Manifest:   m,
		Source:     pluginhost.Source{Order: 1, Label: "test", Path: dir},
	}
	return &serverUpStub{host: pluginhost.NewHost([]*pluginhost.Plugin{p}, nil), argsFile: argsFile}
}

// dispatchedArgs returns the argv the stub was run with, or nil when it was
// never run.
func (s *serverUpStub) dispatchedArgs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(s.argsFile)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var args []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if line != "" {
			args = append(args, line)
		}
	}
	return args
}

// TestResolveUpVenue is the decision table both surfaces share.
func TestResolveUpVenue(t *testing.T) {
	cases := []struct {
		name       string
		connected  bool
		plumbing   bool
		opts       UpVenueOptions
		wantVenue  UpVenue
		wantErr    string
		wantServer bool
	}{
		{name: "no venue bound runs here", wantVenue: UpVenueLocal},
		{name: "venue bound but no plumbing runs here", connected: true, wantVenue: UpVenueLocal, wantServer: true},
		{name: "venue bound with plumbing dispatches", connected: true, plumbing: true, wantVenue: UpVenueServer, wantServer: true},
		{
			name: "local still dispatches so the server is pushed", connected: true, plumbing: true,
			opts: UpVenueOptions{Local: true}, wantVenue: UpVenueServer, wantServer: true,
		},
		{
			name: "plan never dispatches", connected: true, plumbing: true,
			opts: UpVenueOptions{Plan: true}, wantVenue: UpVenueLocal, wantServer: true,
		},
		{
			name: "server demanded without a venue fails", plumbing: true,
			opts: UpVenueOptions{ForceServer: true}, wantErr: "needs a recipe connected",
		},
		{
			name: "server demanded without plumbing fails", connected: true,
			opts: UpVenueOptions{ForceServer: true}, wantErr: "kapi plugin install bowrain", wantServer: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recipe := newVenueTestProject(t, tc.connected)
			a := &App{}
			if tc.plumbing {
				a.PluginHost = newServerUpStub(t).host
			}

			dec, err := a.ResolveUpVenue(recipe, tc.opts)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantVenue, dec.Venue)
			assert.Equal(t, tc.wantServer, dec.HasServer)
			if tc.wantVenue == UpVenueServer {
				require.NotNil(t, dec.Route, "the server venue must carry the route it dispatches to")
				assert.Equal(t, "https://bowrain.example/ws/venue-test", dec.ServerURL)
			} else {
				assert.Nil(t, dec.Route)
			}
		})
	}
}

// callMCPUp runs the registered MCP `up` tool over a real session — the path an
// agent takes — and returns the structured result.
func callMCPUp(t *testing.T, a *App, in map[string]any) (*ConvergeOutput, error) {
	t.Helper()
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "kapi-test", Version: "test"}, nil)
	registerUpMCPTools(server, a)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "up", Arguments: in})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, toolErrorFrom(res)
	}
	var out ConvergeOutput
	require.NoError(t, json.Unmarshal(mustJSON(t, res.StructuredContent), &out))
	return &out, nil
}

// toolErrorFrom turns an MCP tool error result into a Go error so a caller can
// assert on the message.
func toolErrorFrom(res *mcp.CallToolResult) error {
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return &mcpToolError{msg: strings.Join(parts, "")}
}

type mcpToolError struct{ msg string }

func (e *mcpToolError) Error() string { return e.msg }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

// TestMCPUp_RunsAtTheServerVenue is the core regression: over MCP, a
// server-connected project's `up` reaches the venue plumbing — with the project
// and the structured stream it needs — and returns the run's result.
func TestMCPUp_RunsAtTheServerVenue(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	stub := newServerUpStub(t)
	a := &App{PluginHost: stub.host}

	out, err := callMCPUp(t, a, map[string]any{"project": recipe, "passes": 2})
	require.NoError(t, err)

	require.NotNil(t, out)
	assert.Equal(t, "server-venue", out.Flow, "the result must be the venue run's, not a local loop's")
	assert.Equal(t, 2, out.Passes)
	assert.True(t, out.Converged)
	assert.Equal(t, 3, out.MaterializedFiles)

	args := stub.dispatchedArgs(t)
	require.NotEmpty(t, args, "the agent's up must dispatch to the venue plumbing")
	assert.Equal(t, []string{"command", serverUpCommand}, args[:2], "dispatch is a Mode-A command")
	assert.Contains(t, args, "--project="+recipe, "the plumbing must be pointed at this recipe, not a discovered one")
	assert.Contains(t, args, "--json", "an embedded run reads the structured document, and --json is also what stops the prompt")
	assert.Contains(t, args, "--passes=2")
}

// TestMCPUp_LocalProjectStillRunsHere is the control: without a bound venue
// nothing is dispatched, so a plain project keeps the local loop.
func TestMCPUp_LocalProjectStillRunsHere(t *testing.T) {
	recipe := newVenueTestProject(t, false)
	stub := newServerUpStub(t)
	a := &App{PluginHost: stub.host}
	a.SourceLang = "en"

	out, err := callMCPUp(t, a, map[string]any{"project": recipe, "passes": 1, "no_checks": true})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Empty(t, stub.dispatchedArgs(t), "a project binding no venue must not reach the plumbing")
	assert.NotEqual(t, "server-venue", out.Flow)
}

// TestRunUpDispatch_ServerVenueForwardsTheEmbeddedOptions pins the option
// mapping: what an embedded caller asks for must reach the plumbing as the
// flags it parses, or the agent's knobs silently stop working at the venue.
func TestRunUpDispatch_ServerVenueForwardsTheEmbeddedOptions(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	stub := newServerUpStub(t)
	a := &App{PluginHost: stub.host}

	out, err := a.RunUpDispatch(context.Background(), recipe, "", UpOptions{
		UntilGate:   true,
		MaxPasses:   3,
		Jobs:        2,
		NoChecks:    true,
		Materialize: true,
		Local:       true,
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	args := stub.dispatchedArgs(t)
	for _, want := range []string{"--passes=3", "--jobs=2", "--no-checks=true", "--materialize=true", "--local=true"} {
		assert.Contains(t, args, want)
	}
}

// TestRunUpDispatch_LocalVenueRunsTheLoopHere keeps the fallback honest: with
// the plumbing absent, a connected recipe still converges locally rather than
// failing — the same degradation the CLI has.
func TestRunUpDispatch_LocalVenueRunsTheLoopHere(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	a := &App{}
	a.SourceLang = "en"

	out, err := a.RunUpDispatch(context.Background(), recipe, "", UpOptions{
		UntilGate: false,
		MaxPasses: 1,
		NoChecks:  true,
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.NotEqual(t, "server-venue", out.Flow)
}

// TestParseUpResultStream reads the venue's document the way the dispatcher
// does: progress lines are skipped and the closing record is the result.
func TestParseUpResultStream(t *testing.T) {
	t.Run("the closing record is the result", func(t *testing.T) {
		out, err := parseUpResultStream([]byte(
			"{\"type\":\"locale\",\"locale\":\"nb\"}\n" +
				"{\"type\":\"result\",\"flow\":\"remote\",\"converged\":true}\n"))
		require.NoError(t, err)
		assert.Equal(t, "remote", out.Flow)
		assert.True(t, out.Converged)
	})

	t.Run("a document with no result is an error", func(t *testing.T) {
		_, err := parseUpResultStream([]byte("{\"type\":\"locale\"}\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no result record")
	})

	t.Run("plugin noise between records is tolerated", func(t *testing.T) {
		out, err := parseUpResultStream([]byte(
			"Pushing local changes...\n{\"type\":\"result\",\"flow\":\"remote\"}\n"))
		require.NoError(t, err)
		assert.Equal(t, "remote", out.Flow)
	})
}
