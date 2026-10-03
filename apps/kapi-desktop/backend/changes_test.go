package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
)

// bindingService carries the change contract to the desktop's bindings as the
// frontend does, as JSON strings, so the conformance suite runs through Read
// and Apply rather than the service behind them. The actor a case names is not
// sent: the desktop applies every change set as the person at the keyboard.
type bindingService struct {
	app *App
	tab string
}

var _ changetest.Service = bindingService{}

func (b bindingService) Read(_ context.Context, q change.ReadRequest) (*change.Page, error) {
	req, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	raw, err := b.app.Read(b.tab, string(req))
	if err != nil {
		return nil, err
	}
	var page change.Page
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (b bindingService) Apply(_ context.Context, set change.Set, _ change.Actor) (*change.Result, error) {
	body, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	raw, err := b.app.Apply(b.tab, string(body))
	if err != nil {
		return nil, err
	}
	var res change.Result
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// changeProject opens a project whose content is files, each a source the
// recipe claims under paths, with targets at target ("" for none).
func changeProject(t *testing.T, app *App, files map[string]string, paths []string, target string) (tabID, root string) {
	t.Helper()
	var items []project.ContentItem
	for _, p := range paths {
		items = append(items, project.ContentItem{Path: p, Target: target})
	}
	return changeProjectOf(t, app, files, items)
}

// changeProjectOf opens a project whose content is files, claimed by items.
func changeProjectOf(t *testing.T, app *App, files map[string]string, items []project.ContentItem) (tabID, root string) {
	t.Helper()
	root = t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	proj := &project.KapiProject{
		Version:     project.CurrentVersion,
		Name:        "Changes",
		Defaults:    project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []project.Collection{{Name: "app", Content: items}},
	}
	recipe := filepath.Join(root, "project.kapi")
	require.NoError(t, project.Save(recipe, proj))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, root
}

// TestChangeBindings_Conformance runs the change service's conformance suite
// through the desktop's Read and Apply bindings, over the project's file home.
func TestChangeBindings_Conformance(t *testing.T) {
	changetest.Run(t, func(t *testing.T) changetest.Env {
		app := NewApp()
		catalog := func(lang, msgstr string) string {
			return "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: " + lang + "\\n\"\n\nmsgid \"Hello there\"\nmsgstr \"" + msgstr + "\"\n"
		}
		tab, root := changeProjectOf(t, app, map[string]string{
			"a.json": `{"greeting": "Hello there", "farewell": "Goodbye now", "thanks": "Thank you"}` + "\n",
			"b.json": `{"title": "Welcome"}` + "\n",
			// A catalog whose French translation is the catalog beside it.
			"po/en.po": catalog("en", ""),
			"po/fr.po": catalog("fr", "Bonjour"),
		}, []project.ContentItem{
			{Path: "a.json"}, {Path: "b.json"},
			{Path: "po/en.po", Format: &project.FormatSpec{Name: "po"}, Target: "po/{lang}.po"},
		})
		return changetest.Env{
			Service:    bindingService{app: app, tab: tab},
			DocA:       "a.json",
			DocB:       "b.json",
			Translated: "po/en.po",
			Snapshot: func(t *testing.T, doc string) []byte {
				b, err := os.ReadFile(filepath.Join(root, doc))
				require.NoError(t, err)
				return b
			},
			Mode: func(t *testing.T, doc string) os.FileMode {
				info, err := os.Stat(filepath.Join(root, doc))
				require.NoError(t, err)
				return info.Mode().Perm()
			},
		}
	})
}

// readVia reads a page through the Read binding.
func readVia(t *testing.T, app *App, tab string, q change.ReadRequest) change.Page {
	t.Helper()
	page, err := bindingService{app: app, tab: tab}.Read(t.Context(), q)
	require.NoError(t, err)
	return *page
}

// blockVia reads one block of doc through the Read binding.
func blockVia(t *testing.T, app *App, tab, doc, key string) change.BlockRead {
	t.Helper()
	page := readVia(t, app, tab, change.ReadRequest{Doc: doc, Blocks: []string{key}})
	require.Len(t, page.Blocks, 1, "%s holds one block keyed %s", doc, key)
	return page.Blocks[0]
}

// applyVia applies ops through the Apply binding.
func applyVia(t *testing.T, app *App, tab string, set change.Set) change.Result {
	t.Helper()
	res, err := bindingService{app: app, tab: tab}.Apply(t.Context(), set, change.Actor{})
	require.NoError(t, err)
	return *res
}

// historyVia reads an edition's history through the History binding.
func historyVia(t *testing.T, app *App, tab string, ref change.Ref) change.History {
	t.Helper()
	req, err := json.Marshal(change.HistoryRequest{Ref: ref})
	require.NoError(t, err)
	raw, err := app.History(tab, string(req))
	require.NoError(t, err)
	var h change.History
	require.NoError(t, json.Unmarshal([]byte(raw), &h))
	return h
}

func setText(ref change.Ref, rev, text string) change.Op {
	return change.Op{Kind: change.KindSetContent, At: ref, IfMatch: rev, Body: &change.SetContent{Text: &text}}
}

func decideOp(ref change.Ref, rev string, outcome change.Outcome) change.Op {
	return change.Op{Kind: change.KindDecide, At: ref, IfMatch: rev, Body: &change.Decide{Outcome: outcome}}
}

// A translation is edited from its own file, as the review pane reads it: the
// reference resolves to the edition of the source, the file keeps every unit
// the edit did not touch, the history names the person and the desktop, and
// the review model reads the edit as a person's with no decision on it.
func TestApply_EditsATranslationThroughItsFile(t *testing.T) {
	app := NewApp()
	tab, root := newReviewProject(t, app)
	fr := blockVia(t, app, tab.ID, "locales/fr-FR.json", "greeting")
	require.Equal(t, change.Ref{Doc: "locales/en.json", Block: "greeting", Edition: model.EditionKey{Locale: "fr-FR"}}, fr.Ref,
		"a read of the translation's file names the edition of the source")

	res := applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{setText(fr.Ref, fr.Rev, strings.Replace(fr.Text, "Bonjour", "Salut", 1))}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Record, "the edit is recorded")

	data, err := os.ReadFile(filepath.Join(root, "locales", "fr-FR.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Salut {name}", "the edit landed with the placeholder kept")
	assert.Contains(t, string(data), "Au revoir", "the unit nobody edited is kept")

	h := historyVia(t, app, tab.ID, fr.Ref)
	require.Len(t, h.Entries, 1)
	require.NotNil(t, h.Entries[0].Actor)
	assert.Equal(t, change.ActorPerson, h.Entries[0].Actor.Kind)
	assert.Equal(t, "desktop", h.Entries[0].Origin)
	assert.Equal(t, fr.Rev, h.Entries[0].Before)
	assert.Equal(t, h.Rev, h.Entries[0].After)

	d, err := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting")
	require.NoError(t, err)
	assert.Equal(t, "Salut {name}", d.Target)
	assert.Equal(t, "translated", d.Status)
	require.NotNil(t, d.Context)
	require.NotNil(t, d.Context.Provenance.Origin)
	assert.Equal(t, model.OriginHuman, d.Context.Provenance.Origin.Kind, "the reviewer wrote the wording in front of them")
	assert.Empty(t, d.Context.Provenance.ReviewState, "an edit records no decision")
}

// Approve and Reject are decide operations bound to the revision the reviewer
// read: approve establishes, reject sends back to draft with the note, and an
// edit after an approval leaves the unit to be approved again.
func TestApply_DecideRecordsReviewDecisions(t *testing.T) {
	app := NewApp()
	tab, _ := newReviewProject(t, app)
	file := filepath.Join("locales", "fr-FR.json")

	greeting := blockVia(t, app, tab.ID, "locales/fr-FR.json", "greeting")
	res := applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{decideOp(greeting.Ref, greeting.Rev, change.OutcomeEstablish)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	d, err := app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "established", d.Status)

	farewell := blockVia(t, app, tab.ID, "locales/fr-FR.json", "farewell")
	res = applyVia(t, app, tab.ID, change.Set{Note: "too literal", Ops: []change.Op{decideOp(farewell.Ref, farewell.Rev, change.OutcomeReject)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	d, err = app.GetReviewUnit(tab.ID, "fr-FR", file, "farewell")
	require.NoError(t, err)
	assert.Equal(t, "draft", d.Status)
	assert.Equal(t, "rejected", d.ReviewState)
	assert.Equal(t, "too literal", d.Note)
	queue, err := app.ReviewQueue(tab.ID, ProjectFilter{Languages: []string{"fr-FR"}})
	require.NoError(t, err)
	for _, it := range queue.Pending {
		assert.NotEqual(t, "farewell", it.Key, "the rejected unit left the review queue")
	}

	// An edit after the approval: the approval judged text that is gone.
	greeting = blockVia(t, app, tab.ID, "locales/fr-FR.json", "greeting")
	res = applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{setText(greeting.Ref, greeting.Rev, strings.Replace(greeting.Text, "Bonjour", "Salut", 1))}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	d, err = app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "translated", d.Status, "the prior approval no longer judges the edited text")

	greeting = blockVia(t, app, tab.ID, "locales/fr-FR.json", "greeting")
	res = applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{decideOp(greeting.Ref, greeting.Rev, change.OutcomeEstablish)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	d, err = app.GetReviewUnit(tab.ID, "fr-FR", file, "greeting")
	require.NoError(t, err)
	assert.Equal(t, "established", d.Status, "approving again approves the edit")
}

// A decision names the revision the reviewer read; one that changed since is
// refused with the content now there, and nothing is recorded.
func TestApply_ADecisionOnAChangedTranslationIsStale(t *testing.T) {
	app := NewApp()
	tab, root := newReviewProject(t, app)
	greeting := blockVia(t, app, tab.ID, "locales/fr-FR.json", "greeting")
	require.NoError(t, os.WriteFile(filepath.Join(root, "locales", "fr-FR.json"),
		[]byte(`{"greeting":"Coucou {name}","farewell":"Au revoir"}`), 0o644))

	res := applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{decideOp(greeting.Ref, greeting.Rev, change.OutcomeEstablish)}})
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current, "a stale refusal carries the edition as it stands")
	assert.Contains(t, res.Ops[0].Current.Text, "Coucou")
	d, err := app.GetReviewUnit(tab.ID, "fr-FR", filepath.Join("locales", "fr-FR.json"), "greeting")
	require.NoError(t, err)
	assert.Equal(t, "translated", d.Status, "nothing was approved")
}

// An edit made against a revision another writer has moved past is refused
// with the current text, and the file keeps the other writer's change.
func TestApply_AStaleEditCarriesTheCurrentText(t *testing.T) {
	app := NewApp()
	tab, root := changeProject(t, app, map[string]string{"en.json": `{"title": "Welcome"}` + "\n"}, []string{"en.json"}, "")
	title := blockVia(t, app, tab, "en.json", "title")
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"), []byte(`{"title": "Welcome aboard"}`+"\n"), 0o644))

	res := applyVia(t, app, tab, change.Set{Ops: []change.Op{setText(title.Ref, title.Rev, "Hello")}})
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeStale, res.Ops[0].Error.Code)
	require.NotNil(t, res.Ops[0].Current)
	assert.Equal(t, "Welcome aboard", res.Ops[0].Current.Text)

	// Re-applying over the current revision is the person's choice, and lands.
	res = applyVia(t, app, tab, change.Set{Ops: []change.Op{setText(title.Ref, res.Ops[0].Current.Rev, "Hello")}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	data, err := os.ReadFile(filepath.Join(root, "en.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"Hello"`)
}

// A formatted paragraph is edited in placeholder form, so its inline codes
// survive the edit: the plain-text rewrite the desktop once had refused it.
func TestApply_AFormattedParagraphKeepsItsCodes(t *testing.T) {
	app := NewApp()
	tab, root := changeProject(t, app, map[string]string{
		"page.html": `<html><body><p>Please <b>utilize</b> the <a href="https://a.example/">guide</a>.</p></body></html>` + "\n",
	}, []string{"page.html"}, "")
	page := readVia(t, app, tab, change.ReadRequest{Doc: "page.html"})
	var para *change.BlockRead
	for i, b := range page.Blocks {
		if strings.Contains(b.Text, "utilize") {
			para = &page.Blocks[i]
		}
	}
	require.NotNil(t, para)
	require.NotEmpty(t, para.Codes, "the paragraph's markup reads as codes")

	edited := strings.Replace(para.Text, "utilize", "use", 1)
	res := applyVia(t, app, tab, change.Set{Ops: []change.Op{setText(para.Ref, para.Rev, edited)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	data, err := os.ReadFile(filepath.Join(root, "page.html"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `<p>Please <b>use</b> the <a href="https://a.example/">guide</a>.</p>`)
}

// pluralCatalog is a KBF catalog holding one block whose source is an ICU
// plural: "Your cart is empty", "1 item in your cart", "{count} items in your
// cart".
func pluralCatalog(t *testing.T) []byte {
	t.Helper()
	f := &kbf.File{
		SchemaVersion: kbf.SchemaVersion,
		Kind:          kbf.Kind,
		Created:       "2026-10-03T09:00:00Z",
		Generator:     kbf.GeneratorInfo{ID: "desktop-test", Version: "0.0.1"},
		Project:       kbf.ProjectInfo{ID: "desktop-test", SourceLocale: "en"},
		Documents: []kbf.Document{{
			ID: "cart", DocumentType: kbf.DocumentTypeJSX, Path: "src/Cart.tsx",
			Blocks: []kbf.Block{{
				ID: "cart-count", Hash: "h1", Translatable: true, Type: kbf.BlockTypeJSXElement,
				Source: []kbf.Run{{Plural: &kbf.PluralRun{Pivot: "count", Forms: map[kbf.PluralForm][]kbf.Run{
					kbf.PluralZero: {{Text: &kbf.TextRun{Text: "Your cart is empty"}}},
					kbf.PluralOne:  {{Text: &kbf.TextRun{Text: "1 item in your cart"}}},
					kbf.PluralOther: {
						{Ph: &kbf.PlaceholderRun{ID: "1", Type: "jsx:var", SubType: "number", Data: "{count}", Equiv: "count", Disp: "count"}},
						{Text: &kbf.TextRun{Text: " items in your cart"}},
					},
				}}}},
			}},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, kbf.Encode(&buf, f))
	return buf.Bytes()
}

// A plural is edited one branch at a time, through the path a read lists for
// it, and the other branches stay as they were.
func TestApply_APluralBranch(t *testing.T) {
	app := NewApp()
	tab, root := changeProject(t, app, map[string]string{"catalog.kbf.json": string(pluralCatalog(t))}, []string{"catalog.kbf.json"}, "")
	page := readVia(t, app, tab, change.ReadRequest{Doc: "catalog.kbf.json"})
	var cart *change.BlockRead
	for i, b := range page.Blocks {
		if len(b.Structures) > 0 {
			cart = &page.Blocks[i]
		}
	}
	require.NotNil(t, cart, "the catalog's plural reads as a structure: %+v", page.Blocks)
	st := cart.Structures[0]
	require.Equal(t, "plural", st.Kind)
	require.Equal(t, "1 item in your cart", st.Branches["one"])
	require.Contains(t, st.Branches["other"], "<x id=\"1/\"/> items in your cart", "the variable reads as a code in its branch")

	branch := func(form model.PluralForm) model.RunPath {
		return append(append(model.RunPath{}, st.Path...), model.RunPathStep{Kind: model.StepPlural, PluralForm: form})
	}
	one := "1 article in your cart"
	other := strings.Replace(st.Branches["other"], "items", "articles", 1)
	res := applyVia(t, app, tab, change.Set{Ops: []change.Op{
		{Kind: change.KindSetContent, At: cart.Ref, IfMatch: cart.Rev, Body: &change.SetContent{Text: &one, Path: branch(model.PluralOne)}},
		{Kind: change.KindSetContent, At: cart.Ref, IfMatch: cart.Rev, Body: &change.SetContent{Text: &other, Path: branch(model.PluralOther)}},
	}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	after := blockVia(t, app, tab, "catalog.kbf.json", cart.Ref.Block)
	require.Len(t, after.Structures, 1)
	assert.Equal(t, "1 article in your cart", after.Structures[0].Branches["one"])
	assert.Equal(t, other, after.Structures[0].Branches["other"], "the variable is kept")
	assert.Equal(t, "Your cart is empty", after.Structures[0].Branches["zero"], "the branch nobody edited is kept")
	data, err := os.ReadFile(filepath.Join(root, "catalog.kbf.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "articles in your cart")
}

// A source edit leaves every translation in place and lists the translations
// it made stale, the languages the next run re-drafts; approving the source
// is a decide on the revision the pane read.
func TestApply_ASourceEditListsTheTranslationsItMadeStale(t *testing.T) {
	app := NewApp()
	tab, root := newReviewProject(t, app)
	fr := readCatalog(t, filepath.Join(root, "locales", "fr-FR.json"))["greeting"]

	src := blockVia(t, app, tab.ID, "locales/en.json", "greeting")
	res := applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{setText(src.Ref, src.Rev, strings.Replace(src.Text, "Hello", "Hi", 1))}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	var stale []string
	for _, inv := range res.Ops[0].Invalidates {
		stale = append(stale, inv.Edition)
	}
	assert.ElementsMatch(t, []string{"de-DE", "fr-FR"}, stale, "the translations made from the old wording")
	assert.Equal(t, "Hi {name}", readCatalog(t, filepath.Join(root, "locales", "en.json"))["greeting"])
	assert.Equal(t, fr, readCatalog(t, filepath.Join(root, "locales", "fr-FR.json"))["greeting"], "fr keeps its translation for the loop to supersede")

	src = blockVia(t, app, tab.ID, "locales/en.json", "greeting")
	res = applyVia(t, app, tab.ID, change.Set{Ops: []change.Op{decideOp(src.Ref, src.Rev, change.OutcomeEstablish)}})
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
}

// A save the project's checks refuse is gate_failed with its findings; the
// person may save it anyway, sending the change set again with gate report,
// and the edit's record lists the findings overridden. An agent may never
// choose report.
func TestApply_SaveAnywayRecordsTheOverriddenFindings(t *testing.T) {
	// One workspace for the context the project reads in and for the
	// desktop's engine, so the test can read the record the edit leaves.
	wsRoot := t.TempDir()
	app := NewApp()
	app.hostEngine().SetWorkspaceRoot(wsRoot)
	tab, src := overrideProject(t, app, wsRoot, `{"greeting":"Please use the dashboard"}`)
	root := filepath.Dir(filepath.Dir(src))
	b := blockVia(t, app, tab, "locales/en.json", "greeting")
	utilize := change.Set{Note: "Say utilize", Ops: []change.Op{setText(b.Ref, b.Rev, "Please utilize the dashboard")}}

	res := applyVia(t, app, tab, utilize)
	require.Equal(t, change.SetRefused, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeGateFailed, res.Ops[0].Error.Code)
	require.NotEmpty(t, res.Ops[0].Findings, "the refusal carries the findings the person reads")
	assert.Equal(t, "terms.vocabulary", res.Ops[0].Findings[0].Rule)

	ctx := t.Context()
	svc, err := app.changeServiceFor(ctx, tab, nil)
	require.NoError(t, err)
	agent := utilize
	agent.Gate = change.GateReport
	res2, err := svc.Apply(ctx, agent, change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"})
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res2.Status)
	assert.Equal(t, change.CodeNotPermitted, res2.Ops[0].Error.Code, "an agent may not save anyway")

	anyway := utilize
	anyway.Gate = change.GateReport
	res = applyVia(t, app, tab, anyway)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.NotEmpty(t, res.Ops[0].Findings, "the edit lands with its findings")
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Please utilize the dashboard")

	engine := app.hostEngine()
	p, err := engine.Projector(ctx, root)
	require.NoError(t, err)
	ws, err := engine.Workspace(ctx)
	require.NoError(t, err)
	ops, err := ws.Select(ctx, workspace.OpQuery{Project: p.Key(), KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	require.Len(t, ops, 1, "the override is the one edit recorded")
	var e projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
	assert.Equal(t, change.ActorPerson, e.Actor.Kind)
	assert.Equal(t, "desktop", e.Origin.By)
	require.NotEmpty(t, e.Overridden, "the record names what the person overrode")
	assert.Equal(t, "terms.vocabulary", e.Overridden[0].Rule)
	assert.True(t, e.Overridden[0].Fails)
}

// overrideProject is setupCheckProject with the context read into the
// workspace at wsRoot.
func overrideProject(t *testing.T, app *App, wsRoot, sourceJSON string) (tabID, srcPath string) {
	t.Helper()
	dir := t.TempDir()
	srcPath = filepath.Join(dir, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o755))
	require.NoError(t, os.WriteFile(srcPath, []byte(sourceJSON), 0o644))
	proj := &project.KapiProject{
		Version:     project.CurrentVersion,
		Defaults:    project.Defaults{SourceLanguage: "en"},
		Collections: []project.Collection{{Path: "locales/en.json", Target: "locales/{lang}.json"}},
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, project.RelStatePath("voice.yaml")), []byte(houseVoiceYAML), 0o644))
	projPath := filepath.Join(dir, "proj.kapi")
	require.NoError(t, project.Save(projPath, proj))

	reader := &host.App{}
	reader.InitRegistries()
	reader.SetWorkspaceRoot(wsRoot)
	_, err := reader.ImportProjectContext(context.Background(), projPath, host.ContextImportRequest{})
	require.NoError(t, err)
	reader.Shutdown()

	tab, err := app.OpenProject(projPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab.ID, srcPath
}

func TestChangeBindings_Refusals(t *testing.T) {
	app := NewApp()
	tab, _ := changeProject(t, app, map[string]string{"en.json": `{"title": "Welcome"}` + "\n"}, []string{"en.json"}, "")

	t.Run("a change set that does not decode is a refused result with its error", func(t *testing.T) {
		raw, err := app.Apply(tab, `{"ops": [{"op": "rewrite"}]}`)
		require.NoError(t, err)
		var res change.Result
		require.NoError(t, json.Unmarshal([]byte(raw), &res))
		assert.Equal(t, change.SetRefused, res.Status)
		require.NotNil(t, res.Error)
		assert.Equal(t, change.CodeInvalid, res.Error.Code)
		assert.Empty(t, res.Ops)
	})
	t.Run("a read refuses a field the request does not declare", func(t *testing.T) {
		_, err := app.Read(tab, `{"doc": "en.json", "path": "x"}`)
		require.Error(t, err)
	})
	t.Run("a document the project does not hold is not found", func(t *testing.T) {
		_, err := app.Read(tab, `{"doc": "missing.json"}`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not_found")
	})
	t.Run("an unknown tab is an error", func(t *testing.T) {
		_, err := app.Apply("nope", `{"ops": []}`)
		require.Error(t, err, "a change set with no operations decodes, and the tab it names is not open")
		raw, err := app.Apply("nope", `{"note": "no ops list"}`)
		require.NoError(t, err, "a change set that does not decode is refused before any tab is consulted")
		var res change.Result
		require.NoError(t, json.Unmarshal([]byte(raw), &res))
		assert.Equal(t, change.CodeInvalid, res.Error.Code)
		_, err = app.Read("nope", `{"doc": "en.json"}`)
		require.Error(t, err)
	})
	t.Run("describe names what a document's format supports", func(t *testing.T) {
		raw, err := app.Describe(tab, `{"doc": "en.json"}`)
		require.NoError(t, err)
		var d change.Description
		require.NoError(t, json.Unmarshal([]byte(raw), &d))
		assert.Equal(t, "json", d.Format)
		assert.NotNil(t, d.Ops.SetContent)
	})
}
