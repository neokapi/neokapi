package backend

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := NewApp()
	t.Cleanup(func() {
		app.memoryHandles.CloseAll()
		app.tbHandles.CloseAll()
	})
	return app
}

func openTestMemory(t *testing.T, app *App) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	handle, err := app.OpenMemory(path)
	require.NoError(t, err)
	require.NotEmpty(t, handle)
	t.Cleanup(func() { app.CloseMemory(handle) })
	return handle
}

// multilingualInput builds a variants map with three locales.
func multilingualInput(en, fr, de string) map[string]VariantInputDTO {
	return map[string]VariantInputDTO{
		"en-US": {Text: en},
		"fr-FR": {Text: fr},
		"de-DE": {Text: de},
	}
}

func TestMemory_AddMultilingualEntry(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)

	err := app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants:    multilingualInput("Hello", "Bonjour", "Hallo"),
		HintSrcLang: "en-US",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, app.GetMemoryStats(handle).Count)

	result := app.SearchMemoryEntries(handle, "", "", "", 0, 50)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Entries, 1)
	e := result.Entries[0]
	require.Contains(t, e.Variants, "en-US")
	assert.Equal(t, "Hello", e.Variants["en-US"].Text)
	assert.Equal(t, "Bonjour", e.Variants["fr-FR"].Text)
	assert.Equal(t, "Hallo", e.Variants["de-DE"].Text)
	assert.Equal(t, "en-US", e.HintSrcLang)
}

func TestMemory_UpdateEntry_VariantsMap(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)

	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants:    multilingualInput("Save", "Enregistrer", "Speichern"),
		HintSrcLang: "en-US",
	}))
	r := app.SearchMemoryEntries(handle, "", "", "", 0, 10)
	require.Len(t, r.Entries, 1)
	eid := r.Entries[0].ID

	// Replace: add Italian, drop German.
	require.NoError(t, app.UpdateMemoryEntry(handle, UpdateMemoryEntryRequest{
		EntryID: eid,
		Variants: map[string]VariantInputDTO{
			"en-US": {Text: "Save"},
			"fr-FR": {Text: "Enregistrer"},
			"it-IT": {Text: "Salva"},
		},
		HintSrcLang: "en-US",
	}))
	got := app.GetMemoryEntry(handle, eid)
	require.NotNil(t, got)
	assert.Contains(t, got.Variants, "it-IT")
	assert.NotContains(t, got.Variants, "de-DE")
}

func TestMemory_SearchReturnsVariants(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants:    multilingualInput("Hello world", "Bonjour monde", "Hallo welt"),
		HintSrcLang: "en-US",
	}))
	result := app.SearchMemoryEntries(handle, "monde", "", "", 0, 10)
	require.Equal(t, 1, result.TotalCount)
}

func TestMemory_GetFacets_LocalesAndSessions(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)

	// Seed a session and an entry tagged with it.
	tm, _ := app.memoryHandles.Get(handle)
	require.NoError(t, tm.CreateImportSession(t.Context(), memory.ImportSession{
		ID: "s1", FileKey: "seed.tmx", ImportedAt: time.Now(),
	}))
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "hi"}}},
			"fr-FR": {{Text: &model.TextRun{Text: "salut"}}},
		},
		Origins: []memory.Origin{{Source: "import", SessionID: "s1"}},
	}))

	facets := app.GetMemoryFacets(handle)
	require.NotNil(t, facets)
	assert.NotEmpty(t, facets.Locales)
	assert.Len(t, facets.ImportSessions, 1)
	assert.Equal(t, "s1", facets.ImportSessions[0].SessionID)
}

func TestMemory_ListImportSessions(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	tm, _ := app.memoryHandles.Get(handle)
	require.NoError(t, tm.CreateImportSession(t.Context(), memory.ImportSession{
		ID: "s1", FileKey: "a.tmx", ImportedAt: time.Now(),
	}))
	sessions := app.ListMemoryImportSessions(handle)
	require.Len(t, sessions, 1)
	assert.Equal(t, "a.tmx", sessions[0].FileKey)
}

func TestMemory_GetImportSession_NotFound(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	assert.Nil(t, app.GetMemoryImportSession(handle, "missing"))
}

func TestMemory_DeleteImportSession_KeepsEntries(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	tm, _ := app.memoryHandles.Get(handle)
	require.NoError(t, tm.CreateImportSession(t.Context(), memory.ImportSession{
		ID: "s1", FileKey: "a.tmx", ImportedAt: time.Now(),
	}))
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "hi"}}},
			"fr-FR": {{Text: &model.TextRun{Text: "salut"}}},
		},
		Origins: []memory.Origin{{Source: "import", SessionID: "s1"}},
	}))
	require.NoError(t, app.DeleteMemoryImportSession(handle, "s1"))
	got := app.GetMemoryEntry(handle, "e1")
	require.NotNil(t, got)
	require.Len(t, got.Origins, 1)
	assert.Empty(t, got.Origins[0].SessionID)
}

func TestMemory_AnnotateEntities_ResolvesConceptID(t *testing.T) {
	app := newTestApp(t)
	memoryHandle := openTestMemory(t, app)

	// Add a content-memory entry with "Acme" in the text.
	require.NoError(t, app.AddMemoryEntry(memoryHandle, AddMemoryEntryRequest{
		Variants: map[string]VariantInputDTO{
			"en-US": {Text: "Contact Acme for support"},
			"fr-FR": {Text: "Contactez Acme pour le support"},
		},
		HintSrcLang: "en-US",
	}))
	r := app.SearchMemoryEntries(memoryHandle, "", "", "", 0, 10)
	require.Len(t, r.Entries, 1)
	entryID := r.Entries[0].ID

	// Create a terms store with "Acme" as an organization concept.
	tbPath := filepath.Join(t.TempDir(), "tb.db")
	tb, err := terms.NewSQLiteStore(tbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:     "concept-acme",
		Domain: "brand",
		Terms: []terms.Term{
			{Text: "Acme", Locale: "en-US", Status: model.TermApproved},
			{Text: "Acme", Locale: "fr-FR", Status: model.TermApproved},
		},
	}))
	tbHandle := app.tbHandles.Open(projector.StandaloneTerms(tb))
	t.Cleanup(func() { app.tbHandles.Close(tbHandle) })

	// Annotate: mark "Acme" as entity:organization — with terms handle
	// so the concept ID gets resolved automatically.
	result, err := app.AnnotateEntities(memoryHandle, AnnotateEntitiesRequest{
		EntryIDs: []string{entryID},
		Patterns: []EntityPatternRequest{
			{Text: "Acme", EntityType: "entity:organization", CaseSensitive: true},
		},
		TermsHandle: tbHandle,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.EntriesUpdated)
	assert.GreaterOrEqual(t, result.EntitiesAdded, 1)

	// Verify the entity got the concept_id from the terms store.
	got := app.GetMemoryEntry(memoryHandle, entryID)
	require.NotNil(t, got)
	require.NotEmpty(t, got.Entities)
	assert.Equal(t, "concept-acme", got.Entities[0].ConceptID,
		"entity should be cross-referenced to the terms store concept")
}

func TestMemory_ResolveEntityConcepts(t *testing.T) {
	app := newTestApp(t)
	memoryHandle := openTestMemory(t, app)

	// Add a content-memory entry with an entity that has no concept ID.
	tm, _ := app.memoryHandles.Get(memoryHandle)
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en-US": {{Text: &model.TextRun{Text: "hello"}}},
		},
		Entities: []memory.EntityMapping{{
			PlaceholderID: "e1",
			Type:          "entity:product",
			Values:        map[model.LocaleID]memory.EntityValue{"en-US": {Text: "Widget"}},
		}},
	}))

	// Create terms with matching concept.
	tbPath := filepath.Join(t.TempDir(), "tb.db")
	tb, err := terms.NewSQLiteStore(tbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "concept-widget",
		Terms: []terms.Term{
			{Text: "Widget", Locale: "en-US", Status: model.TermApproved},
		},
	}))
	tbHandle := app.tbHandles.Open(projector.StandaloneTerms(tb))
	t.Cleanup(func() { app.tbHandles.Close(tbHandle) })

	// Resolve — should link the entity to the concept.
	updated, err := app.ResolveEntityConcepts(memoryHandle, tbHandle, []string{"e1"}, false)
	require.NoError(t, err)
	assert.Equal(t, 1, updated)

	got := app.GetMemoryEntry(memoryHandle, "e1")
	require.NotNil(t, got)
	assert.Equal(t, "concept-widget", got.Entities[0].ConceptID)
}

// TestMemory_LookupMemory_MatchesTheRequestText guards the source the lookup
// block carries: the request text, with each entity turned into a placeholder
// run. A lookup block without that source matches nothing.
func TestMemory_LookupMemory_MatchesTheRequestText(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants: map[string]VariantInputDTO{
			"en-US": {Text: "Contact Acme for support"},
			"fr-FR": {Text: "Contactez Acme pour le support"},
		},
		HintSrcLang: "en-US",
	}))

	tests := []struct {
		name      string
		entities  []EntityAnnotationDTO
		wantExact bool
	}{
		{name: "plain text matches exactly", wantExact: true},
		{
			name:     "an entity placeholder still finds the entry",
			entities: []EntityAnnotationDTO{{Text: "Acme", Type: "entity:organization", Start: 8, End: 12}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := app.LookupMemory(handle, LookupMemoryRequest{
				Text:         "Contact Acme for support",
				Entities:     tt.entities,
				SourceLocale: "en-US",
				TargetLocale: "fr-FR",
				MinScore:     0.5,
			})
			require.Len(t, got, 1)
			assert.Equal(t, "Contactez Acme pour le support", got[0].Entry.Variants["fr-FR"].Text)
			if tt.wantExact {
				assert.InDelta(t, 1.0, got[0].Score, 1e-9)
			} else {
				assert.Less(t, got[0].Score, 1.0, "the placeholder changes the structure, so the match is not exact")
			}
		})
	}
}

// TestMemory_LookupMemory_AdaptsTheRequestEntity guards the entity half of the
// lookup: each entity the request names is located on the lookup block by an
// entity overlay span over its placeholder run, and the stores read that
// overlay, so a generalized match adapts the stored entity to the requested
// one. The span was anchored by byte offsets into text that skips placeholders,
// which put it on the text after the placeholder, and the stores read the
// block's annotations map, which held none.
func TestMemory_LookupMemory_AdaptsTheRequestEntity(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants: map[string]VariantInputDTO{
			"en-US": {Text: "Contact Acme for support"},
			"fr-FR": {Text: "Contactez Acme pour le support"},
		},
		HintSrcLang: "en-US",
	}))
	entries := app.SearchMemoryEntries(handle, "", "", "", 0, 10)
	require.Len(t, entries.Entries, 1)
	annotated, err := app.AnnotateEntities(handle, AnnotateEntitiesRequest{
		EntryIDs: []string{entries.Entries[0].ID},
		Patterns: []EntityPatternRequest{{Text: "Acme", EntityType: "entity:organization"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, annotated.EntriesUpdated)

	got := app.LookupMemory(handle, LookupMemoryRequest{
		Text:         "Contact Globex for support",
		Entities:     []EntityAnnotationDTO{{Text: "Globex", Type: "entity:organization", Start: 8, End: 14}},
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
		MinScore:     0.5,
	})
	require.Len(t, got, 1)
	require.Len(t, got[0].EntityAdaptations, 1, "the stored entity is adapted to the requested one")
	a := got[0].EntityAdaptations[0]
	assert.Equal(t, "Acme", a.StoredValue)
	assert.Equal(t, "Globex", a.CurrentValue)
	assert.Equal(t, "entity:organization", a.Type)
}

// TestMemory_AnnotateEntities_PairsAnEntityAcrossVariants holds one entity to
// one placeholder id in every variant, whatever order a translation puts the
// entities in, so a lookup adapts each stored entity to the one the request
// names in its place.
func TestMemory_AnnotateEntities_PairsAnEntityAcrossVariants(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants: map[string]VariantInputDTO{
			"en-US": {Text: "Ask Acme or Initech."},
			"fr-FR": {Text: "Demandez à Initech ou Acme."},
		},
		HintSrcLang: "en-US",
	}))
	entries := app.SearchMemoryEntries(handle, "", "", "", 0, 10)
	require.Len(t, entries.Entries, 1)
	_, err := app.AnnotateEntities(handle, AnnotateEntitiesRequest{
		EntryIDs: []string{entries.Entries[0].ID},
		Patterns: []EntityPatternRequest{
			{Text: "Acme", EntityType: "entity:organization", CaseSensitive: true},
			{Text: "Initech", EntityType: "entity:organization", CaseSensitive: true},
		},
	})
	require.NoError(t, err)

	got := app.GetMemoryEntry(handle, entries.Entries[0].ID)
	require.NotNil(t, got)
	for _, em := range got.Entities {
		assert.Equal(t, em.Values["en-US"].Text, em.Values["fr-FR"].Text, "%s pairs one entity in both variants", em.PlaceholderID)
	}

	matches := app.LookupMemory(handle, LookupMemoryRequest{
		Text: "Ask Globex or Hooli.",
		Entities: []EntityAnnotationDTO{
			{Text: "Globex", Type: "entity:organization", Start: 4, End: 10},
			{Text: "Hooli", Type: "entity:organization", Start: 14, End: 19},
		},
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
		MinScore:     0.5,
	})
	require.Len(t, matches, 1)
	adapted := map[string]string{}
	for _, a := range matches[0].EntityAdaptations {
		adapted[a.StoredValue] = a.CurrentValue
	}
	assert.Equal(t, map[string]string{"Acme": "Globex", "Initech": "Hooli"}, adapted)
}

// TestLookupBlock_AnchorsEachEntityToItsPlaceholder holds the overlay span of
// every entity to the placeholder run the entity became, wherever it sits and
// whatever the text before it is written in. Offsets count code points.
func TestLookupBlock_AnchorsEachEntityToItsPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		entities []EntityAnnotationDTO
		want     []string
	}{
		{
			name:     "one entity",
			text:     "Contact Acme for support",
			entities: []EntityAnnotationDTO{{Text: "Acme", Type: "entity:organization", Start: 8, End: 12}},
			want:     []string{"Acme"},
		},
		{
			name: "two entities after text that is not ASCII",
			text: "Kontakt för Acme och Globex idag",
			entities: []EntityAnnotationDTO{
				{Text: "Globex", Type: "entity:organization", Start: 21, End: 27},
				{Text: "Acme", Type: "entity:organization", Start: 12, End: 16},
			},
			want: []string{"Acme", "Globex"},
		},
		{
			name: "an entity that overlaps an earlier one is left out",
			text: "Contact Acme Corp now",
			entities: []EntityAnnotationDTO{
				{Text: "Acme Corp", Type: "entity:organization", Start: 8, End: 17},
				{Text: "Corp", Type: "entity:organization", Start: 13, End: 17},
			},
			want: []string{"Acme Corp"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := lookupBlock(tt.text, tt.entities)
			overlay := b.OverlayOf(model.OverlayEntity)
			require.NotNil(t, overlay)
			require.Len(t, overlay.Spans, len(tt.want))
			for i, sp := range overlay.Spans {
				covered := sp.Range.ExtractRuns(b.SourceRuns())
				require.Len(t, covered, 1, "the span covers the placeholder run and nothing else")
				require.NotNil(t, covered[0].Ph)
				assert.Equal(t, tt.want[i], covered[0].Ph.Data)
				assert.Equal(t, tt.want[i], sp.Value.(*model.EntityAnnotation).Text)
			}
		})
	}
}

func TestMemory_DeleteEntry(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)
	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		Variants:    multilingualInput("Save", "Enregistrer", "Speichern"),
		HintSrcLang: "en-US",
	}))
	r := app.SearchMemoryEntries(handle, "", "", "", 0, 10)
	require.Len(t, r.Entries, 1)
	require.NoError(t, app.DeleteMemoryEntry(handle, r.Entries[0].ID))
	assert.Equal(t, 0, app.GetMemoryStats(handle).Count)
}
