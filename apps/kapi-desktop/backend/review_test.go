package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newReviewProject scaffolds a project with a placeholder-bearing source, a
// clean fr-FR translation, and a de-DE translation that drops the placeholder
// (a check finding), then opens it.
func newReviewProject(t *testing.T, app *App) (*TabInfo, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greeting":"Hello {name}","farewell":"Goodbye"}`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Bonjour {name}","farewell":"Au revoir"}`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "locales", "de-DE.json"),
		[]byte(`{"greeting":"Hallo","farewell":"Tschüss"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Review",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
		Collections: []project.Collection{{
			Name:    "App",
			Content: []project.ContentItem{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
		}},
		ShipGate: gate.Gate{"translated": {Pct: 100}, "reviewed": {Pct: 50}},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab, root
}

func TestGetReviewUnit_ReturnsTextsAndCleanFindings(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)

	d, err := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting")
	require.NoError(t, err)
	assert.Equal(t, "Hello {name}", d.Source)
	assert.Equal(t, "Bonjour {name}", d.Target)
	assert.Equal(t, "greeting", d.Key)
	assert.Equal(t, "App", d.Collection)
	assert.Equal(t, "translated", d.Status, "no decision recorded yet — presence baseline")
	assert.Empty(t, d.Findings, "the fr-FR translation keeps the placeholder")
	assert.True(t, d.Editable, "a plain JSON string is editable")
}

func TestGetReviewUnit_SurfacesPlaceholderFinding(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)

	d, err := app.GetReviewUnit(tab.ID, "de-DE", filepath.Join("locales", "de-DE.json"), "greeting")
	require.NoError(t, err)
	assert.Equal(t, "Hallo", d.Target)
	require.NotEmpty(t, d.Findings, "the de-DE translation dropped {name}")
	assert.Equal(t, "target", d.Findings[0].Field)
	assert.Equal(t, "de-DE", d.Findings[0].Locale, "a target-side finding carries the checked target locale")
}

func TestGetReviewUnit_NotFound(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)
	_, err := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "missing")
	require.Error(t, err)
	_, err = app.GetReviewUnit(tab.ID, "fr-FR", "nope.json", "greeting")
	require.Error(t, err)
	_, err = app.GetReviewUnit("no-tab", "fr-FR", "locales/fr-FR.json", "greeting")
	require.Error(t, err)
}

func TestGetReviewQueue_MarksFindings(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)

	items, err := app.GetReviewQueue(tab.ID)
	require.NoError(t, err)
	require.Len(t, items, 4, "two units × two locales await review")

	byID := map[string]host.ReviewItem{}
	for _, it := range items {
		byID[it.Locale+":"+it.Key] = it
	}
	de := byID["de-DE:greeting"]
	require.NotNil(t, de.HasFindings, "queue items carry the findings marker")
	assert.True(t, *de.HasFindings, "de-DE greeting dropped its placeholder")
	assert.Equal(t, "en-US", de.SourceLocale, "items carry the project's source locale")
	fr := byID["fr-FR:greeting"]
	require.NotNil(t, fr.HasFindings)
	assert.False(t, *fr.HasFindings, "fr-FR greeting is clean")
	assert.Equal(t, "App", fr.Collection, "items carry their collection")
}

func TestRejectReviewItem_SendsUnitBackToDraft(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)
	file := filepath.Join("locales", "fr-FR.json")

	require.NoError(t, app.RejectReviewItem(tab.ID, "fr-FR", file, "farewell", "too literal"))

	// The rejected unit left the review queue…
	rep, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	for _, it := range rep.Review {
		if it.Locale == "fr-FR" {
			assert.NotEqual(t, "farewell", it.Key, "the rejected unit is out of the review queue")
		}
	}
	// …and reads draft with its note on the unit detail.
	d, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "farewell")
	require.NoError(t, err)
	assert.Equal(t, "draft", d.Status)
	assert.Equal(t, "rejected", d.ReviewState)
	assert.Equal(t, "too literal", d.Note)
}

func TestSignOffReviewItem_TopRung(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)
	file := filepath.Join("locales", "fr-FR.json")

	require.NoError(t, app.SignOffReviewItem(tab.ID, "fr-FR", file, "greeting"))

	rep, err := app.GetConvergence(tab.ID)
	require.NoError(t, err)
	for _, lc := range rep.Locales {
		if lc.Locale == "fr-FR" {
			assert.Equal(t, 50, lc.Pct["signed-off"], "1 of 2 fr-FR units signed off")
		}
	}
	d, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "signed-off", d.Status)
}

func TestUpdateReviewTarget_EditsFileAndInvalidatesDecision(t *testing.T) {
	app := NewApp()
	tab, root := newReviewProject(t, app)
	file := filepath.Join("locales", "fr-FR.json")

	// Approve first, then edit: the hash-bound approval must go stale.
	require.NoError(t, app.ApproveReviewItem(tab.ID, "fr-FR", file, "greeting"))
	d, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "reviewed", d.Status)

	require.NoError(t, app.UpdateReviewTarget(tab.ID, "fr-FR", file, "greeting", "Salut {name}"))

	data, err := os.ReadFile(filepath.Join(root, "locales", "fr-FR.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Salut {name}", "the edit landed in the target file")
	assert.Contains(t, string(data), "Au revoir", "untouched units are preserved")

	d2, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "Salut {name}", d2.Target)
	assert.Equal(t, "translated", d2.Status, "the prior approval no longer judges the edited text")

	// The unit re-entered the review queue for its new text; approving again
	// blesses the edit.
	require.NoError(t, app.ApproveReviewItem(tab.ID, "fr-FR", file, "greeting"))
	d3, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "reviewed", d3.Status)
}

func TestUpdateReviewTarget_RefusesEmptyText(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)
	err := app.UpdateReviewTarget(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting", "   ")
	require.Error(t, err)
}
