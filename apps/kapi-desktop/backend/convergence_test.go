package backend

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	appconfig "github.com/neokapi/neokapi/host/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newConvergenceProject scaffolds a project whose default flow is an inline
// pseudo-translate flow, with two target locales and a translated:100 ship gate,
// and opens it. Returns the tab and project root.
func newConvergenceProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting":"Hello","farewell":"Goodbye"}`), 0o644))

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Converge",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
			Flow:            "pseudo",
		},
		Content: []project.ContentCollection{{
			Name:  "App",
			Items: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
		}},
		Flows: map[string]*flow.StepsSpec{
			"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
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

func TestGetConvergence_DerivesCoverage(t *testing.T) {
	app := NewApp()
	tab, root := newConvergenceProject(t, app)

	// Before any translation: both locales present in the report, none translated,
	// neither shippable against translated:100.
	rep, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, rep)
	require.Len(t, rep.Locales, 2)
	for _, lc := range rep.Locales {
		assert.Equal(t, 0, lc.Pct["translated"], "%s has no target file yet", lc.Locale)
		assert.False(t, lc.Shippable, "%s gate (translated:100) unmet", lc.Locale)
	}

	// Materialize a fully-translated fr-FR target; coverage reflects it on the
	// next derivation (file-based, the same as `kapi status`).
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour","farewell":"Au revoir"}`), 0o644))

	rep2, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	byLoc := map[string]LocalePct{}
	for _, lc := range rep2.Locales {
		byLoc[lc.Locale] = LocalePct{lc.Pct["translated"], lc.Shippable}
	}
	assert.Equal(t, 100, byLoc["fr-FR"].translated, "fr-FR fully translated")
	assert.True(t, byLoc["fr-FR"].shippable, "fr-FR clears translated:100")
	assert.Equal(t, 0, byLoc["de-DE"].translated, "de-DE still untranslated")
	assert.False(t, byLoc["de-DE"].shippable)
}

// LocalePct is a tiny test helper bundling the two fields asserted above.
type LocalePct struct {
	translated int
	shippable  bool
}

func TestBringUpToDate_RunsSharedUpEngine(t *testing.T) {
	app := NewApp()
	tab, root := newConvergenceProject(t, app)

	// BringUpToDate drives the CLI's up engine (loop-to-gate, auto-extract on
	// drift, checks in the loop) and reaches a terminal state.
	require.NoError(t, app.BringUpToDate(tab.ID))
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError)
	}, 30*time.Second, 50*time.Millisecond, "the convergence run should reach a terminal state")
	require.Equal(t, string(RunStateComplete), app.GetRunState(), "the pseudo default flow converges")

	// The engine honors the project's target locales (unlike a raw flow run):
	// both target files materialize, so file-derived coverage clears the gate.
	for _, loc := range []string{"fr-FR", "de-DE"} {
		_, serr := os.Stat(filepath.Join(root, "locales", loc+".json"))
		require.NoError(t, serr, "convergence must materialize %s", loc)
	}
	rep, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	for _, lc := range rep.Locales {
		assert.Equal(t, 100, lc.Pct["translated"], "%s fully translated", lc.Locale)
		assert.True(t, lc.Shippable, "%s clears translated:100", lc.Locale)
	}

	// The event stream carries the convergence protocol: typed per-pass/locale
	// events and a final structured result (what the runner's run view renders).
	events := app.GetRunEvents()
	var passes int
	var result *host.ConvergeOutput
	for _, ev := range events {
		if ev.Type == "converge_event" {
			require.NotNil(t, ev.ConvergeEvent)
			if ev.ConvergeEvent.Type == convergence.EventPassDone {
				passes++
				assert.Positive(t, ev.ConvergeEvent.Produced)
			}
		}
		if ev.Type == "complete" {
			result = ev.ConvergeResult
		}
	}
	assert.Positive(t, passes, "at least one pass_done event")
	require.NotNil(t, result, "the complete event carries the structured result")
	assert.True(t, result.Converged)
	assert.Empty(t, result.ParkedScopes)
}

func TestBringUpToDate_RefusesConcurrentRun(t *testing.T) {
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)

	require.NoError(t, app.BringUpToDate(tab.ID))
	// While the first run is live, a second launch is refused.
	err := app.BringUpToDate(tab.ID)
	if err == nil {
		// The first run may already have finished on a fast machine — only a
		// still-running state must refuse.
		t.Skip("first run finished before the second launch; nothing to assert")
	}
	assert.Contains(t, err.Error(), "already running")
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError)
	}, 30*time.Second, 50*time.Millisecond)
}

func TestGetConvergePlan_ReportsPendingWorkAndDrift(t *testing.T) {
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)

	// Never extracted, nothing translated: every unit × locale is pending and
	// the store is missing (the drift Bring up to date would heal).
	plan, err := app.GetConvergePlan(tab.ID)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.True(t, plan.StoreMissing, "no block store yet")
	assert.Equal(t, 4, plan.Plan.Totals.MissingTarget, "2 units × 2 locales")
	assert.Equal(t, 4, plan.Plan.Totals.AIRemaining, "no content memory → all AI work")
	assert.Positive(t, plan.Plan.Totals.TokenEstimate)
	assert.NotEmpty(t, plan.Plan.Note, "the token heuristic is disclosed")
	require.Len(t, plan.Plan.Scopes, 2)
	for _, s := range plan.Plan.Scopes {
		assert.Equal(t, 2, s.MissingTarget)
	}
}

func TestGetConvergePlan_ConvergedProjectHasNoWork(t *testing.T) {
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)

	// Bring the project up to date, then re-plan: nothing left to do.
	require.NoError(t, app.BringUpToDate(tab.ID))
	require.Eventually(t, func() bool {
		return app.GetRunState() == string(RunStateComplete)
	}, 30*time.Second, 50*time.Millisecond)

	plan, err := app.GetConvergePlan(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, plan.Plan.Totals.MissingTarget)
	assert.Empty(t, plan.Plan.Scopes)
	assert.False(t, plan.StoreMissing, "the run's auto-extract populated the store")
	assert.Zero(t, plan.ChangedFiles, "sources unchanged since the run's extract")
}

func TestGetConvergePlan_UnknownTab(t *testing.T) {
	app := NewApp()
	_, err := app.GetConvergePlan("nope")
	require.Error(t, err)
}

// TestBringUpToDate_NoDefaultFlowUsesBuiltinDefault: a recipe with no
// defaults.flow launches fine — the shared up engine synthesizes the built-in
// default flow (#1078 G6: recycle → translate). With the offline demo provider
// the run converges and materializes every target.
func TestBringUpToDate_NoDefaultFlowUsesBuiltinDefault(t *testing.T) {
	app := NewApp()
	app.aiConfig = appconfig.NewAppConfig()
	app.aiConfig.Set(appconfig.KeyAIProvider, "demo")
	tab, root := newCoverageProject(t, app) // no defaults.flow

	require.NoError(t, app.BringUpToDate(tab.ID), "a flowless recipe must launch, not error")
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError)
	}, 30*time.Second, 50*time.Millisecond, "the convergence run should reach a terminal state")
	require.Equal(t, string(RunStateComplete), app.GetRunState(), "the built-in default (demo provider) converges")

	for _, loc := range []string{"fr-FR", "de-DE"} {
		_, serr := os.Stat(filepath.Join(root, "locales", loc+".json"))
		require.NoError(t, serr, "the built-in default must materialize %s", loc)
	}

	// The final structured result reports the built-in default flow.
	var result *host.ConvergeOutput
	for _, ev := range app.GetRunEvents() {
		if ev.Type == "complete" {
			result = ev.ConvergeResult
		}
	}
	require.NotNil(t, result)
	assert.Equal(t, host.BuiltinDefaultFlowLabel, result.Flow)
}

// TestGetConvergence_MissingProjectFiles: a tab whose recipe vanished from
// disk (moved/deleted directory) gets the typed friendly error, not a raw
// open-error — and both derivation bindings agree.
func TestGetConvergence_MissingProjectFiles(t *testing.T) {
	app := NewApp()
	tab, root := newConvergenceProject(t, app)
	require.NoError(t, os.Rename(
		filepath.Join(root, "project.kapi"),
		filepath.Join(root, "project.kapi.moved")))

	_, err := app.GetConvergence(tab.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project files are missing or moved")

	_, err = app.GetConvergePlan(tab.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project files are missing or moved")

	// The file returns → derivations recover without reopening the tab.
	require.NoError(t, os.Rename(
		filepath.Join(root, "project.kapi.moved"),
		filepath.Join(root, "project.kapi")))
	_, err = app.GetConvergence(tab.ID)
	require.NoError(t, err)
}

func TestGetConvergence_UnknownTab(t *testing.T) {
	app := NewApp()
	_, err := app.GetConvergence("nope")
	require.Error(t, err)
}

func TestApproveReviewItem_PromotesToReviewed(t *testing.T) {
	app := NewApp()
	tab, root := newConvergenceProject(t, app)

	// Materialize a fully-translated fr-FR target so its units enter the review
	// queue (translated, not yet approved).
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour","farewell":"Au revoir"}`), 0o644))

	rep, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	var item *struct{ Locale, File, Key string }
	for _, it := range rep.Review {
		if it.Locale == "fr-FR" {
			item = &struct{ Locale, File, Key string }{it.Locale, it.File, it.Key}
			break
		}
	}
	require.NotNil(t, item, "fr-FR has translated units awaiting review")

	require.NoError(t, app.ApproveReviewItem(tab.ID, item.Locale, item.File, item.Key))

	after, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	for _, lc := range after.Locales {
		if lc.Locale == "fr-FR" {
			assert.Equal(t, 50, lc.Pct["reviewed"], "1 of 2 fr-FR units approved")
		}
	}
	// The approved unit left the queue.
	for _, it := range after.Review {
		if it.Locale == "fr-FR" {
			assert.NotEqual(t, item.Key, it.Key, "the approved unit is gone")
		}
	}
}

func TestApproveReviewItem_UnknownTab(t *testing.T) {
	app := NewApp()
	err := app.ApproveReviewItem("nope", "fr-FR", "f.json", "k")
	require.Error(t, err)
}
