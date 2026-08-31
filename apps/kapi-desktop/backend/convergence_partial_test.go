package backend

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A converge run that fails partway must leave the UI able to tell. Two things
// used to conspire against that:
//
//   - the failure path returned before emitting "project:extracted" /
//     "outputs-changed", so the home surfaces never re-derived and kept showing
//     the pre-run numbers, and
//   - nothing recorded that the run had failed at all, so a project whose
//     catch-up died on its first locale was indistinguishable from a converged
//     one.
//
// The fixture makes the failure real, offline, and deterministic without a
// provider call: fr-FR is pre-written as a complete translation (so it is
// already converged and the planner has nothing to do for it), while the
// default flow's translate step names a provider that does not exist — so the
// de-DE pass cannot even assemble its tool. The result is exactly the founder's
// scenario: one locale done, one locale unproduced, and a failed run.

// newPartialFailureProject scaffolds a two-locale project where fr-FR is already
// complete and de-DE's pass is guaranteed to fail.
func newPartialFailureProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye"}`), 0o644))
	// fr-FR is already fully translated — the converged half of the picture.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour","farewell":"Au revoir"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "PartialConverge",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
			Flow:            "pseudo",
		},
		Collections: []project.Collection{{
			Name:    "App",
			Content: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
		}},
		Flows: map[string]*flow.StepsSpec{
			// An unknown provider fails at tool assembly — no network, no
			// dependence on whether the developer happens to run Ollama.
			"pseudo": {Steps: []flow.FlowStep{{
				Tool:   "translate",
				Config: map[string]any{"provider": "no-such-provider"},
			}}},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

func TestBringUpToDate_PartialFailure_StateReflectsPartial(t *testing.T) {
	// Isolate the app config: this test runs a real converge, and reading the
	// developer's ~/.config/kapi would make the outcome depend on whichever
	// provider they happen to have configured (the dogfood isolation contract).
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	tab, root := newPartialFailureProject(t, app)

	// Observe the derived-state invalidation events the home surfaces listen on.
	var mu sync.Mutex
	seen := map[string]int{}
	InjectEventSink(app, func(name string, _ any) {
		mu.Lock()
		seen[name]++
		mu.Unlock()
	})
	t.Cleanup(func() { InjectEventSink(app, nil) })

	require.NoError(t, app.BringUpToDate(tab.ID))
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError) ||
			s == string(RunStateCanceled)
	}, 60*time.Second, 50*time.Millisecond, "the run must reach a terminal state")

	require.Equal(t, string(RunStateError), app.GetRunState(),
		"a pass whose tool cannot be assembled must fail the run, not be swallowed")

	// 1. The failure is recorded, so the UI can report it rather than infer it.
	runErr := app.GetLastRunError()
	require.NotNil(t, runErr, "a failed run must be recorded for the home surfaces")
	assert.NotEmpty(t, runErr.Headline)
	assert.NotEmpty(t, runErr.Raw, "the raw chain stays available for Details")
	assert.NotEqual(t, RunErrCanceled, runErr.Kind)

	// 2. The failure path re-derives, exactly as the success path does. Without
	//    these the panels keep rendering pre-run numbers as current.
	mu.Lock()
	extracted, outputs := seen["project:extracted"], seen["outputs-changed"]
	mu.Unlock()
	assert.Positive(t, extracted,
		`a failed run must still emit "project:extracted" so coverage re-derives`)
	assert.Positive(t, outputs,
		`a failed run must still emit "outputs-changed" so file tables refresh`)

	// 3. The run event stream carries the structured failure.
	var sawStructuredError bool
	for _, ev := range app.GetRunEvents() {
		if ev.Type == "error" && ev.Error != nil {
			sawStructuredError = true
			assert.NotEmpty(t, ev.Error.Headline)
			assert.NotEmpty(t, ev.Error.Raw)
		}
	}
	assert.True(t, sawStructuredError, "the error event must carry the classified failure")

	// 4. The derived state is honest: de-DE has no output file and must not be
	//    counted as converged, nor clear the translated:100 ship gate.
	rep, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	require.NotEmpty(t, rep.Locales)

	byLocale := map[string]int{}
	shippable := map[string]bool{}
	for _, lc := range rep.Locales {
		byLocale[lc.Locale] = lc.Pct["translated"]
		shippable[lc.Locale] = lc.Shippable
	}

	require.Contains(t, byLocale, "de-DE")
	assert.NotEqual(t, 100, byLocale["de-DE"],
		"de-DE never got a target file — it cannot read as fully translated")
	assert.False(t, shippable["de-DE"], "de-DE cannot clear translated:100")

	// de-DE really has no output file on disk.
	_, serr := os.Stat(filepath.Join(root, "locales", "de-DE.json"))
	assert.True(t, os.IsNotExist(serr), "de-DE was never written")

	// 5. Partial, not total: fr-FR was already complete and stays so. The report
	//    distinguishes the two halves instead of collapsing to one verdict — which
	//    is precisely what "100% up to date" after a failed run got wrong.
	require.Contains(t, byLocale, "fr-FR")
	assert.Equal(t, 100, byLocale["fr-FR"], "fr-FR was already complete")
	assert.True(t, shippable["fr-FR"])
	assert.Len(t, rep.Locales, 2, "both locales are still reported")
}

func TestBringUpToDate_SuccessClearsARecordedFailure(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)

	// Seed a stale failure as though a previous run had died.
	app.runState = newRunner()
	app.recordRunFailure(&RunError{Kind: RunErrModelNotInstalled, Headline: "stale", Raw: "stale"})
	require.NotNil(t, app.GetLastRunError())

	require.NoError(t, app.BringUpToDate(tab.ID))
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError)
	}, 60*time.Second, 50*time.Millisecond)
	require.Equal(t, string(RunStateComplete), app.GetRunState())

	assert.Nil(t, app.GetLastRunError(),
		"a successful run clears the recorded failure — otherwise the home stays red forever")
}
