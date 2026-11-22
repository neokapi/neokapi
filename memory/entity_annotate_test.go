package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func TestMarkEntities_PlacesEachOccurrence(t *testing.T) {
	runs, n := MarkEntities(textRuns("Acme helps acme users"), []EntityPattern{{Text: "Acme", Type: "entity:organization"}})
	require.Equal(t, 2, n, "matching ignores case unless asked")
	require.Len(t, runs, 4)
	assert.Equal(t, "e1", runs[0].Ph.ID)
	assert.Equal(t, "Acme", runs[0].Ph.Data)
	assert.Equal(t, " helps ", runs[1].Text.Text)
	assert.Equal(t, "e1.2", runs[2].Ph.ID, "a second occurrence of one pattern")
	assert.Equal(t, "acme", runs[2].Ph.Data, "the placeholder keeps the text as written")

	_, n = MarkEntities(textRuns("Acme helps acme users"), []EntityPattern{{Text: "Acme", Type: "entity:organization", CaseSensitive: true}})
	assert.Equal(t, 1, n)
}

func TestMarkEntities_KeepsTheEarlierOfOverlappingMatches(t *testing.T) {
	runs, n := MarkEntities(textRuns("New York City"), []EntityPattern{
		{Text: "York City", Type: "entity:location"},
		{Text: "New York", Type: "entity:location"},
	})
	require.Equal(t, 1, n)
	assert.Equal(t, "New York", runs[0].Ph.Data)
	assert.Equal(t, "e2", runs[0].Ph.ID, "the id follows the pattern, not the position")
}

func TestPlaceEntities_SkipsSpansOutsideOrOverlapping(t *testing.T) {
	runs, placed := PlaceEntities("héllo world", []EntitySpan{
		{ID: "e1", Text: "héllo", Type: "entity:other", Start: 0, End: 5},
		{ID: "e2", Text: "llo", Type: "entity:other", Start: 2, End: 5},
		{ID: "e3", Text: "x", Type: "entity:other", Start: 40, End: 41},
	})
	require.Len(t, placed, 1)
	assert.Equal(t, 0, placed[0].Run)
	require.Len(t, runs, 2)
	assert.Equal(t, " world", runs[1].Text.Text)
}

func TestAnnotateEntries_PairsAcrossVariantsAndLinksConcepts(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	require.NoError(t, store.Add(ctx, Entry{ID: "one", Variants: map[model.LocaleID][]model.Run{
		"en-US": textRuns("Contact Acme today"),
		"fr-FR": textRuns("Contactez Acme aujourd'hui"),
	}}))
	require.NoError(t, store.Add(ctx, Entry{ID: "two", Variants: map[model.LocaleID][]model.Run{
		"en-US": textRuns("Nothing here"),
	}}))

	asked := 0
	resolve := func(_ context.Context, text string, _ model.LocaleID) (string, bool) {
		asked++
		return "concept-" + text, text == "Acme"
	}
	res, err := AnnotateEntries(ctx, store, []string{"one", "two", "missing"},
		[]EntityPattern{{Text: "Acme", Type: "entity:organization"}}, resolve)
	require.NoError(t, err)
	assert.Equal(t, AnnotateResult{EntriesUpdated: 1, EntitiesAdded: 2}, res)

	got, found, err := store.GetEntry(ctx, "one")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, got.Entities, 1, "one entity, paired across both variants")
	em := got.Entities[0]
	assert.Equal(t, "e1", em.PlaceholderID)
	assert.Len(t, em.Values, 2)
	assert.Equal(t, "concept-Acme", em.ConceptID)

	// Linking again leaves a linked entity alone unless forced.
	asked = 0
	n, err := LinkEntryConcepts(ctx, store, []string{"one"}, false, resolve)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Zero(t, asked)
	other := func(context.Context, string, model.LocaleID) (string, bool) { return "concept-other", true }
	n, err = LinkEntryConcepts(ctx, store, []string{"one"}, true, other)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	got, _, _ = store.GetEntry(ctx, "one")
	assert.Equal(t, "concept-other", got.Entities[0].ConceptID)
}
