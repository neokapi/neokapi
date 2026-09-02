package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// readCatalog reads a flat JSON catalog written by the fixture.
func readCatalog(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	out := map[string]string{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

// A source edit made during review names the languages the next run will
// re-draft, and leaves their translations alone. The loop records the source it
// translated for every target it writes, so it reads the rewrite as drift and
// heals it; emptying the files here destroyed the wording a reviewer compares
// the new draft against and the corpus recycles from.
func TestUpdateSourceText_NamesTheLocalesAwaitingARedraft(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, root := newReviewProject(t, app)

	fr := filepath.Join(root, "locales", "fr-FR.json")
	de := filepath.Join(root, "locales", "de-DE.json")
	frBefore := readCatalog(t, fr)["greeting"]
	deBefore := readCatalog(t, de)["greeting"]
	require.NotEmpty(t, frBefore)
	require.NotEmpty(t, deBefore)

	pending, err := app.UpdateSourceText(tab.ID, "locales/en.json", "greeting", "Hi {name}!")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"de-DE", "fr-FR"}, pending)

	assert.Equal(t, "Hi {name}!", readCatalog(t, filepath.Join(root, "locales", "en.json"))["greeting"],
		"the source carries the new wording")
	assert.Equal(t, frBefore, readCatalog(t, fr)["greeting"], "fr keeps its translation for the loop to supersede")
	assert.Equal(t, deBefore, readCatalog(t, de)["greeting"], "de keeps its translation for the loop to supersede")

	// The unit nobody edited is untouched in every language.
	assert.NotEmpty(t, readCatalog(t, fr)["farewell"])
	assert.NotEmpty(t, readCatalog(t, de)["farewell"])
}

func TestUpdateSourceText_RejectsAnEmptyEdit(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, _ := newReviewProject(t, app)

	_, err := app.UpdateSourceText(tab.ID, "locales/en.json", "greeting", "   ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to translate")
}

func TestUpdateSourceText_RejectsAFileTheProjectDoesNotDeclare(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, _ := newReviewProject(t, app)

	_, err := app.UpdateSourceText(tab.ID, "locales/fr-FR.json", "greeting", "Salut")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not content this project declares",
		"a target file is not a source file, however editable it looks")
}

// The queue is empty under the default `checked` gate and lists everything
// unsigned under `approved`; approving one takes it out.
func TestReviewQueue_SourceRowsAndApprove(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, root := newReviewProject(t, app)

	queue, err := app.ReviewQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	assert.Empty(t, sourceRows(queue.Pending),
		"the default gate asks for checks, not a signature")

	// Raise the gate, reopen so the recipe is re-read.
	recipe := filepath.Join(root, "project.kapi")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.SourceGate = string(model.SourceGateApproved)
	require.NoError(t, project.Save(recipe, proj))

	tab2, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab2.ID) })

	queue, err = app.ReviewQueue(tab2.ID, ProjectFilter{})
	require.NoError(t, err)
	rows := sourceRows(queue.Pending)
	require.NotEmpty(t, rows, "an approved gate asks a person to sign every unit")
	for _, it := range rows {
		assert.True(t, it.Held)
		assert.NotEqual(t, string(model.SourceStatusApproved), it.Status)
		assert.Equal(t, "en-US", it.Language, "a source row belongs to the source language")
		assert.Nil(t, it.HasFindings, "a source row has no translation to check")
	}
	before := len(rows)

	// The source language carries its own pending count in the summary, so the
	// language selector can offer source review beside the targets.
	var srcLang *host.ReviewLanguage
	for i, l := range queue.Languages {
		if l.Source {
			srcLang = &queue.Languages[i]
		}
	}
	require.NotNil(t, srcLang, "the summary marks the source language")
	assert.Equal(t, before, srcLang.Pending)

	require.NoError(t, app.ApproveSourceUnit(tab2.ID, rows[0].File, rows[0].Key))

	after, err := app.ReviewQueue(tab2.ID, ProjectFilter{})
	require.NoError(t, err)
	assert.Len(t, sourceRows(after.Pending), before-1, "the approved unit leaves the queue")
}

// sourceRows is the source half of a unified queue.
func sourceRows(items []host.ReviewQueueItem) []host.ReviewQueueItem {
	var out []host.ReviewQueueItem
	for _, it := range items {
		if it.IsSource {
			out = append(out, it)
		}
	}
	return out
}
