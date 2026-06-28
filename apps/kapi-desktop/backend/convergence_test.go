package backend

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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
		ShipGate: gate.Gate{"translated": 100},
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

func TestBringUpToDate_LaunchesDefaultFlow(t *testing.T) {
	app := NewApp()
	tab, _ := newConvergenceProject(t, app)

	// BringUpToDate resolves the project's content + locales and launches the
	// default flow; the run reaches a terminal state. (The pseudo preview flow
	// writes its own qps locale, so we assert wiring, not target coverage — a
	// real translate/recycle default flow honors the project's target locales.)
	require.NoError(t, app.BringUpToDate(tab.ID))
	require.Eventually(t, func() bool {
		s := app.GetRunState()
		return s == string(RunStateComplete) || s == string(RunStateError)
	}, 20*time.Second, 50*time.Millisecond, "the default-flow run should reach a terminal state")
	assert.Equal(t, string(RunStateComplete), app.GetRunState(), "the pseudo default flow completes")

	// The report is still derivable after a run.
	_, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
}

func TestBringUpToDate_NoDefaultFlow(t *testing.T) {
	app := NewApp()
	tab, _ := newCoverageProject(t, app) // no defaults.flow
	err := app.BringUpToDate(tab.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default flow")
}

func TestGetConvergence_UnknownTab(t *testing.T) {
	app := NewApp()
	_, err := app.GetConvergence("nope")
	require.Error(t, err)
}
