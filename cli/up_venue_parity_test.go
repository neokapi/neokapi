package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi up` is offered by two surfaces: this cobra command and the MCP `up`
// tool an agent calls. Both take the decision "where does this run" from
// host.ResolveUpVenue, so one recipe converges in one place for a human and an
// agent; this test holds the two surfaces to it.

// venueParityProject writes a server-connected recipe and returns its path.
func venueParityProject(t *testing.T) string {
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
	require.NoError(t, os.WriteFile(recipe, []byte(`version: v1
name: VenueParity
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    path: src/en.json
    target: src/{lang}.json
bowrain:
  url: https://bowrain.example/ws/parity
  converge: manual
`), 0o644))
	return recipe
}

// venuePluginHost returns a plugin host owning the `server-up` plumbing, so the
// server venue is reachable in this process. The plumbing's manifest declares
// the flags named by declared, and the stub writes the arguments it is given
// to the file "args" beside it.
func venuePluginHost(t *testing.T, declared ...string) *pluginhost.Host {
	t.Helper()
	return venuePluginHostWith(t, flagSpecs(declared))
}

// venuePluginHostWith is venuePluginHost with the manifest's flag
// declaration as given: nil declares none, an empty list declares that the
// plumbing reads no flag.
func venuePluginHostWith(t *testing.T, flags []manifest.FlagSpec) *pluginhost.Host {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}
	dir := t.TempDir()
	binPath := filepath.Join(dir, "kapi-venue-stub")
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\necho \"$@\" > \"$(dirname \"$0\")/args\"\n"), 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "venue-stub",
		Version:         "0.0.1",
		Binary:          filepath.Base(binPath),
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: "server-up", Hidden: true, Flags: flags}},
		},
	}
	require.NoError(t, m.Validate())

	return pluginhost.NewHost([]*pluginhost.Plugin{{
		Dir:        dir,
		BinaryPath: binPath,
		Manifest:   m,
		Source:     pluginhost.Source{Order: 1, Label: "test", Path: dir},
	}}, nil)
}

// TestUpVenue_CLIAndMCPAgree: for one connected project, the venue the cobra
// command resolves and the venue the embedded (MCP) path resolves are the same
// decision, down to the route dispatched to.
func TestUpVenue_CLIAndMCPAgree(t *testing.T) {
	recipe := venueParityProject(t)
	a := &App{}
	a.PluginHost = venuePluginHost(t)

	// The CLI's own reading of its flags, on a command with none set: a plain
	// `kapi up` in a connected project.
	cmd := NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags(nil))
	cliDecision, err := a.ResolveUpVenue(recipe, upVenueOptions(cmd))
	require.NoError(t, err)

	// What RunUpDispatch, the MCP tool's entry point, resolves for a default
	// agent call.
	mcpDecision, err := a.ResolveUpVenue(recipe, host.UpOptions{}.VenueOptions())
	require.NoError(t, err)

	assert.Equal(t, host.UpVenueServer, cliDecision.Venue,
		"a connected project with the plumbing installed runs at the server venue")
	assert.Equal(t, cliDecision, mcpDecision,
		"the agent's up must resolve the venue the human's up resolves")
}

// TestUpVenue_CLIAndMCPAgreeWithoutAServer is the control: with no venue bound
// both surfaces keep the loop on this machine, so the parity above is not the
// accident of one shared "always server" answer.
func TestUpVenue_CLIAndMCPAgreeWithoutAServer(t *testing.T) {
	recipe := venueParityProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	delete(proj.Extras, "bowrain")
	require.NoError(t, project.Save(recipe, proj))

	a := &App{}
	a.PluginHost = venuePluginHost(t)

	cmd := NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags(nil))
	cliDecision, err := a.ResolveUpVenue(recipe, upVenueOptions(cmd))
	require.NoError(t, err)
	mcpDecision, err := a.ResolveUpVenue(recipe, host.UpOptions{}.VenueOptions())
	require.NoError(t, err)

	assert.Equal(t, host.UpVenueLocal, cliDecision.Venue)
	assert.Equal(t, cliDecision, mcpDecision)
}

// TestUpVenue_LocalFlagStillDispatches pins the one that reads backwards:
// --local means "converge here, then push", so it still dispatches to the
// plumbing, on both surfaces.
func TestUpVenue_LocalFlagStillDispatches(t *testing.T) {
	recipe := venueParityProject(t)
	a := &App{}
	a.PluginHost = venuePluginHost(t)

	cmd := NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags([]string{"--local"}))
	cliDecision, err := a.ResolveUpVenue(recipe, upVenueOptions(cmd))
	require.NoError(t, err)

	mcpDecision, err := a.ResolveUpVenue(recipe, host.UpOptions{Local: true}.VenueOptions())
	require.NoError(t, err)

	assert.Equal(t, host.UpVenueServer, cliDecision.Venue)
	assert.Equal(t, cliDecision, mcpDecision)
}

// A venue flag the plumbing acts on is still typed at `kapi up`, and this
// command is what parses argv: a flag only the plumbing declares is rejected as
// unknown before the plumbing runs at all. So the flag is registered here too,
// and forwarded to the subprocess that acts on it.
func TestUpVenue_AVenueFlagParsesHereAndReachesThePlumbing(t *testing.T) {
	cmd := NewUpCmd(&App{})
	require.NoError(t, cmd.ParseFlags([]string{"--fail-on-incomplete-watch"}),
		"kapi up parses argv before it execs the plumbing, so an unregistered flag never reaches it")

	assert.Contains(t, forwardedFlagArgs(cmd.Flags(), "server"), "--fail-on-incomplete-watch=true",
		"the plumbing parses the same flag surface, so what the user set has to reach it")
}

// The control: --server resolves the venue here and means nothing to the
// plumbing, so it is the one flag deliberately held back.
func TestUpVenue_TheServerFlagIsNotForwarded(t *testing.T) {
	cmd := NewUpCmd(&App{})
	require.NoError(t, cmd.ParseFlags([]string{"--server"}))

	assert.NotContains(t, forwardedFlagArgs(cmd.Flags(), "server"), "--server=true")
}

// flagSpecs declares names as bool flags.
func flagSpecs(names []string) []manifest.FlagSpec {
	var specs []manifest.FlagSpec
	for _, n := range names {
		specs = append(specs, manifest.FlagSpec{Name: n, Type: "bool"})
	}
	return specs
}

// serverRunFlags stands in for the flags the bowrain plumbing declares.
var serverRunFlags = []string{"project", "json", "local", "timeout", "fail-on-incomplete-watch"}

// stubArgs is what the stub plumbing of ph was last run with.
func stubArgs(t *testing.T, ph *pluginhost.Host) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ph.CommandRoute("server-up").Plugin.Dir, "args"))
	require.NoError(t, err)
	return string(b)
}

// At the server venue the host refuses, before it dispatches, every flag the
// plumbing does not declare, so a plugin that would accept the flag (the stub
// exits 0 on anything) never runs. Under --local the plumbing runs this
// machine's loop, and the same flag is forwarded.
func TestUpVenue_ServerRefusesUndeclaredFlagsBeforeDispatch(t *testing.T) {
	recipe := venueParityProject(t)
	for _, args := range [][]string{
		{"--provider", "demo"}, {"--model", "m"}, {"--memory", "m.db"}, {"--termstore", "t"},
		{"--trace", "t.json"}, {"--encoding", "utf-8"}, {"--concurrency", "2"},
		{"--fail-on-unknown"}, {"--strict"},
		{"--jobs", "2"}, {"--no-extract"}, {"--no-checks"}, {"--materialize"}, {"--passes", "1"},
	} {
		t.Run(args[0], func(t *testing.T) {
			a := &App{}
			ph := venuePluginHost(t, serverRunFlags...)
			a.PluginHost = ph
			cmd := NewUpCmd(a)
			require.NoError(t, cmd.ParseFlags(args))

			err := runUpWithVenue(a, cmd, recipe)
			require.Error(t, err)
			assert.Contains(t, err.Error(), args[0]+" applies to a run on this machine")
			assert.Contains(t, err.Error(), "--local")

			cmd = NewUpCmd(a)
			require.NoError(t, cmd.ParseFlags(append([]string{"--local"}, args...)))
			require.NoError(t, runUpWithVenue(a, cmd, recipe), "--local runs the loop here, which reads %s", args[0])
			assert.Contains(t, stubArgs(t, ph), args[0])
		})
	}
}

// The flags forwarded to a server run are the ones the plumbing's manifest
// declares: a declared flag reaches it, and plumbing that declares none is
// forwarded none.
func TestUpVenue_ServerForwardsWhatTheManifestDeclares(t *testing.T) {
	recipe := venueParityProject(t)

	a := &App{}
	ph := venuePluginHost(t, serverRunFlags...)
	a.PluginHost = ph
	cmd := NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags([]string{"--project", recipe, "--json", "--timeout", "1m", "--fail-on-incomplete-watch"}))
	require.NoError(t, runUpWithVenue(a, cmd, recipe))
	for _, want := range []string{"--project=" + recipe, "--json=true", "--timeout=1m0s", "--fail-on-incomplete-watch=true"} {
		assert.Contains(t, stubArgs(t, ph), want, "a server run reads it, so it reaches the plumbing")
	}

	// A declaration that lists no flag forwards none: the run itself is
	// dispatched, and every flag is refused.
	a = &App{}
	ph = venuePluginHostWith(t, []manifest.FlagSpec{})
	a.PluginHost = ph
	cmd = NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags([]string{"--json"}))
	require.ErrorContains(t, runUpWithVenue(a, cmd, recipe), "--json applies to a run on this machine")
	cmd = NewUpCmd(a)
	require.NoError(t, runUpWithVenue(a, cmd, recipe))
	assert.NotContains(t, stubArgs(t, ph), "--", "no flag is forwarded")
}

// A plugin whose manifest declares no flags predates this kapi. A server run
// through it is refused before dispatch, naming the plugin and the command
// that updates it, whatever flags were set; under --local it runs as before.
func TestUpVenue_APluginWithNoDeclarationIsRefusedWithTheUpdate(t *testing.T) {
	recipe := venueParityProject(t)
	for _, args := range [][]string{nil, {"--project", recipe}, {"--json"}} {
		a := &App{}
		ph := venuePluginHost(t)
		a.PluginHost = ph
		cmd := NewUpCmd(a)
		require.NoError(t, cmd.ParseFlags(args))
		err := runUpWithVenue(a, cmd, recipe)
		require.Error(t, err, "%v", args)
		assert.Contains(t, err.Error(), "venue-stub plugin 0.0.1")
		assert.Contains(t, err.Error(), "older than this kapi")
		assert.Contains(t, err.Error(), "kapi plugin update venue-stub")
		assert.NotContains(t, err.Error(), "applies to a run on this machine")
		_, statErr := os.Stat(filepath.Join(ph.CommandRoute("server-up").Plugin.Dir, "args"))
		assert.True(t, os.IsNotExist(statErr), "nothing was dispatched")
	}

	a := &App{}
	ph := venuePluginHost(t)
	a.PluginHost = ph
	cmd := NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags([]string{"--local", "--jobs", "2"}))
	require.NoError(t, runUpWithVenue(a, cmd, recipe))
	assert.Contains(t, stubArgs(t, ph), "--jobs=2")
}
