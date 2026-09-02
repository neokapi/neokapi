package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
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

// TestToDesktopFinding_CarriesTheAnchor guards the run range on its way to the
// panel: a checker that says which words it objects to must be able to have
// them underlined rather than described.
func TestToDesktopFinding_CarriesTheAnchor(t *testing.T) {
	anchor := model.Anchor{
		Kind:  model.AnchorRange,
		Start: model.RunPos{Run: 0, Offset: 4},
		End:   model.RunPos{Run: 0, Offset: 11},
	}
	f := check.Finding{
		Category:     "terminology",
		Severity:     check.SeverityMajor,
		Message:      "say `use` instead of `utilize`",
		OriginalText: "utilize",
		Position:     anchor,
	}
	b := &model.Block{ID: "b1", Translatable: true}

	got := toDesktopFinding(f, b, "target", "nb", ContextPointDTO{})
	assert.Equal(t, anchor, got.Position)
}

// TestGetReviewUnit_CarriesTheReviewContext holds the bar the review model
// exists for: a reviewer sees at least what the model was told. The prompt
// carries a block's key and its neighbours; so does the unit detail.
func TestGetReviewUnit_CarriesTheReviewContext(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)

	d, err := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting")
	require.NoError(t, err)
	require.NotNil(t, d.Context)

	t.Run("the point names where the content sits", func(t *testing.T) {
		assert.Equal(t, "App", d.Context.Point.Collection)
		assert.Equal(t, filepath.ToSlash(filepath.Join("locales", "en.json")), d.Context.Point.Path)
	})

	t.Run("the neighbourhood keeps document order", func(t *testing.T) {
		assert.Equal(t, "greeting", d.Context.Neighbourhood.Key)
		assert.Equal(t, host.DefaultReviewWindow, d.Context.Neighbourhood.Window)
		assert.Empty(t, d.Context.Neighbourhood.Before, "greeting is the first unit in the file")
		require.Len(t, d.Context.Neighbourhood.After, 1)
		assert.Equal(t, "farewell", d.Context.Neighbourhood.After[0].Key)
		assert.NotEmpty(t, d.Context.Neighbourhood.After[0].Source, "a neighbour travels as runs")
	})

	t.Run("the second unit sees the first behind it", func(t *testing.T) {
		later, lerr := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "farewell")
		require.NoError(t, lerr)
		require.NotNil(t, later.Context)
		require.Len(t, later.Context.Neighbourhood.Before, 1)
		assert.Equal(t, "greeting", later.Context.Neighbourhood.Before[0].Key)
		assert.Empty(t, later.Context.Neighbourhood.After, "farewell is the last unit in the file")
	})

	t.Run("provenance is empty before anyone decides", func(t *testing.T) {
		assert.Empty(t, d.Context.Provenance.ReviewState)
		assert.Empty(t, d.Context.Provenance.By)
	})
}

// TestGetReviewUnit_DirectoryMirrorTarget guards a real bug: a collection
// whose target is a bare directory mirror ("{lang}", no {filename}/{relpath}/*
// token) — kapimart's "Contracts" collection (`base: legal`, `target: "{lang}"`)
// is exactly this shape. findReviewSource used to reconstruct the target path
// with a {lang}/* -only substitution that can't produce a directory-mirror
// path at all, so opening the Review page on such a project failed with
// `no content file resolves to target "legal/ar/pricing-schedule.json" (ar)`.
func TestGetReviewUnit_DirectoryMirrorTarget(t *testing.T) {
	app := NewApp()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "legal", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "legal", "ar"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "legal", "en", "pricing-schedule.json"),
		[]byte(`{"clause":"Net 30 days"}`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "legal", "ar", "pricing-schedule.json"),
		[]byte(`{"clause":"صافي 30 يومًا"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "Contracts",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"ar"},
		},
		Collections: []project.Collection{{
			Name: "Contracts",
			Base: "legal",
			Content: []project.ContentItem{{
				Path:   "en/*.json",
				Target: "{lang}",
			}},
		}},
	}
	path := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(path, proj))

	tab, err := app.OpenProject(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	// Exactly the path a directory-mirror target resolves to, and exactly
	// what the review queue's File field would list.
	d, err := app.GetReviewUnit(tab.ID, "ar", filepath.Join("legal", "ar", "pricing-schedule.json"), "clause")
	require.NoError(t, err)
	assert.Equal(t, "Net 30 days", d.Source)
	assert.Equal(t, "صافي 30 يومًا", d.Target)
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

	items, err := app.GetReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	require.Len(t, items, 4, "two units × two locales await review")

	byID := map[string]host.ReviewQueueItem{}
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

// TestUpdateReviewTarget_RecordsAHumanOrigin: the reviewer who rewrites an AI
// draft produced the wording in front of them, and the review model's
// provenance layer is where that is read. The edit records production and no
// decision, so the unit waits in the queue for an explicit approval of the text
// the reviewer typed.
func TestUpdateReviewTarget_RecordsAHumanOrigin(t *testing.T) {
	app := NewApp()
	tab, root := newReviewProject(t, app)
	file := filepath.Join("locales", "fr-FR.json")
	ctx := context.Background()

	// The record a convergence pass leaves for its own output: the source it
	// translated, the translation it wrote, and the model that wrote it.
	st, err := app.hostEngine().OpenProjectState(ctx, root)
	require.NoError(t, err)
	scope := app.hostEngine().DocumentScope(ctx, root, filepath.Join(root, "locales", "en.json"))
	require.NoError(t, st.Record(ctx, state.UnitState{
		Unit:        "greeting",
		Variant:     model.Variant("fr-FR"),
		Scope:       scope,
		Status:      model.TargetStatusTranslated,
		Origin:      model.Origin{Kind: model.OriginAI, Engine: "claude", Timestamp: "2026-09-01T10:00:00Z"},
		TargetHash:  state.TargetHash("Bonjour {name}"),
		ContentHash: state.SourceHash("Hello {name}"),
	}))

	asDrafted, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	require.NotNil(t, asDrafted.Context)
	require.NotNil(t, asDrafted.Context.Provenance.Origin)
	require.Equal(t, model.OriginAI, asDrafted.Context.Provenance.Origin.Kind)

	require.NoError(t, app.UpdateReviewTarget(tab.ID, "fr-FR", file, "greeting", "Salut {name}"))

	edited, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	require.NotNil(t, edited.Context)
	require.NotNil(t, edited.Context.Provenance.Origin)
	assert.Equal(t, model.OriginHuman, edited.Context.Provenance.Origin.Kind)
	assert.Empty(t, edited.Context.Provenance.Origin.Engine, "the model that drafted it wrote none of this wording")
	assert.NotEmpty(t, edited.Context.Provenance.Origin.Timestamp)

	// Production, with no verdict on it.
	assert.Empty(t, edited.Context.Provenance.ReviewState)
	assert.Empty(t, edited.ReviewState)
	assert.Equal(t, "translated", edited.Status)

	queue, err := app.GetReviewQueue(tab.ID, ProjectFilter{Languages: []string{"fr-FR"}})
	require.NoError(t, err)
	pending := false
	for _, it := range queue {
		if it.Key == "greeting" && it.Locale == "fr-FR" {
			pending = true
		}
	}
	assert.True(t, pending, "a hand edit leaves the unit in the review queue")
}

func TestUpdateReviewTarget_RefusesEmptyText(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)
	err := app.UpdateReviewTarget(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting", "   ")
	require.Error(t, err)
}

// The menu-bar Active Filter governs the Checks panel; a Review queue that
// ignored it left two controls that look alike behaving differently, and made
// the page enrich (and run every checker over) thousands of units the user had
// already narrowed away.
func TestGetReviewQueue_HonoursTheActiveFilter(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, _ := newReviewProject(t, app)

	all, err := app.GetReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, all)

	locales := map[string]bool{}
	for _, it := range all {
		locales[it.Locale] = true
	}
	require.Greater(t, len(locales), 1, "the fixture needs more than one locale to narrow")

	only, err := app.GetReviewQueue(tab.ID, ProjectFilter{Languages: []string{"fr-FR"}})
	require.NoError(t, err)
	require.NotEmpty(t, only)
	for _, it := range only {
		assert.Equal(t, "fr-FR", it.Locale)
	}
	assert.Less(t, len(only), len(all))

	// A glob is written about the content, so it matches the SOURCE path.
	for _, it := range all {
		assert.Equal(t, "locales/en.json", it.Relative, "an item carries its source-relative path")
	}
	onSource, err := app.GetReviewQueue(tab.ID, ProjectFilter{Glob: "locales/en.json"})
	require.NoError(t, err)
	assert.Len(t, onSource, len(all), "a glob naming the source keeps every item")

	// The same glob written against a TARGET path matches nothing. This is the
	// assertion that distinguishes the two: locales/de-DE.json is a real file in
	// this project, and a queue matching the target path would return its units.
	onTarget, err := app.GetReviewQueue(tab.ID, ProjectFilter{Glob: "locales/de-DE.json"})
	require.NoError(t, err)
	assert.Empty(t, onTarget, "the glob is matched against the source, never the target")
}
