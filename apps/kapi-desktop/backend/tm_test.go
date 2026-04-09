package backend

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := NewApp()
	t.Cleanup(func() {
		app.tmHandles.CloseAll()
		app.tbHandles.CloseAll()
	})
	return app
}

func openTestTM(t *testing.T, app *App) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	handle, err := app.OpenTM(path)
	require.NoError(t, err)
	require.NotEmpty(t, handle)
	t.Cleanup(func() { app.CloseTM(handle) })
	return handle
}

func TestTM_OpenAndClose(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	stats := app.GetTMStats(handle)
	require.NotNil(t, stats)
	assert.Equal(t, 0, stats.Count)

	app.CloseTM(handle)
	assert.Nil(t, app.GetTMStats(handle))
}

func TestTM_CRUD(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	// Add
	err := app.AddTMEntry(handle, AddTMEntryRequest{
		Source:       "Hello world",
		Target:       "Bonjour le monde",
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
	})
	require.NoError(t, err)

	stats := app.GetTMStats(handle)
	assert.Equal(t, 1, stats.Count)

	// Search
	result := app.SearchTMEntries(handle, "", "", "", 0, 50)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "Hello world", result.Entries[0].SourceText)
	assert.Equal(t, "Bonjour le monde", result.Entries[0].TargetText)
	assert.Equal(t, "en-US", result.Entries[0].SourceLocale)

	entryID := result.Entries[0].ID

	// Get
	entry := app.GetTMEntry(handle, entryID)
	require.NotNil(t, entry)
	assert.Equal(t, "Hello world", entry.SourceText)

	// Update
	err = app.UpdateTMEntry(handle, UpdateTMEntryRequest{
		EntryID:      entryID,
		Source:       "Hello world",
		Target:       "Bonjour le monde!",
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
	})
	require.NoError(t, err)

	entry = app.GetTMEntry(handle, entryID)
	assert.Equal(t, "Bonjour le monde!", entry.TargetText)

	// Delete
	err = app.DeleteTMEntry(handle, entryID)
	require.NoError(t, err)
	assert.Equal(t, 0, app.GetTMStats(handle).Count)
}

func TestTM_BatchDelete(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	for i := 0; i < 5; i++ {
		require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
			Source:       "Source " + string(rune('A'+i)),
			Target:       "Target " + string(rune('A'+i)),
			SourceLocale: "en", TargetLocale: "fr",
		}))
	}
	assert.Equal(t, 5, app.GetTMStats(handle).Count)

	result := app.SearchTMEntries(handle, "", "", "", 0, 50)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		ids = append(ids, result.Entries[i].ID)
	}

	require.NoError(t, app.DeleteTMEntries(handle, ids))
	assert.Equal(t, 2, app.GetTMStats(handle).Count)
}

func TestTM_Search(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Source: "The quick brown fox", Target: "Le renard brun rapide",
		SourceLocale: "en", TargetLocale: "fr",
	}))
	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Source: "Hello world", Target: "Bonjour le monde",
		SourceLocale: "en", TargetLocale: "fr",
	}))

	result := app.SearchTMEntries(handle, "fox", "", "", 0, 50)
	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, "The quick brown fox", result.Entries[0].SourceText)
}

func TestTM_FragmentToDTO(t *testing.T) {
	frag := &model.Fragment{}
	frag.AppendText("Click ")
	frag.AppendSpan(&model.Span{
		SpanType: model.SpanOpening, Type: "fmt:bold", Data: "<b>",
	})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{
		SpanType: model.SpanClosing, Type: "fmt:bold", Data: "</b>",
	})

	coded, spans := fragmentToDTO(frag)
	assert.Contains(t, coded, "Click")
	assert.Len(t, spans, 2)
	assert.Equal(t, "opening", spans[0].SpanType)
	assert.Equal(t, "fmt:bold", spans[0].Type)
	assert.Equal(t, "closing", spans[1].SpanType)
}

func TestTM_FragmentToDTO_Nil(t *testing.T) {
	coded, spans := fragmentToDTO(nil)
	assert.Empty(t, coded)
	assert.Nil(t, spans)
}

func TestTM_BuildFragmentWithEntities(t *testing.T) {
	frag := buildFragmentWithEntities("Bob works at Acme", []EntityAnnotationDTO{
		{Text: "Bob", Type: "entity:person", Start: 0, End: 3},
		{Text: "Acme", Type: "entity:organization", Start: 13, End: 17},
	})

	assert.True(t, frag.HasSpans())
	assert.Len(t, frag.Spans, 2)
	assert.Equal(t, "entity:person", frag.Spans[0].Type)
	assert.Equal(t, "Bob", frag.Spans[0].Data)
	assert.Equal(t, "entity:organization", frag.Spans[1].Type)
	assert.Equal(t, "Acme", frag.Spans[1].Data)

	// Generalized text should have typed placeholders.
	assert.Equal(t, "{PERSON} works at {ORGANIZATION}", frag.GeneralizedText())

	// Plain text returns only non-marker text (entity data lives in Span.Data).
	assert.Equal(t, " works at ", frag.Text())

	// But entity values are accessible via EntityValues().
	vals := frag.EntityValues()
	assert.Equal(t, "Bob", vals["e1"])
	assert.Equal(t, "Acme", vals["e2"])
}

func TestTM_BuildFragmentWithEntities_NoEntities(t *testing.T) {
	frag := buildFragmentWithEntities("plain text", nil)
	assert.Equal(t, "plain text", frag.Text())
	assert.False(t, frag.HasSpans())
}

func TestTM_EntityAwareLookup(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "lookup.db")
	handle, err := app.OpenTM(path)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseTM(handle) })

	// Add an entry with entity spans: "John is a hero" with John as entity:person.
	tm, ok := app.tmHandles.Get(handle)
	require.True(t, ok)

	frag := buildFragmentWithEntities("John is a hero", []EntityAnnotationDTO{
		{Text: "John", Type: "entity:person", Start: 0, End: 4},
	})
	tgtFrag := buildFragmentWithEntities("John est un héros", []EntityAnnotationDTO{
		{Text: "John", Type: "entity:person", Start: 0, End: 4},
	})

	entry := sievepen.TMEntry{
		ID:           "entry-1",
		Source:       frag,
		Target:       tgtFrag,
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
		Entities: []sievepen.EntityMapping{
			{
				PlaceholderID: "e1",
				Type:          model.EntityPerson,
				SourceValue:   "John",
				TargetValue:   "John",
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, tm.Add(entry))

	// Lookup "Bob is a hero" with Bob as entity:person.
	// Should match at 100% generalized-exact because both generalize to "{PERSON} is a hero".
	matches := app.LookupTM(handle, LookupTMRequest{
		Text: "Bob is a hero",
		Entities: []EntityAnnotationDTO{
			{Text: "Bob", Type: "entity:person", Start: 0, End: 3},
		},
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
		MinScore:     0.7,
		MaxResults:   10,
	})

	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, "generalized-exact", matches[0].MatchType)

	// Should have entity adaptation: John → Bob.
	require.Len(t, matches[0].EntityAdaptations, 1)
	assert.Equal(t, "John", matches[0].EntityAdaptations[0].StoredValue)
	assert.Equal(t, "Bob", matches[0].EntityAdaptations[0].CurrentValue)
	assert.Equal(t, "entity:person", matches[0].EntityAdaptations[0].Type)
}

func TestTM_AnnotateEntities(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	// Add entries with plain text.
	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Source: "Bob is a hero", Target: "Bob est un héros",
		SourceLocale: "en-US", TargetLocale: "fr-FR",
	}))
	require.NoError(t, app.AddTMEntry(handle, AddTMEntryRequest{
		Source: "Bob went home", Target: "Bob est rentré",
		SourceLocale: "en-US", TargetLocale: "fr-FR",
	}))

	// Get entry IDs.
	result := app.SearchTMEntries(handle, "", "", "", 0, 50)
	require.Equal(t, 2, result.TotalCount)
	ids := []string{result.Entries[0].ID, result.Entries[1].ID}

	// Annotate "Bob" as entity:person.
	annotateResult, err := app.AnnotateEntities(handle, AnnotateEntitiesRequest{
		EntryIDs: ids,
		Patterns: []EntityPatternRequest{
			{Text: "Bob", EntityType: "entity:person", CaseSensitive: true},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, annotateResult.EntriesUpdated)
	assert.True(t, annotateResult.EntitiesAdded > 0)

	// Verify entries now have entity spans.
	for _, eid := range ids {
		entry := app.GetTMEntry(handle, eid)
		require.NotNil(t, entry)
		// Source should have entity spans.
		hasEntitySpan := false
		for _, s := range entry.SourceSpans {
			if s.Type == "entity:person" {
				hasEntitySpan = true
				break
			}
		}
		assert.True(t, hasEntitySpan, "entry %s should have entity:person span in source", eid)
	}
}

func TestTM_FindPatternOccurrences(t *testing.T) {
	tests := []struct {
		text          string
		pattern       string
		caseSensitive bool
		expected      []int
	}{
		{"Bob is Bob", "Bob", true, []int{0, 7}},
		{"bob is Bob", "Bob", false, []int{0, 7}},
		{"bob is Bob", "Bob", true, []int{7}},
		{"hello", "xyz", true, nil},
		{"aaa", "aa", true, []int{0}}, // non-overlapping
	}

	for _, tt := range tests {
		positions := findPatternOccurrences(tt.text, tt.pattern, tt.caseSensitive)
		assert.Equal(t, tt.expected, positions, "text=%q pattern=%q cs=%v", tt.text, tt.pattern, tt.caseSensitive)
	}
}

func TestTM_CreateNamedTM(t *testing.T) {
	app := newTestApp(t)

	// Override config dir by setting KAPI_PLUGIN_DIR (doesn't affect us, but let's use temp dir).
	dir := t.TempDir()
	origDir := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", dir)
	t.Cleanup(func() {
		if origDir != "" {
			os.Setenv("XDG_CONFIG_HOME", origDir)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	})

	// Note: CreateNamedTM uses os.UserConfigDir() which may not respect XDG_CONFIG_HOME on macOS.
	// This test verifies the flow works with a direct path instead.
	path := filepath.Join(dir, "test-tm.db")
	handle, err := app.CreateTM(path)
	require.NoError(t, err)
	require.NotEmpty(t, handle)
	t.Cleanup(func() { app.CloseTM(handle) })

	stats := app.GetTMStats(handle)
	require.NotNil(t, stats)
	assert.Equal(t, 0, stats.Count)
}

func TestTM_ListNamedTMs(t *testing.T) {
	// This tests the listing function with a known directory.
	// We don't test the actual KAPI_HOME path to avoid side effects.
	list := listNamedResources("nonexistent-dir-12345")
	assert.Nil(t, list)
}

func TestTM_GetTMFacets(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	// Empty TM facets.
	facets := app.GetTMFacets(handle)
	require.NotNil(t, facets)
	assert.Empty(t, facets.LocalePairs)
	assert.Empty(t, facets.Projects)
	assert.Empty(t, facets.EntityTypes)
	assert.Equal(t, 0, facets.HasCodes)
	assert.Equal(t, 0, facets.NoCodes)

	// Add entries across locales and projects.
	tm, ok := app.tmHandles.Get(handle)
	require.True(t, ok)

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: "en-US", TargetLocale: "fr-FR", ProjectID: "proj-x",
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
		SourceLocale: "en-US", TargetLocale: "de-DE", ProjectID: "proj-x",
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e3", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: "en-US", TargetLocale: "fr-FR", ProjectID: "proj-y",
	}))

	facets = app.GetTMFacets(handle)
	require.NotNil(t, facets)
	assert.Len(t, facets.LocalePairs, 2)
	assert.Len(t, facets.Projects, 2)
	assert.Equal(t, 0, facets.HasCodes)
	assert.Equal(t, 3, facets.NoCodes)

	// Invalid handle returns nil.
	assert.Nil(t, app.GetTMFacets("bad-handle"))
}

func TestTM_SearchTMEntriesGrouped(t *testing.T) {
	app := newTestApp(t)
	handle := openTestTM(t, app)

	tm, ok := app.tmHandles.Get(handle)
	require.True(t, ok)

	// Same source, multiple target locales.
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello world"), Target: model.NewFragment("Bonjour le monde"),
		SourceLocale: "en-US", TargetLocale: "fr-FR",
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Hello world"), Target: model.NewFragment("Hallo Welt"),
		SourceLocale: "en-US", TargetLocale: "de-DE",
	}))
	// Different source.
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e3", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: "en-US", TargetLocale: "fr-FR",
	}))

	result := app.SearchTMEntriesGrouped(handle, "", "", 0, 10)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.TotalCount, "two distinct source texts")
	require.Len(t, result.Groups, 2)

	// Find the "Hello world" group and verify it has 2 targets.
	var helloGroup *TMGroupedResult
	for i := range result.Groups {
		if result.Groups[i].SourceText == "hello world" || result.Groups[i].SourceText == "Hello world" {
			helloGroup = &result.Groups[i]
			break
		}
	}
	require.NotNil(t, helloGroup, "should find 'Hello world' group")
	assert.Len(t, helloGroup.Targets, 2)
	assert.NotEmpty(t, helloGroup.SourceLocale)

	// Each target should have non-empty fields.
	for _, tgt := range helloGroup.Targets {
		assert.NotEmpty(t, tgt.ID)
		assert.NotEmpty(t, tgt.TargetText)
		assert.NotEmpty(t, tgt.TargetLocale)
	}

	// Invalid handle returns empty result.
	empty := app.SearchTMEntriesGrouped("bad-handle", "", "", 0, 10)
	require.NotNil(t, empty)
	assert.Equal(t, 0, empty.TotalCount)
}
