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
func TestGetSourceQueue_AndApprove(t *testing.T) {
	app := newAIReviewApp(t, aiprovider.NewMockProvider())
	tab, root := newReviewProject(t, app)

	queue, err := app.GetSourceQueue(tab.ID, ProjectFilter{})
	require.NoError(t, err)
	assert.Empty(t, queue, "the default gate asks for checks, not a signature")

	// Raise the gate, reopen so the recipe is re-read.
	recipe := filepath.Join(root, "project.kapi")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.SourceGate = string(model.SourceGateApproved)
	require.NoError(t, project.Save(recipe, proj))

	tab2, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab2.ID) })

	queue, err = app.GetSourceQueue(tab2.ID, ProjectFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, queue, "an approved gate asks a person to sign every unit")
	for _, it := range queue {
		assert.True(t, it.Held)
		assert.False(t, it.Approved)
	}
	before := len(queue)

	require.NoError(t, app.ApproveSourceUnit(tab2.ID, queue[0].File, queue[0].Key))

	after, err := app.GetSourceQueue(tab2.ID, ProjectFilter{})
	require.NoError(t, err)
	assert.Len(t, after, before-1, "the approved unit leaves the queue")
}
