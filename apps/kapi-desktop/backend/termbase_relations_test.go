package backend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addConceptAndID adds a one-term concept and returns its generated ID.
func addConceptAndID(t *testing.T, app *App, handle, domain, text, locale string) string {
	t.Helper()
	before := app.SearchTerms(handle, "", "", "", 0, 200)
	known := make(map[string]bool, len(before.Concepts))
	for _, c := range before.Concepts {
		known[c.ID] = true
	}
	require.NoError(t, app.AddConcept(handle, AddConceptRequest{
		Domain: domain,
		Terms: []TermDTO{
			{Text: text, Locale: locale, Status: "approved"},
		},
	}))
	after := app.SearchTerms(handle, "", "", "", 0, 200)
	for _, c := range after.Concepts {
		if !known[c.ID] {
			return c.ID
		}
	}
	t.Fatalf("new concept %q not found after add", text)
	return ""
}

func TestTermbase_RelationsRoundTrip(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	c1 := addConceptAndID(t, app, handle, "software", "file", "en-US")
	c2 := addConceptAndID(t, app, handle, "software", "document", "en-US")

	// No relations to begin with.
	rels, err := app.GetRelations(handle, c1)
	require.NoError(t, err)
	assert.Empty(t, rels)

	// Add a BROADER relation c1 → c2.
	rel, err := app.AddRelation(handle, AddRelationRequest{
		SourceID: c1,
		TargetID: c2,
		Type:     "BROADER",
		Note:     "a file is broader than a document",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, rel.ID)
	assert.Equal(t, c1, rel.SourceID)
	assert.Equal(t, c2, rel.TargetID)
	assert.Equal(t, "BROADER", rel.Type)
	assert.Equal(t, "a file is broader than a document", rel.Note)

	// Visible from BOTH endpoints (RelationsOf is direction-agnostic).
	relsFrom, err := app.GetRelations(handle, c1)
	require.NoError(t, err)
	require.Len(t, relsFrom, 1)
	assert.Equal(t, rel.ID, relsFrom[0].ID)

	relsTo, err := app.GetRelations(handle, c2)
	require.NoError(t, err)
	require.Len(t, relsTo, 1)
	assert.Equal(t, rel.ID, relsTo[0].ID)

	// Remove it; both endpoints go quiet.
	require.NoError(t, app.RemoveRelation(handle, rel.ID))
	relsFrom, err = app.GetRelations(handle, c1)
	require.NoError(t, err)
	assert.Empty(t, relsFrom)
	relsTo, err = app.GetRelations(handle, c2)
	require.NoError(t, err)
	assert.Empty(t, relsTo)
}

func TestTermbase_AddRelationValidates(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	c1 := addConceptAndID(t, app, handle, "software", "file", "en-US")
	c2 := addConceptAndID(t, app, handle, "software", "document", "en-US")

	// Unknown relation type is rejected by the framework AddRelation.
	_, err := app.AddRelation(handle, AddRelationRequest{
		SourceID: c1, TargetID: c2, Type: "FRIENDS_WITH",
	})
	require.Error(t, err)

	// A missing target concept is rejected.
	_, err = app.AddRelation(handle, AddRelationRequest{
		SourceID: c1, TargetID: "ghost", Type: "RELATED",
	})
	require.Error(t, err)

	// Lower-case type is normalised to its upper graph.Label form.
	rel, err := app.AddRelation(handle, AddRelationRequest{
		SourceID: c1, TargetID: c2, Type: " related ",
	})
	require.NoError(t, err)
	assert.Equal(t, "RELATED", rel.Type)
}

func TestTermbase_AddRelationWithValidity(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	c1 := addConceptAndID(t, app, handle, "brand", "old name", "en-US")
	c2 := addConceptAndID(t, app, handle, "brand", "new name", "en-US")

	from := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	rel, err := app.AddRelation(handle, AddRelationRequest{
		SourceID:  c1,
		TargetID:  c2,
		Type:      "REPLACED_BY",
		ValidFrom: from,
		Tags:      map[string]string{"market": "dach"},
	})
	require.NoError(t, err)
	require.NotNil(t, rel.Validity)
	assert.Equal(t, from, rel.Validity.ValidFrom)
	assert.Equal(t, map[string]string{"market": "dach"}, rel.Validity.Tags)

	// Validity round-trips through GetRelations.
	rels, err := app.GetRelations(handle, c1)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	require.NotNil(t, rels[0].Validity)
	assert.Equal(t, from, rels[0].Validity.ValidFrom)
	assert.Equal(t, "dach", rels[0].Validity.Tags["market"])
}

func TestTermbase_SetTermStatus(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	c1 := addConceptAndID(t, app, handle, "brand", "Hello", "en-US")

	until := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second).Format(time.RFC3339)
	require.NoError(t, app.SetTermStatus(handle, SetTermStatusRequest{
		ConceptID: c1,
		Locale:    "en-US",
		Text:      "Hello",
		Status:    "preferred",
		ValidTo:   until,
		Tags:      map[string]string{"market": "us"},
	}))

	// The status + validity persist and surface on the view DTO.
	view, err := app.GetConceptForView(handle, c1)
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Len(t, view.Terms, 1)
	assert.Equal(t, "preferred", view.Terms[0].Status)
	require.NotNil(t, view.Terms[0].Validity)
	assert.Equal(t, until, view.Terms[0].Validity.ValidTo)
	assert.Equal(t, "us", view.Terms[0].Validity.Tags["market"])

	// An unknown status is rejected.
	require.Error(t, app.SetTermStatus(handle, SetTermStatusRequest{
		ConceptID: c1, Locale: "en-US", Text: "Hello", Status: "bogus",
	}))

	// A term that does not exist is rejected.
	require.Error(t, app.SetTermStatus(handle, SetTermStatusRequest{
		ConceptID: c1, Locale: "fr-FR", Text: "Bonjour", Status: "preferred",
	}))
}

func TestTermbase_GetConceptForView_Missing(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTermbase(t, app)

	// Missing concept → (nil, nil): the adapter reads this as "no longer exists".
	view, err := app.GetConceptForView(handle, "does-not-exist")
	require.NoError(t, err)
	assert.Nil(t, view)

	// A bad handle is a real error.
	_, err = app.GetConceptForView("bogus-handle", "x")
	require.Error(t, err)
}
