package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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
