package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestTermbase(t *testing.T, app *App) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	handle, err := app.OpenTermbase(path)
	require.NoError(t, err)
	require.NotEmpty(t, handle)
	t.Cleanup(func() { app.CloseTermbase(handle) })
	return handle
}

func TestTermbase_OpenAndClose(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	stats := app.GetTermbaseStats(handle)
	require.NotNil(t, stats)
	assert.Equal(t, 0, stats.Count)

	app.CloseTermbase(handle)
	assert.Nil(t, app.GetTermbaseStats(handle))
}

func TestTermbase_CRUD(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	// Add
	err := app.AddConcept(handle, AddConceptRequest{
		Domain:     "Legal",
		Definition: "A legally binding agreement",
		Terms: []TermDTO{
			{Text: "contract", Locale: "en-US", Status: "preferred"},
			{Text: "contrat", Locale: "fr-FR", Status: "approved"},
		},
	})
	require.NoError(t, err)

	stats := app.GetTermbaseStats(handle)
	assert.Equal(t, 1, stats.Count)

	// Search
	result := app.SearchTerms(handle, "", "", "", 0, 50)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Concepts, 1)
	assert.Equal(t, "Legal", result.Concepts[0].Domain)
	assert.Len(t, result.Concepts[0].Terms, 2)
	assert.Equal(t, "contract", result.Concepts[0].Terms[0].Text)

	conceptID := result.Concepts[0].ID

	// Get
	concept := app.GetConcept(handle, conceptID)
	require.NotNil(t, concept)
	assert.Equal(t, "Legal", concept.Domain)

	// Update
	err = app.UpdateConcept(handle, UpdateConceptRequest{
		ConceptID:  conceptID,
		Domain:     "Legal",
		Definition: "Updated definition",
		Terms: []TermDTO{
			{Text: "contract", Locale: "en-US", Status: "preferred"},
			{Text: "contrat", Locale: "fr-FR", Status: "approved"},
			{Text: "Vertrag", Locale: "de-DE", Status: "approved"},
		},
	})
	require.NoError(t, err)

	concept = app.GetConcept(handle, conceptID)
	assert.Equal(t, "Updated definition", concept.Definition)
	assert.Len(t, concept.Terms, 3)

	// Delete
	err = app.DeleteConcept(handle, conceptID)
	require.NoError(t, err)
	assert.Equal(t, 0, app.GetTermbaseStats(handle).Count)
}

func TestTermbase_BatchDelete(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	for i := range 5 {
		require.NoError(t, app.AddConcept(handle, AddConceptRequest{
			Domain: "Domain " + string(rune('A'+i)),
			Terms: []TermDTO{
				{Text: "term" + string(rune('A'+i)), Locale: "en", Status: "approved"},
			},
		}))
	}
	assert.Equal(t, 5, app.GetTermbaseStats(handle).Count)

	result := app.SearchTerms(handle, "", "", "", 0, 50)
	ids := make([]string, 0, 3)
	for i := range 3 {
		ids = append(ids, result.Concepts[i].ID)
	}

	require.NoError(t, app.DeleteConcepts(handle, ids))
	assert.Equal(t, 2, app.GetTermbaseStats(handle).Count)
}

func TestTermbase_Search(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	require.NoError(t, app.AddConcept(handle, AddConceptRequest{
		Domain:     "Software",
		Definition: "A reusable software component",
		Terms: []TermDTO{
			{Text: "widget", Locale: "en", Status: "preferred"},
		},
	}))
	require.NoError(t, app.AddConcept(handle, AddConceptRequest{
		Domain:     "Legal",
		Definition: "A binding agreement",
		Terms: []TermDTO{
			{Text: "contract", Locale: "en", Status: "preferred"},
		},
	}))

	result := app.SearchTerms(handle, "widget", "", "", 0, 50)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "widget", result.Concepts[0].Terms[0].Text)
}

func TestTermbase_ConceptDTO_Conversion(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	require.NoError(t, app.AddConcept(handle, AddConceptRequest{
		ProjectID:  "proj-1",
		Domain:     "Medical",
		Definition: "Inflammation of the appendix",
		Terms: []TermDTO{
			{Text: "appendicitis", Locale: "en-US", Status: "preferred", PartOfSpeech: "noun"},
			{Text: "appendicite", Locale: "fr-FR", Status: "approved", Gender: "feminine"},
		},
	}))

	result := app.SearchTerms(handle, "", "", "", 0, 50)
	require.Len(t, result.Concepts, 1)

	c := result.Concepts[0]
	assert.Equal(t, "proj-1", c.ProjectID)
	assert.Equal(t, "Medical", c.Domain)
	assert.NotEmpty(t, c.CreatedAt)
	assert.NotEmpty(t, c.UpdatedAt)

	assert.Equal(t, "en-US", c.Terms[0].Locale)
	assert.Equal(t, "preferred", c.Terms[0].Status)
	assert.Equal(t, "noun", c.Terms[0].PartOfSpeech)
	assert.Equal(t, "fr-FR", c.Terms[1].Locale)
	assert.Equal(t, "feminine", c.Terms[1].Gender)
}

func TestTermbase_ListNamed(t *testing.T) {
	list := listNamedResources("nonexistent-termbases-dir-12345")
	assert.Nil(t, list)
}
