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
// tool an agent calls. Each used to answer "where does this run" for itself,
// and only one of them knew about the venue — so the same recipe converged on
// the server for a human and on the laptop for an agent. The decision belongs
// to host.ResolveUpVenue; this test holds the two surfaces to it.

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
// server venue is reachable in this process.
func venuePluginHost(t *testing.T) *pluginhost.Host {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}
	dir := t.TempDir()
	binPath := filepath.Join(dir, "kapi-venue-stub")
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "venue-stub",
		Version:         "0.0.1",
		Binary:          filepath.Base(binPath),
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: "server-up", Hidden: true}},
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

	// The CLI's own reading of its flags, on a command with none set — a plain
	// `kapi up` in a connected project.
	cmd := NewUpCmd(a)
	require.NoError(t, cmd.ParseFlags(nil))
	cliDecision, err := a.ResolveUpVenue(recipe, upVenueOptions(cmd))
	require.NoError(t, err)

	// What RunUpDispatch — the MCP tool's entry point — resolves for a default
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
// --local is not "skip the venue", it is "converge here, then push" — so it
// still dispatches to the plumbing, on both surfaces.
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
