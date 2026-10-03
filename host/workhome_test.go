package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changetest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
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

// TestChangeService_TheRecipePicksTheHomeOfAnEditionWithNoFile: under
// `materialize: on-converge` a translation whose file does not exist lives in
// the workspace home until a delivery writes the file, whether or not the
// workspace held anything of it, so a change set never delivers a file the
// ship gate withholds. Under `manual` the translation is written to its file.
func TestChangeService_TheRecipePicksTheHomeOfAnEditionWithNoFile(t *testing.T) {
	cases := []struct {
		materialize string
		kept        bool
	}{
		{materialize: project.MaterializeManual},
		{materialize: project.MaterializeOnConverge, kept: true},
	}
	for _, tc := range cases {
		t.Run(tc.materialize, func(t *testing.T) {
			a, recipe := changeProject(t, project.ContentItem{Path: "docs/*.json", Target: "out/{lang}/{path}.json"},
				map[string]string{"docs/a.json": `{"greeting": "Hello there"}` + "\n"},
				func(p *project.KapiProject) { p.Defaults.Materialize = tc.materialize })
			t.Cleanup(a.Shutdown)
			res := draftGerman(t, changeService(t, a, recipe), "docs/a.json", map[string]string{"greeting": "Hallo"})
			target := targetRef(t, a, recipe, "docs/a.json", "de")
			_, err := os.Stat(filepath.Join(filepath.Dir(recipe), filepath.FromSlash(target)))
			if !tc.kept {
				require.NoError(t, err, "the translation is written to its file")
				assert.Contains(t, readFile(t, recipe, target), "Hallo")
				assert.Empty(t, keptGerman(t, a, recipe, "docs/a.json"))
				return
			}
			assert.True(t, os.IsNotExist(err), "the withheld translation reaches no file")
			assert.Equal(t, map[string]string{"greeting": "Hallo"}, keptGerman(t, a, recipe, "docs/a.json"))
			homes := map[string]string{}
			for _, d := range res.Docs {
				homes[d.Edition] = d.Home
			}
			assert.Equal(t, workhome.Name, homes["de"])
		})
	}
}

// redactingKeptProject is withheldProject with a redaction policy whose rule
// withholds "Falcon", detected as detectors names.
func redactingKeptProject(t *testing.T, detectors ...string) (*App, string) {
	t.Helper()
	a, recipe := withheldProject(t, map[string]string{"docs/a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"})
	root := filepath.Dir(recipe)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Redaction = &project.RedactionSpec{Enabled: true, Rules: "redaction.yaml", Detectors: detectors}
	require.NoError(t, project.Save(recipe, proj))
	rules := &redaction.RulesFile{Rules: []redaction.Rule{{Term: "Falcon", Category: "product"}}}
	require.NoError(t, rules.Save(filepath.Join(root, "redaction.yaml")))
	return a, recipe
}

// recordedLeaks counts the operations of the project's log, and the blobs
// they name, that hold text.
func recordedLeaks(t *testing.T, a *App, root, text string) int {
	t.Helper()
	ws := boundWorkspace(t, a, root)
	leaks := 0
	for _, op := range editOps(t, a, root) {
		if strings.Contains(string(op.Payload), text) {
			leaks++
		}
		for _, ref := range workspace.BlobRefs(op) {
			blob, err := ws.Blob(context.Background(), ref)
			require.NoError(t, err)
			if strings.Contains(string(blob), text) {
				leaks++
			}
		}
	}
	return leaks
}

// TestChangeService_AKeptEditionRecordsNoWithheldValue: in a project that
// declares redaction, what the workspace home records of a kept edition (a
// producer's draft, an agent's edit and its note) holds no withheld value,
// and the edition reads back with the value on the machine that withheld it.
func TestChangeService_AKeptEditionRecordsNoWithheldValue(t *testing.T) {
	ctx := context.Background()
	a, recipe := redactingKeptProject(t, "rules")
	root := filepath.Dir(recipe)
	keepGerman(t, a, recipe, "docs/a.json", map[string]string{"greeting": "Hallo", "farewell": "Falcon sagt Tschüss"})
	svc := changeService(t, a, recipe)
	target := targetRef(t, a, recipe, "docs/a.json", "de")
	page, err := svc.Read(ctx, change.ReadRequest{Doc: target, Blocks: []string{"greeting"}})
	require.NoError(t, err)
	res, err := svc.Apply(ctx, change.Set{Note: "Falcon note", Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Falcon kommt")}},
		change.Actor{Kind: change.ActorAgent, Name: "claude"})
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	assert.Zero(t, recordedLeaks(t, a, root, "Falcon"), "no record names the withheld value")
	assert.Equal(t, map[string]string{"greeting": "Falcon kommt", "farewell": "Falcon sagt Tschüss"}, keptGerman(t, a, recipe, "docs/a.json"),
		"the kept edition reads back whole where it was withheld")
	page, err = svc.Read(ctx, change.ReadRequest{Doc: target, Blocks: []string{"greeting"}})
	require.NoError(t, err)
	assert.Equal(t, "Falcon kommt", page.Blocks[0].Text)

	// The edition reads at the revision it was written at, so the next
	// change to it lands.
	res, err = svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(page.Blocks[0].Ref, page.Blocks[0].Rev, "Falcon geht")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Zero(t, recordedLeaks(t, a, root, "Falcon"))
}

// TestChangeService_AnEntityPolicyKeepsNoEdition: a redaction policy that
// detects entities cannot redact a record of a kept edition, so a change to a
// translation with no file is refused rather than recorded with the values
// the policy withholds.
func TestChangeService_AnEntityPolicyKeepsNoEdition(t *testing.T) {
	a, recipe := redactingKeptProject(t, "entities")
	svc := changeService(t, a, recipe)
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setTo(change.Ref{Doc: "docs/a.json", Block: "greeting", Edition: model.EditionKey{Locale: "de"}}, model.AbsentRevision, "Falcon kommt")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Error)
	assert.Equal(t, change.CodeUnsupported, res.Ops[0].Error.Code)
	assert.Empty(t, keptGerman(t, a, recipe, "docs/a.json"))
	assert.Zero(t, recordedLeaks(t, a, filepath.Dir(recipe), "Falcon"))
}

// TestDraftStamp_KeepsTheOverlayAStepAfterTheProducerChanged: a step after
// the producer (unredact, a post-processing tool) can change a draft, so the
// overlay the producer serves the draft from differs from the draft the
// workspace keeps. The stamp keeps the overlay's own runs then, and only then.
func TestDraftStamp_KeepsTheOverlayAStepAfterTheProducerChanged(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()
	doc := &flowDoc{ref: "src/en.json", edition: model.EditionKey{Locale: "nl"}}
	lb := &leftBlock{id: "title", source: "Tide window"}
	key := blockstore.StoreKey(filepath.FromSlash(doc.ref), lb.id, lb.source)
	served := []model.Run{model.TextR("Getij [PH1]")}
	payload, err := json.Marshal(blockstore.TargetOverlay{Runs: served, Text: model.RunsText(served), Provider: "demo", Config: "fp",
		Source: blockstore.SourceStamp(lb.source)})
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: blockstore.TargetOverlayKind("nl"), BlockHash: key, Payload: payload}))

	cases := []struct {
		name  string
		draft []model.Run
		runs  string
	}{
		{name: "the draft is what the producer served", draft: served},
		{name: "a later step changed the draft", draft: []model.Run{model.TextR("Getij Falcon")}, runs: "Getij [PH1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := draftStampOf(sess, doc, lb, leftEdition{runs: tc.draft})
			require.NotNil(t, data, "the overlay of the block's source is stamped")
			var st draftStamp
			require.NoError(t, json.Unmarshal(data, &st))
			assert.Equal(t, key, st.Key)
			assert.Equal(t, "fp", st.Config)
			assert.Equal(t, tc.runs, model.RunsText(st.Runs))
		})
	}
}

// TestRestoreKeptOverlays_WritesTheOverlayAsTheProducerLeftIt: a deleted
// block store gets back the overlay of each kept draft, with the overlay's
// own runs where the stamp keeps them and the draft's otherwise.
func TestRestoreKeptOverlays_WritesTheOverlayAsTheProducerLeftIt(t *testing.T) {
	ctx := context.Background()
	a, recipe := withheldProject(t, map[string]string{"docs/a.json": `{"greeting": "Hello there", "farewell": "Goodbye"}` + "\n"})
	root := filepath.Dir(recipe)
	h, err := a.keptEditions(root).open(ctx, true)
	require.NoError(t, err)
	stampOf := func(block, source string, runs []model.Run) (string, json.RawMessage) {
		key := blockstore.StoreKey(filepath.FromSlash("docs/a.json"), block, source)
		data, err := json.Marshal(draftStamp{Key: key, Provider: "demo", Config: "fp", Source: blockstore.SourceStamp(source), Runs: runs})
		require.NoError(t, err)
		return key, data
	}
	greetingKey, greetingStamp := stampOf("greeting", "Hello there", []model.Run{model.TextR("Hallo [PH1]")})
	farewellKey, farewellStamp := stampOf("farewell", "Goodbye", nil)
	draft := model.Status(model.TargetStatusDraft)
	_, err = h.Produce(ctx, workhome.Produce{Doc: "docs/a.json", Edition: model.EditionKey{Locale: "de"},
		Actor: change.Actor{Kind: change.ActorTool, Name: "translate"}, Origin: "flow:translate",
		Blocks: []workhome.Produced{
			{Block: "greeting", Before: model.AbsentRevision, Stamp: greetingStamp,
				Edition: model.Edition{Runs: []model.Run{model.TextR("Hallo Falcon")}, Status: draft}},
			{Block: "farewell", Before: model.AbsentRevision, Stamp: farewellStamp,
				Edition: model.Edition{Runs: []model.Run{model.TextR("Tschüss")}, Status: draft}},
		}})
	require.NoError(t, err)

	n, err := a.restoreKeptOverlays(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	sess, err := a.projectBlocksAutocommit(db).Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()
	for key, want := range map[string]string{greetingKey: "Hallo [PH1]", farewellKey: "Tschüss"} {
		o, err := sess.GetOverlay(blockstore.TargetOverlayKind("de"), key)
		require.NoError(t, err)
		var stored blockstore.TargetOverlay
		require.NoError(t, json.Unmarshal(o.Payload, &stored))
		assert.Equal(t, want, stored.TargetText())
		assert.Equal(t, "fp", stored.Config)
	}
}
