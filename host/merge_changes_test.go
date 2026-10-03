package host

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
)

// mergeProject is a project with one JSON catalog whose French translation
// the recipe keeps in a file of its own, and the merge task over it.
func mergeProject(t *testing.T, files map[string]string, policy string) (*App, mergeTask) {
	t.Helper()
	a, recipe := changeProject(t, project.ContentItem{
		Path: "src/en/app.json", Format: &project.FormatSpec{Name: "json"}, Target: "src/{lang}/app.json",
	}, files)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	return a, mergeTask{layout: layout, ctx: project.NewProjectContext(proj, recipe), project: proj, recipe: recipe, policy: policy, input: filepath.Join(layout.Root, "return.xliff")}
}

// blockIDs reads the source and maps each block's text to its ID.
func blockIDs(t *testing.T, a *App, task mergeTask) map[string]string {
	t.Helper()
	svc := changeService(t, a, task.recipe)
	ids := map[string]string{}
	_, err := svc.ReadEach(context.Background(), change.ReadRequest{Doc: "src/en/app.json"}, func(b *model.Block, r change.BlockRead) error {
		ids[r.Text] = b.ID
		return nil
	})
	require.NoError(t, err)
	return ids
}

// extracted reads the revisions each unit of the source carries for French,
// as kapi extract stamps them.
func extracted(t *testing.T, a *App, task mergeTask) map[string]unitRevision {
	t.Helper()
	revs, err := interchangeRevisions(context.Background(), changeService(t, a, task.recipe), "src/en/app.json", "fr")
	require.NoError(t, err)
	return revs
}

// returnedUnit is a unit a translator returned: the block, its French and the
// revisions it was extracted against.
func returnedUnit(id, french string, rev unitRevision) *model.Block {
	b := &model.Block{ID: id}
	b.SetTargetText("fr", french)
	stampUnitRevision(b, rev)
	return b
}

func mergeOutcomeOf(outcomes []mergeOutcome, id string) mergeOutcome {
	for _, o := range outcomes {
		if o.Block == id {
			return o
		}
	}
	return mergeOutcome{}
}

func TestMergeReturned_ASourceEditedSinceExtractIsRefusedStaleNamingBasis(t *testing.T) {
	a, task := mergeProject(t, map[string]string{
		"src/en/app.json": `{"greeting": "Hello", "farewell": "Goodbye"}` + "\n",
	}, project.ConflictPolicyTranslatorWins)
	ids := blockIDs(t, a, task)
	revs := extracted(t, a, task)

	// The source of one message changes after the extraction; nothing but the
	// returned units says what they were made from.
	src := filepath.Join(task.layout.Root, "src", "en", "app.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting": "Hello there", "farewell": "Goodbye"}`+"\n"), 0o644))

	rf := &returnedFile{input: task.input, doc: "src/en/app.json", locale: "fr", blocks: []*model.Block{
		returnedUnit(ids["Hello"], "Bonjour", revs[ids["Hello"]]),
		returnedUnit(ids["Goodbye"], "Au revoir", revs[ids["Goodbye"]]),
	}}
	stats, outcomes, err := a.mergeReturned(context.Background(), task, rf)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Applied)
	assert.Equal(t, 1, stats.Stale)

	stale := mergeOutcomeOf(outcomes, ids["Hello"])
	assert.Equal(t, mergeStale, stale.Status)
	require.NotNil(t, stale.Error, "the change service refuses the unit")
	assert.Equal(t, change.CodeStale, stale.Error.Code)
	assert.Equal(t, "basis", stale.Error.Field, "the refusal names the source the unit was made from")
	assert.Equal(t, mergeApplied, mergeOutcomeOf(outcomes, ids["Goodbye"]).Status)

	assert.Equal(t, `{"greeting": "Hello there", "farewell": "Au revoir"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"),
		"the translation is written from the source's skeleton, the stale message falling back to the source")
}

func TestMergeReturned_TheConflictPolicyDecidesATranslationChangedSinceExtract(t *testing.T) {
	for _, tc := range []struct {
		name    string
		policy  string
		edited  bool
		want    string
		outcome string
	}{
		{name: "translator-wins replaces an edited translation", policy: project.ConflictPolicyTranslatorWins, edited: true, want: "Bonjour à tous", outcome: mergeApplied},
		{name: "existing-wins keeps an edited translation", policy: project.ConflictPolicyExistingWins, edited: true, want: "Salut", outcome: mergeSkipped},
		{name: "existing-wins takes a return over the translation it was extracted against", policy: project.ConflictPolicyExistingWins, edited: false, want: "Bonjour à tous", outcome: mergeApplied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, task := mergeProject(t, map[string]string{
				"src/en/app.json": `{"greeting": "Hello"}` + "\n",
				"src/fr/app.json": `{"greeting": "Bonjour"}` + "\n",
			}, tc.policy)
			ids := blockIDs(t, a, task)
			revs := extracted(t, a, task)
			if tc.edited {
				require.NoError(t, os.WriteFile(filepath.Join(task.layout.Root, "src", "fr", "app.json"), []byte(`{"greeting": "Salut"}`+"\n"), 0o644))
			}

			rf := &returnedFile{input: task.input, doc: "src/en/app.json", locale: "fr", blocks: []*model.Block{
				returnedUnit(ids["Hello"], "Bonjour à tous", revs[ids["Hello"]]),
			}}
			_, outcomes, err := a.mergeReturned(context.Background(), task, rf)
			require.NoError(t, err)
			got := mergeOutcomeOf(outcomes, ids["Hello"])
			assert.Equal(t, tc.outcome, got.Status)
			if tc.outcome == mergeSkipped {
				require.NotNil(t, got.Error)
				assert.Equal(t, change.CodeStale, got.Error.Code)
				assert.Equal(t, "if_match", got.Error.Field)
			}
			assert.Equal(t, `{"greeting": "`+tc.want+`"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"))
		})
	}
}

func TestMergeReturned_AUnitWithNoRevisionsIsHeldToTheSourceItCarries(t *testing.T) {
	a, task := mergeProject(t, map[string]string{
		"src/en/app.json": `{"greeting": "Hello", "farewell": "Goodbye"}` + "\n",
	}, project.ConflictPolicyTranslatorWins)
	ids := blockIDs(t, a, task)
	unit := func(id, source, french string) *model.Block {
		b := &model.Block{ID: id, Source: []model.Run{{Text: &model.TextRun{Text: source}}}}
		b.SetTargetText("fr", french)
		return b
	}
	rf := &returnedFile{input: task.input, doc: "src/en/app.json", locale: "fr", blocks: []*model.Block{
		unit(ids["Hello"], "Hello", "Bonjour"),
		unit(ids["Goodbye"], "Good bye", "Au revoir"),
		unit("gone", "Gone", "Parti"),
	}}
	stats, outcomes, err := a.mergeReturned(context.Background(), task, rf)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Applied)
	assert.Equal(t, 2, stats.Stale)
	assert.Equal(t, mergeApplied, mergeOutcomeOf(outcomes, ids["Hello"]).Status)
	assert.Equal(t, "basis", mergeOutcomeOf(outcomes, ids["Goodbye"]).Error.Field, "a unit made from other source text is stale")
	assert.Equal(t, change.CodeNotFound, mergeOutcomeOf(outcomes, "gone").Error.Code, "a unit whose block is gone is stale")
	assert.Equal(t, `{"greeting": "Bonjour", "farewell": "Goodbye"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"))
}

// TestMergeReturned_AUnitIsHeldToTheSourceItCarriesWhateverItsBasis pins that
// a unit whose source is not the source of the block its ID names is stale
// even when the basis it carries is that block's: the translation is of
// other text.
func TestMergeReturned_AUnitIsHeldToTheSourceItCarriesWhateverItsBasis(t *testing.T) {
	a, task := mergeProject(t, map[string]string{
		"src/en/app.json": `{"greeting": "Hello", "farewell": "Goodbye"}` + "\n",
	}, project.ConflictPolicyTranslatorWins)
	ids := blockIDs(t, a, task)
	revs := extracted(t, a, task)
	unit := func(id, source, french string) *model.Block {
		b := returnedUnit(id, french, revs[id])
		b.Source = []model.Run{{Text: &model.TextRun{Text: source}}}
		return b
	}
	rf := &returnedFile{input: task.input, doc: "src/en/app.json", locale: "fr", blocks: []*model.Block{
		unit(ids["Hello"], "Hello", "Bonjour"),
		unit(ids["Goodbye"], "Hello", "Bonjour"),
	}}
	stats, outcomes, err := a.mergeReturned(context.Background(), task, rf)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Applied)
	stale := mergeOutcomeOf(outcomes, ids["Goodbye"])
	assert.Equal(t, mergeStale, stale.Status)
	require.NotNil(t, stale.Error)
	assert.Equal(t, "basis", stale.Error.Field)
	assert.Equal(t, `{"greeting": "Bonjour", "farewell": "Goodbye"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"))
}

// TestMergeReturned_ATranslationThatBreaksARuleIsRefused pins that merge
// applies a return under the enforce gate: a unit whose translation
// introduces a failing finding (the recipe holds French to "gadget" for
// "widget") is refused with gate_failed and named on stderr, and the units
// that pass land.
func TestMergeReturned_ATranslationThatBreaksARuleIsRefused(t *testing.T) {
	f := newCommitFixture(t)
	proj, err := project.Load(f.recipe)
	require.NoError(t, err)
	layout, err := project.LayoutFor(f.recipe)
	require.NoError(t, err)
	task := mergeTask{layout: layout, ctx: project.NewProjectContext(proj, f.recipe), project: proj, recipe: f.recipe,
		policy: project.ConflictPolicyTranslatorWins, input: filepath.Join(layout.Root, "return.xliff")}
	ctx := context.Background()
	svc, err := f.app.ChangeService(ctx, ChangeServiceOptions{Project: f.recipe, TargetLocale: "fr", Materialize: true})
	require.NoError(t, err)
	revs, err := interchangeRevisions(ctx, svc, "docs/guide.md", "fr")
	require.NoError(t, err)
	ids := map[string]string{}
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: "docs/guide.md"}, func(b *model.Block, r change.BlockRead) error {
		ids[r.Text] = b.ID
		return nil
	})
	require.NoError(t, err)
	heading, paragraph := ids["Guide"], ids["We use the widget every day."]
	require.NotEmpty(t, heading)
	require.NotEmpty(t, paragraph)

	rf := &returnedFile{input: task.input, doc: "docs/guide.md", locale: "fr", blocks: []*model.Block{
		returnedUnit(heading, "Guide", revs[heading]),
		returnedUnit(paragraph, "Nous utilisons le widget chaque jour.", revs[paragraph]),
	}}
	var outcomes []mergeOutcome
	var stats mergeStats
	stderr, err := captureStderr(t, func() error {
		var merr error
		stats, outcomes, merr = f.app.mergeReturned(ctx, task, rf)
		return merr
	})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Applied)
	assert.Equal(t, 1, stats.Refused)
	refused := mergeOutcomeOf(outcomes, paragraph)
	assert.Equal(t, mergeRefused, refused.Status)
	require.NotNil(t, refused.Error)
	assert.Equal(t, change.CodeGateFailed, refused.Error.Code)
	assert.Contains(t, stderr, "not merged")
	assert.NotContains(t, readFile(t, f.recipe, "docs/fr/guide.md"), "Nous utilisons le widget",
		"the translation that breaks the rule is not written")
}

// TestMergeStats_SummaryCountsRefusedUnitsApart pins the counts a .kpz merge
// prints: a refused unit is counted as refused, never as skipped.
func TestMergeStats_SummaryCountsRefusedUnitsApart(t *testing.T) {
	for _, tc := range []struct {
		stats mergeStats
		want  string
	}{
		{mergeStats{Applied: 2, Skipped: 1}, "applied=2 stale=0 skipped=1 memory_new=0 memory_updated=0 (conflict_policy=translator-wins)"},
		{mergeStats{Applied: 2, Skipped: 1, Refused: 3}, "applied=2 stale=0 skipped=1 refused=3 memory_new=0 memory_updated=0 (conflict_policy=translator-wins)"},
	} {
		assert.Equal(t, tc.want, tc.stats.summary(project.ConflictPolicyTranslatorWins))
	}
}

func TestMergeReturned_ASourceOnlyDocumentIsRefused(t *testing.T) {
	a, recipe := changeProject(t, project.ContentItem{Path: "src/en/app.json", Format: &project.FormatSpec{Name: "json"}},
		map[string]string{"src/en/app.json": `{"greeting": "Hello"}` + "\n"})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	task := mergeTask{layout: layout, ctx: project.NewProjectContext(proj, recipe), project: proj, recipe: recipe, input: "return.xliff"}
	ids := blockIDs(t, a, task)
	rf := &returnedFile{input: task.input, doc: "src/en/app.json", locale: "fr", blocks: []*model.Block{
		returnedUnit(ids["Hello"], "Bonjour", extracted(t, a, task)[ids["Hello"]]),
	}}
	stats, _, err := a.mergeReturned(context.Background(), task, rf)
	require.Error(t, err, "a merge that lands nothing because the source has no translation file fails")
	assert.Contains(t, err.Error(), "names no target")
	assert.Equal(t, 1, stats.Refused)
}

// TestChangeService_GivesTheWriterHookTheWriterOfATranslation pins the hook
// kapi pull sets a document's locale-variant media through: the writer that
// writes a translation from its source's skeleton is handed to it.
func TestChangeService_GivesTheWriterHookTheWriterOfATranslation(t *testing.T) {
	a, task := mergeProject(t, map[string]string{"src/en/app.json": `{"greeting": "Hello"}` + "\n"}, "")
	ids := blockIDs(t, a, task)
	var hooked []format.DataFormatWriter
	svc, err := a.ChangeService(context.Background(), ChangeServiceOptions{Project: task.recipe, Materialize: true,
		WriterHook: func(w format.DataFormatWriter) { hooked = append(hooked, w) }})
	require.NoError(t, err)
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{
		setTo(change.Ref{Doc: "src/en/app.json", Block: ids["Hello"], Edition: model.EditionKey{Locale: "fr"}}, model.AbsentRevision, "Bonjour"),
	}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.NotEmpty(t, hooked, "the writer of the translation went through the hook")
	assert.Equal(t, `{"greeting": "Bonjour"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"))
}

// TestChangeService_MaterializeWritesABilingualSourcesTranslationToItsTarget
// pins that a service writing whole translations keeps a PO source's
// translation in the file the target template names: the French lands in
// po/fr.po and the English catalog stays as it was.
func TestChangeService_MaterializeWritesABilingualSourcesTranslationToItsTarget(t *testing.T) {
	const en = "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: en\\n\"\n\nmsgid \"Hello\"\nmsgstr \"\"\n"
	a, recipe := changeProject(t, project.ContentItem{
		Path: "po/en.po", Format: &project.FormatSpec{Name: "po"}, Target: "po/{lang}.po",
	}, map[string]string{"po/en.po": en})
	ctx := context.Background()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: recipe, TargetLocale: "fr", Materialize: true})
	require.NoError(t, err)
	page, err := svc.Read(ctx, change.ReadRequest{Doc: "po/en.po"})
	require.NoError(t, err)
	at := blockWith(t, page, "Hello").Ref
	at.Edition = model.EditionKey{Locale: "fr"}
	res, err := svc.Apply(ctx, change.Set{Ops: []change.Op{setTo(at, model.AbsentRevision, "Bonjour")}}, changePerson)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	assert.Contains(t, readFile(t, recipe, "po/fr.po"), `msgstr "Bonjour"`)
	assert.Equal(t, en, readFile(t, recipe, "po/en.po"), "the source catalog is untouched")
}

// TestMaterialize_WritesEachTranslationThroughTheChangeService pins kapi
// merge's materializing form: the targets the block store holds become one
// change set per translation, which the change service writes from the
// source's skeleton and records as an edit. A message the existing
// translation holds and the store does not keeps its text, a message the
// source gained lands, and a source-only collection gets no file.
func TestMaterialize_WritesEachTranslationThroughTheChangeService(t *testing.T) {
	ctx := context.Background()
	a, task := mergeProject(t, map[string]string{
		"src/en/app.json": `{"greeting": "Hello", "farewell": "Goodbye", "new": "Fresh"}` + "\n",
		"src/fr/app.json": `{"farewell":"Au revoir","greeting":"Bonjour"}` + "\n",
		"notes/en.json":   `{"note": "Source only"}` + "\n",
	}, project.ConflictPolicyTranslatorWins)
	proj := task.project
	proj.Collections = append(proj.Collections, project.Collection{Name: "notes", SourceOnly: true,
		Content: []project.ContentItem{{Path: "notes/en.json", Format: &project.FormatSpec{Name: "json"}}}})
	require.NoError(t, project.Save(task.recipe, proj))
	ids := blockIDs(t, a, task)

	db, err := a.ProjectDB(ctx, task.layout.Root)
	require.NoError(t, err)
	sess, err := a.projectBlocksAutocommit(db).Begin(ctx)
	require.NoError(t, err)
	fileCtx := blockstore.WithSourceRel(ctx, "src/en/app.json")
	for text, french := range map[string]string{"Hello": "Salut", "Fresh": "Frais"} {
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      blockstore.TargetOverlayKind("fr"),
			BlockHash: blockstore.OverlayKey(fileCtx, ids[text], text),
			Payload:   []byte(`{"text":"` + french + `","status":"draft"}`),
		}))
	}
	require.NoError(t, sess.Close())

	written, err := a.materializeFromProjectStore(ctx, io.Discard, proj, task.recipe, []model.LocaleID{"fr"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, `{"greeting": "Salut", "farewell": "Au revoir", "new": "Frais"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"))
	assert.NoDirExists(t, filepath.Join(task.layout.Root, "notes", "fr"), "a source-only collection has no translation file")

	ops := editOps(t, a, task.layout.Root)
	require.Len(t, ops, 1, "the translation is written as one recorded edit")
	var e projector.Edit
	require.NoError(t, json.Unmarshal(ops[0].Payload, &e))
	assert.Equal(t, "merge", e.Origin.By)
	assert.Equal(t, change.ActorTool, e.Actor.Kind)
}

// poCatalog is a PO catalog in lang holding each msgid and msgstr pair.
func poCatalog(lang string, entries ...[2]string) string {
	var b strings.Builder
	b.WriteString("msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"Language: " + lang + "\\n\"\n")
	for _, e := range entries {
		b.WriteString("\nmsgid \"" + e[0] + "\"\nmsgstr \"" + e[1] + "\"\n")
	}
	return b.String()
}

// poMergeProject is a project with one PO catalog whose French translation
// the recipe keeps in a catalog of its own, and the merge task over it.
func poMergeProject(t *testing.T, files map[string]string) (*App, mergeTask) {
	t.Helper()
	a, recipe := changeProject(t, project.ContentItem{
		Path: "po/en.po", Format: &project.FormatSpec{Name: "po"}, Target: "po/{lang}.po",
	}, files)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	return a, mergeTask{layout: layout, ctx: project.NewProjectContext(proj, recipe), project: proj, recipe: recipe,
		policy: project.ConflictPolicyTranslatorWins, input: filepath.Join(layout.Root, "return.po")}
}

// TestMergeReturned_APOTranslationKeepsWhatTheReturnLeaves pins a merge into
// the French catalog of a PO source: the extraction stamps each entry with
// the French msgstr's revision (absent for an untranslated entry), and a
// return that translates one entry leaves every other translation and the
// catalog's header as they were.
func TestMergeReturned_APOTranslationKeepsWhatTheReturnLeaves(t *testing.T) {
	en := poCatalog("en", [2]string{"Hello", ""}, [2]string{"Goodbye", ""}, [2]string{"Thanks", ""})
	fr := poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"Thanks", ""})
	a, task := poMergeProject(t, map[string]string{"po/en.po": en, "po/fr.po": fr})
	ctx := context.Background()
	svc, err := a.ChangeService(ctx, ChangeServiceOptions{Project: task.recipe, TargetLocale: "fr", Materialize: true})
	require.NoError(t, err)
	revs, err := interchangeRevisions(ctx, svc, "po/en.po", "fr")
	require.NoError(t, err)
	ids := map[string]string{}
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: "po/en.po"}, func(b *model.Block, r change.BlockRead) error {
		ids[r.Text] = b.ID
		return nil
	})
	require.NoError(t, err)
	french := model.EditionKey{Locale: "fr"}
	assert.Equal(t, model.RunsRevision(french, []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}), revs[ids["Hello"]].IfMatch,
		"an entry is extracted against its French msgstr")
	assert.Equal(t, model.AbsentRevision, revs[ids["Thanks"]].IfMatch, "an untranslated entry holds no French")

	unit := returnedUnit(ids["Thanks"], "Merci", revs[ids["Thanks"]])
	unit.Source = []model.Run{{Text: &model.TextRun{Text: "Thanks"}}}
	rf := &returnedFile{input: task.input, doc: "po/en.po", locale: "fr", plainSource: true, blocks: []*model.Block{unit}}
	stats, _, err := a.mergeReturned(ctx, task, rf)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Applied)
	assert.Equal(t, poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"Thanks", "Merci"}),
		readFile(t, task.recipe, "po/fr.po"))
	assert.Equal(t, en, readFile(t, task.recipe, "po/en.po"), "the source catalog is untouched")
}

// TestMaterialize_KeepsTheTranslationsAPOCatalogHolds pins materialize on a
// PO source whose block store holds a target for one entry only, as on a
// fresh clone after one new string: that entry lands and every translation
// the French catalog holds stays.
func TestMaterialize_KeepsTheTranslationsAPOCatalogHolds(t *testing.T) {
	en := poCatalog("en", [2]string{"Hello", ""}, [2]string{"Goodbye", ""}, [2]string{"New", ""})
	fr := poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"New", ""})
	a, task := poMergeProject(t, map[string]string{"po/en.po": en, "po/fr.po": fr})
	ctx := context.Background()
	ids := map[string]string{}
	_, err := changeService(t, a, task.recipe).ReadEach(ctx, change.ReadRequest{Doc: "po/en.po"}, func(b *model.Block, r change.BlockRead) error {
		ids[r.Text] = b.ID
		return nil
	})
	require.NoError(t, err)
	db, err := a.ProjectDB(ctx, task.layout.Root)
	require.NoError(t, err)
	sess, err := a.projectBlocksAutocommit(db).Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{
		Kind:      blockstore.TargetOverlayKind("fr"),
		BlockHash: blockstore.OverlayKey(blockstore.WithSourceRel(ctx, "po/en.po"), ids["New"], "New"),
		Payload:   []byte(`{"text":"Nouveau","status":"draft"}`),
	}))
	require.NoError(t, sess.Close())

	written, err := a.materializeFromProjectStore(ctx, io.Discard, task.project, task.recipe, []model.LocaleID{"fr"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, poCatalog("fr", [2]string{"Hello", "Bonjour"}, [2]string{"Goodbye", "Au revoir"}, [2]string{"New", "Nouveau"}),
		readFile(t, task.recipe, "po/fr.po"))
}

// TestMaterialize_BringsATranslationBackToItsSource pins that materialize
// writes a translation whose file no longer holds its source's blocks even
// when every stored target is already in it: the key the source gained lands
// with the source's text. A translation that already says everything is left
// alone and not counted as written.
func TestMaterialize_BringsATranslationBackToItsSource(t *testing.T) {
	ctx := context.Background()
	a, task := mergeProject(t, map[string]string{
		"src/en/app.json": `{"greeting": "Hello", "added": "Brand new"}` + "\n",
		"src/fr/app.json": `{"greeting": "Salut"}` + "\n",
	}, project.ConflictPolicyTranslatorWins)
	ids := blockIDs(t, a, task)
	db, err := a.ProjectDB(ctx, task.layout.Root)
	require.NoError(t, err)
	sess, err := a.projectBlocksAutocommit(db).Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{
		Kind:      blockstore.TargetOverlayKind("fr"),
		BlockHash: blockstore.OverlayKey(blockstore.WithSourceRel(ctx, "src/en/app.json"), ids["Hello"], "Hello"),
		Payload:   []byte(`{"text":"Salut","status":"draft"}`),
	}))
	require.NoError(t, sess.Close())

	var out strings.Builder
	written, err := a.materializeFromProjectStore(ctx, &out, task.project, task.recipe, []model.LocaleID{"fr"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, `{"greeting": "Salut", "added": "Brand new"}`+"\n", readFile(t, task.recipe, "src/fr/app.json"))
	assert.Contains(t, out.String(), "Merged src/en/app.json")

	out.Reset()
	written, err = a.materializeFromProjectStore(ctx, &out, task.project, task.recipe, []model.LocaleID{"fr"}, true)
	require.NoError(t, err)
	assert.Zero(t, written, "a translation that already says everything is not written again")
	assert.Empty(t, out.String())
}
