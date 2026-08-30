package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/project/projecttest"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "Bring up to date" is the desktop's `kapi up`, and a project whose recipe
// binds a convergence venue runs it there — pushed, converged on the org's
// keys against the shared content memory and terminology, streamed back,
// pulled. Converging such a project locally instead spends the user's own
// provider and leaves the server holding the state the team reviews from.

// venueStubPlugin installs a `server-up` plugin under a throwaway plugin root
// and points discovery at it, so the desktop resolves the venue route the way
// it does in a real installation. It answers with the NDJSON document the real
// plumbing writes under --json and records the argv it was dispatched with.
//
// Returns the path of the file the stub writes its argv to.
func venueStubPlugin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}
	root := t.TempDir()
	pluginDir := filepath.Join(root, "venue-stub")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	argsFile := filepath.Join(root, "argv")
	binPath := filepath.Join(pluginDir, "kapi-venue-stub")
	lines := []string{
		`{"type":"pass_start","pass":1,"maxPasses":5,"pending":["nb-NO"]}`,
		`{"type":"locale_start","locale":"nb-NO","units":2,"pass":1}`,
		`{"type":"locale_done","locale":"nb-NO","done":2,"viaTM":1,"viaAI":1,"pass":1}`,
		`{"type":"pass_done","pass":1,"produced":2,"producedDelta":2}`,
		`{"type":"done","state":"converged"}`,
		`{"type":"result","flow":"server-venue","passes":1,"converged":true,"materializedFiles":1}`,
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n"
	for _, l := range lines {
		script += "echo '" + l + "'\n"
	}
	require.NoError(t, os.WriteFile(binPath, []byte(script), 0o755))

	manifestBody := `{
		"manifest_version": "1",
		"plugin": "venue-stub",
		"version": "0.0.1",
		"binary": "kapi-venue-stub",
		"capabilities": {"commands": [{"name": "server-up", "hidden": true}]}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifestBody), 0o644))

	// Dogfood isolation contract (CLAUDE.md): pin every root this could inherit,
	// and discover only the stub rather than whatever the developer has
	// installed.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR", root)
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_NO_PROJECT", "1")
	return argsFile
}

// dispatchedVenueArgs returns the argv the venue stub was run with, or nil when
// it was never run.
func dispatchedVenueArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	raw, err := os.ReadFile(argsFile)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var args []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line != "" {
			args = append(args, line)
		}
	}
	return args
}

// newVenueProject writes a recipe binding a convergence venue and opens it. The
// venue is registered as an extension, exactly as a platform plugin registers
// it, so the fixture exercises the registered venue rather than a key name the
// framework would have to know.
//
// The flow is the deterministic pseudo-translate one, so a local fallback (the
// failure this test exists to catch) costs no key, no network and no spend, and
// leaves a target file behind as its evidence.
func newVenueProject(t *testing.T, app *App, connected bool) (*TabInfo, string) {
	t.Helper()
	projecttest.ResetExtensions()
	t.Cleanup(projecttest.ResetExtensions)
	project.RegisterExtensionGroup("platform", []project.Extension{
		{Name: "bowrain", Scope: project.ScopeProject, Venue: true},
	})

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye"}`), 0o644))

	body := `version: v1
name: VenueConverge
defaults:
  source_language: en-US
  target_languages: [nb-NO]
  source_gate: none
  flow: pseudo
collections:
  - name: App
    path: locales/en.json
    target: locales/{lang}.json
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
`
	if connected {
		body += `bowrain:
  url: https://bowrain.example/ws/desktop-venue
  converge: manual
`
	}
	recipe := filepath.Join(root, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(body), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))

	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

// awaitTerminalRun blocks until the run reaches a terminal state.
func awaitTerminalRun(t *testing.T, app *App) {
	t.Helper()
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError) || s == string(RunStateCanceled)
	}, 30*time.Second, 50*time.Millisecond, "the convergence run should reach a terminal state")
}

func TestBringUpToDate_ConnectedProjectRunsAtTheVenue(t *testing.T) {
	argsFile := venueStubPlugin(t)
	app := NewApp()
	tab, root := newVenueProject(t, app, true)

	require.NoError(t, app.BringUpToDate(tab.ID))
	awaitTerminalRun(t, app)
	require.Equal(t, string(RunStateComplete), app.GetRunState())

	// The run reached the venue plumbing, with the project and the structured
	// document the embedded caller reads.
	args := dispatchedVenueArgs(t, argsFile)
	require.NotEmpty(t, args, "a connected project must dispatch to the venue")
	assert.Equal(t, []string{"command", "server-up"}, args[:2],
		"dispatch goes through the plugin's command route")
	assert.Contains(t, args, "--project="+filepath.Join(root, project.RecipeFileName))
	assert.Contains(t, args, "--json")

	// It did not also converge here: a local run of the pseudo flow would have
	// written the target file.
	_, statErr := os.Stat(filepath.Join(root, "locales", "nb-NO.json"))
	assert.True(t, os.IsNotExist(statErr),
		"a venue run must not also converge locally (found a locally materialized target)")
}

func TestBringUpToDate_VenueRunStreamsProgressIntoTheRunView(t *testing.T) {
	venueStubPlugin(t)
	app := NewApp()
	tab, _ := newVenueProject(t, app, true)

	require.NoError(t, app.BringUpToDate(tab.ID))
	awaitTerminalRun(t, app)
	require.Equal(t, string(RunStateComplete), app.GetRunState())

	// The run view renders one stream whatever the venue: the plumbing's typed
	// events arrive as "converge_event" and the result rides "complete", exactly
	// as a local run's do.
	var types []convergence.EventType
	var result *host.ConvergeOutput
	for _, ev := range app.GetRunEvents() {
		switch ev.Type {
		case "converge_event":
			require.NotNil(t, ev.ConvergeEvent)
			types = append(types, ev.ConvergeEvent.Type)
		case "complete":
			result = ev.ConvergeResult
		}
	}
	assert.Equal(t, []convergence.EventType{
		convergence.EventPassStart,
		convergence.EventLocaleStart,
		convergence.EventLocaleDone,
		convergence.EventPassDone,
		convergence.EventDone,
	}, types, "every event the venue emitted reaches the run view, in order")

	require.NotNil(t, result, "the complete event carries the venue run's result")
	assert.Equal(t, "server-venue", result.Flow)
	assert.True(t, result.Converged)
}

// A project with no bound venue behaves exactly as it did: the loop runs here.
func TestBringUpToDate_LocalProjectStillConvergesHere(t *testing.T) {
	argsFile := venueStubPlugin(t)
	app := NewApp()
	tab, root := newVenueProject(t, app, false)

	require.NoError(t, app.BringUpToDate(tab.ID))
	awaitTerminalRun(t, app)
	require.Equal(t, string(RunStateComplete), app.GetRunState())

	assert.Empty(t, dispatchedVenueArgs(t, argsFile),
		"an unconnected project dispatches nowhere")
	_, statErr := os.Stat(filepath.Join(root, "locales", "nb-NO.json"))
	require.NoError(t, statErr, "the local loop materializes the target")
}

// The three faces decide the venue once, together. The CLI and the MCP tool are
// held to each other by cli/up_venue_parity_test.go; this holds the desktop to
// the same decision for the same recipe, so the chain covers all three.
func TestBringUpToDate_VenueDecisionMatchesTheEmbeddedSurfaces(t *testing.T) {
	venueStubPlugin(t)
	app := NewApp()
	_, root := newVenueProject(t, app, true)
	recipe := filepath.Join(root, project.RecipeFileName)

	// The App a run is driven on, built exactly as executeConvergeRun builds it.
	capp := app.borrowEngine(&host.App{})
	desktop, err := capp.ResolveUpVenue(recipe, host.UpOptions{UntilGate: true}.VenueOptions())
	require.NoError(t, err)

	embedded, err := capp.ResolveUpVenue(recipe, host.UpOptions{}.VenueOptions())
	require.NoError(t, err)

	assert.Equal(t, host.UpVenueServer, desktop.Venue,
		"a connected project with the plumbing installed runs at the venue")
	assert.Equal(t, embedded, desktop,
		"the desktop resolves the venue the embedded surfaces do")
}

// The desktop's run events are what the frontend reads over the Wails bridge, so
// a venue run's events must survive the JSON boundary with their fields intact.
func TestBringUpToDate_VenueEventsSurviveTheBridge(t *testing.T) {
	venueStubPlugin(t)
	app := NewApp()
	tab, _ := newVenueProject(t, app, true)

	require.NoError(t, app.BringUpToDate(tab.ID))
	awaitTerminalRun(t, app)

	var localeDone *convergence.Event
	for _, ev := range app.GetRunEvents() {
		if ev.Type == "converge_event" && ev.ConvergeEvent.Type == convergence.EventLocaleDone {
			localeDone = ev.ConvergeEvent
		}
	}
	require.NotNil(t, localeDone)

	raw, err := json.Marshal(localeDone)
	require.NoError(t, err)
	var back convergence.Event
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, "nb-NO", back.Locale)
	assert.Equal(t, 2, back.Done)
	assert.Equal(t, 1, back.ViaMemory)
	assert.Equal(t, 1, back.ViaAI)
}
