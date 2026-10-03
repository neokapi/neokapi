package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workhome"
)

// A parked locale's drafts live in the workspace home (core/workhome): the
// project's operation log, outside the checkout. These tests hold the cycle
// together: a gated run keeps what it withholds there, every read surface
// and every change set finds it there, a delivery moves it into its file, and
// a checkout whose `.kapi/work/` is deleted loses none of it and pays no
// provider to serve it again.

// withheldProject is a project of JSON documents whose recipe withholds
// delivery until a locale clears its ship gate.
func withheldProject(t *testing.T, files map[string]string) (*App, string) {
	t.Helper()
	a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.json", Target: "out/{lang}/{path}.json"}, files,
		func(p *project.KapiProject) { p.Defaults.Materialize = project.MaterializeOnConverge })
	t.Cleanup(a.Shutdown)
	return a, recipe
}

// targetRef is the file the recipe names for doc's translation into locale.
func targetRef(t *testing.T, a *App, recipe, doc, locale string) string {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	units, err := a.UnitsFromProject(proj, filepath.Dir(recipe), locale)
	require.NoError(t, err)
	for _, u := range units {
		if rel, _ := filepath.Rel(filepath.Dir(recipe), u.SourcePath); filepath.ToSlash(rel) == doc && u.Locale == locale {
			return filepath.ToSlash(u.DisplayPath)
		}
	}
	t.Fatalf("the recipe names no %s file for %s", locale, doc)
	return ""
}

// draftGerman gives the German edition of each named block of doc its text,
// as a person's change set creating each.
func draftGerman(t *testing.T, svc *change.Service, doc string, texts map[string]string) *change.Result {
	t.Helper()
	var ops []change.Op
	for key, text := range texts {
		ops = append(ops, setTo(change.Ref{Doc: doc, Block: key, Edition: model.EditionKey{Locale: "de"}}, model.AbsentRevision, text))
	}
	res, err := svc.Apply(context.Background(), change.Set{Ops: ops}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	return res
}

// keepGerman writes drafts of the German edition of doc into the workspace
// home, as a gated run keeps the drafts of a locale it parks.
func keepGerman(t *testing.T, a *App, recipe, doc string, texts map[string]string) {
	t.Helper()
	ctx := context.Background()
	h, err := a.keptEditions(filepath.Dir(recipe)).open(ctx, true)
	require.NoError(t, err)
	require.NotNil(t, h)
	p := workhome.Produce{Doc: doc, Edition: model.EditionKey{Locale: "de"},
		Actor: change.Actor{Kind: change.ActorTool, Name: "translate"}, Origin: "flow:translate"}
	for key, text := range texts {
		p.Blocks = append(p.Blocks, workhome.Produced{Block: key, Before: model.AbsentRevision,
			Edition: model.Edition{Runs: []model.Run{model.TextR(text)}, Status: model.Status(model.TargetStatusDraft)}})
	}
	res, err := h.Produce(ctx, p)
	require.NoError(t, err)
	require.Equal(t, len(texts), res.Written)
}

// keptGerman reads what the workspace home keeps of the German edition of
// doc, by block.
func keptGerman(t *testing.T, a *App, recipe, doc string) map[string]string {
	t.Helper()
	kept, err := a.keptEditions(filepath.Dir(recipe)).Edition(context.Background(), doc, model.EditionKey{Locale: "de"})
	require.NoError(t, err)
	out := map[string]string{}
	for key, ed := range kept.Blocks {
		out[key] = model.RunsText(ed.Runs)
	}
	return out
}

// TestChangeService_ConformanceOnTheWorkspaceHome runs the conformance suite
// on the editions a project keeps in the workspace home: each document of the
// suite is the German file of a source the recipe withholds, which no file
// holds yet.
func TestChangeService_ConformanceOnTheWorkspaceHome(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		a, recipe := withheldProject(t, map[string]string{
			"docs/a.json": `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			"docs/b.json": `{"title": "Welcome"}` + "\n",
		})
		var hook func(string)
		svc, err := a.ChangeService(t.Context(), ChangeServiceOptions{Project: recipe, Origin: "test", BeforeSettle: func(doc string) {
			if hook != nil {
				hook(doc)
			}
		}})
		require.NoError(t, err)
		keepGerman(t, a, recipe, "docs/a.json", map[string]string{"greeting": "Hallo", "farewell": "Tschüss", "thanks": "Danke"})
		keepGerman(t, a, recipe, "docs/b.json", map[string]string{"title": "Willkommen"})
		docA, docB := targetRef(t, a, recipe, "docs/a.json", "de"), targetRef(t, a, recipe, "docs/b.json", "de")
		return changetest.Env{
			Service:         svc,
			SetBeforeSettle: func(fn func(string)) { hook = fn },
			DocA:            docA,
			DocB:            docB,
			Snapshot: func(t *testing.T, doc string) []byte {
				source := "docs/a.json"
				if doc == docB {
					source = "docs/b.json"
				}
				data, err := json.Marshal(keptGerman(t, a, recipe, source))
				require.NoError(t, err)
				return data
			},
		}
	})
}

// TestChangeService_EditsAKeptEditionInTheWorkspaceHome: a change to a
// translation the workspace home keeps, whose file does not exist, lands in
// the workspace home and writes no file, and every read finds it there.
func TestChangeService_EditsAKeptEditionInTheWorkspaceHome(t *testing.T) {
	ctx := context.Background()
	a, recipe := withheldProject(t, map[string]string{"docs/a.json": `{"greeting": "Hello there"}` + "\n"})
	keepGerman(t, a, recipe, "docs/a.json", map[string]string{"greeting": "Hallo"})
	svc := changeService(t, a, recipe)

	target := targetRef(t, a, recipe, "docs/a.json", "de")
	page, err := svc.Read(ctx, change.ReadRequest{Doc: target})
	require.NoError(t, err)
	assert.Equal(t, workhome.Name, page.Home)
	require.Len(t, page.Blocks, 1)
	assert.Equal(t, "Hallo", page.Blocks[0].Text, "a read of the translation's file reads the kept edition")

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Guten Tag")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	_, err = os.Stat(filepath.Join(filepath.Dir(recipe), filepath.FromSlash(target)))
	assert.True(t, os.IsNotExist(err), "the kept translation reaches no file")
	homes := map[string]string{}
	for _, d := range res.Docs {
		homes[d.Edition] = d.Home
	}
	assert.Equal(t, workhome.Name, homes["de"], "the result names the workspace home")
	assert.Equal(t, map[string]string{"greeting": "Guten Tag"}, keptGerman(t, a, recipe, "docs/a.json"))

	hist, err := svc.History(ctx, change.HistoryRequest{Ref: change.Ref{Doc: "docs/a.json", Block: "greeting", Edition: model.EditionKey{Locale: "de"}}})
	require.NoError(t, err)
	require.Len(t, hist.Entries, 2, "the draft kept, and the edit, each recorded once by the commit that kept it")
	require.NotNil(t, res.Record)
	assert.Equal(t, *res.Record, hist.Entries[0].Record)
	require.NotNil(t, hist.Entries[0].Actor)
	assert.Equal(t, change.ActorPerson, hist.Entries[0].Actor.Kind)
}

// TestChangeService_AnEditionWithAFileIsNeverKept is the guard against the
// workspace home shadowing a file: once the translation's file exists, the
// file is the edition's home, and whatever the workspace still keeps of it is
// neither read nor written.
func TestChangeService_AnEditionWithAFileIsNeverKept(t *testing.T) {
	ctx := context.Background()
	a, recipe := withheldProject(t, map[string]string{"docs/a.json": `{"greeting": "Hello there"}` + "\n"})
	keepGerman(t, a, recipe, "docs/a.json", map[string]string{"greeting": "Hallo"})
	svc := changeService(t, a, recipe)

	target := targetRef(t, a, recipe, "docs/a.json", "de")
	path := filepath.Join(filepath.Dir(recipe), filepath.FromSlash(target))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`{"greeting": "Guten Tag"}`+"\n"), 0o644))

	page, err := svc.Read(ctx, change.ReadRequest{Doc: target})
	require.NoError(t, err)
	assert.Equal(t, "file", page.Home)
	require.Len(t, page.Blocks, 1)
	assert.Equal(t, "Guten Tag", page.Blocks[0].Text, "the file holds the edition")

	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Servus")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Contains(t, readFile(t, recipe, target), "Servus", "the change lands in the file")
	assert.Equal(t, map[string]string{"greeting": "Hallo"}, keptGerman(t, a, recipe, "docs/a.json"),
		"and the workspace home is left as it was")
}

// TestChangeService_AnEditionNoHomeHoldsIsWrittenToItsFile is the boundary:
// a translation with neither a file nor a kept draft is written to its file,
// as a delivery writes it, whatever the recipe's delivery policy.
func TestChangeService_AnEditionNoHomeHoldsIsWrittenToItsFile(t *testing.T) {
	for _, materialize := range []string{project.MaterializeManual, project.MaterializeOnConverge} {
		t.Run(materialize, func(t *testing.T) {
			a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.json", Target: "out/{lang}/{path}.json"},
				map[string]string{"docs/a.json": `{"greeting": "Hello there"}` + "\n"},
				func(p *project.KapiProject) { p.Defaults.Materialize = materialize })
			t.Cleanup(a.Shutdown)
			draftGerman(t, changeService(t, a, recipe), "docs/a.json", map[string]string{"greeting": "Hallo"})
			target := targetRef(t, a, recipe, "docs/a.json", "de")
			assert.Contains(t, readFile(t, recipe, target), "Hallo")
			assert.Empty(t, keptGerman(t, a, recipe, "docs/a.json"))
		})
	}
}
