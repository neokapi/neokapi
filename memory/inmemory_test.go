package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryMemory_MultilingualAdd(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Save", "Enregistrer", "Speichern")))
	assert.Equal(t, 1, mustCount(t, tm))
	got, ok := mustGetEntry(t, tm, "e1")
	require.True(t, ok)
	assert.True(t, got.HasLocale("fr"))
}

func TestInMemoryMemory_LookupExact(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Save", "Enregistrer", "Speichern")))
	matches, err := tm.LookupText(context.Background(), "Save", "en", "fr", memory.LookupOptions{MinScore: 1.0, MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Enregistrer", matches[0].Entry.VariantText("fr"))
}

func TestInMemoryMemory_LookupCrossDirection(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Save", "Enregistrer", "Speichern")))
	matches, err := tm.LookupText(context.Background(), "Enregistrer", "fr", "de", memory.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Speichern", matches[0].Entry.VariantText("de"))
}

func TestInMemoryMemory_LookupMissingTarget(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Save"}}},
			"fr": {{Text: &model.TextRun{Text: "Enregistrer"}}},
		},
	}))
	matches, _ := tm.LookupText(context.Background(), "Save", "en", "de", memory.DefaultLookupOptions())
	assert.Empty(t, matches)
}

func TestInMemoryMemory_LookupFuzzy(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1",
		"The file was saved successfully",
		"Le fichier a été sauvegardé",
		"Die Datei wurde gespeichert")))
	matches, err := tm.LookupText(context.Background(), "The file was saved", "en", "fr",
		memory.LookupOptions{MinScore: 0.5, MaxResults: 5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, memory.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
}

func TestInMemoryMemory_SearchAnyLocale(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "hello world", "bonjour monde", "hallo welt")))
	require.NoError(t, tm.Add(context.Background(), trilingual("e2", "goodbye", "au revoir", "auf wiedersehen")))
	entries, total, _ := tm.SearchEntries(context.Background(), memory.SearchParams{Query: "monde", AnyLocale: "", RequireLocale: "", Offset: 0, Limit: 10})
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestInMemoryMemory_SearchRequireLocale(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), memory.Entry{
		ID: "e1",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "hello"}}},
			"fr": {{Text: &model.TextRun{Text: "bonjour"}}},
		},
	}))
	require.NoError(t, tm.Add(context.Background(), memory.Entry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "hello"}}},
		},
	}))
	entries, total, _ := tm.SearchEntries(context.Background(), memory.SearchParams{Query: "hello", AnyLocale: "en", RequireLocale: "fr", Offset: 0, Limit: 10})
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestInMemoryMemory_Delete(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Delete(context.Background(), "e1"))
	assert.Equal(t, 0, mustCount(t, tm))
}

func TestInMemoryMemory_FacetStats(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Add(context.Background(), memory.Entry{
		ID: "e2",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "d"}}},
			"fr": {{Text: &model.TextRun{Text: "e"}}},
		},
	}))
	facets := mustFacetStats(t, tm)
	counts := map[string]int{}
	for _, l := range facets.Locales {
		counts[l.Locale] = l.Count
	}
	assert.Equal(t, 2, counts["en"])
	assert.Equal(t, 2, counts["fr"])
	assert.Equal(t, 1, counts["de"])
}

func TestInMemoryMemory_ImportSessionCRUD(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.CreateImportSession(context.Background(), memory.ImportSession{
		ID: "s1", FileKey: "a.tmx", FileHash: "deadbeef",
		ImportedAt: time.Now(),
	}))
	s, ok := mustGetImportSession(t, tm, "s1")
	require.True(t, ok)
	assert.Equal(t, "a.tmx", s.FileKey)

	require.NoError(t, tm.UpdateImportSessionCount(context.Background(), "s1", 7))
	s, _ = mustGetImportSession(t, tm, "s1")
	assert.Equal(t, 7, s.EntryCount)

	hit, ok := mustFindImportSessionByHash(t, tm, "deadbeef")
	require.True(t, ok)
	assert.Equal(t, "s1", hit.ID)

	require.NoError(t, tm.DeleteImportSession(context.Background(), "s1"))
	_, ok = mustGetImportSession(t, tm, "s1")
	assert.False(t, ok)
}

// --- Interface compliance ---

var _ memory.Store = (*memory.InMemoryStore)(nil)
var _ memory.Store = (*memory.SQLiteStore)(nil)

func TestInMemoryMemory_InterfaceCompliance(t *testing.T) {
	var _ memory.ContentMemory = memory.NewInMemoryStore()
}

// --- Basic helpers ---

func TestMatchType_IsExact(t *testing.T) {
	assert.True(t, memory.MatchGeneralizedExact.IsExact())
	assert.True(t, memory.MatchExact.IsExact())
	assert.True(t, memory.MatchStructuralExact.IsExact())
	assert.False(t, memory.MatchFuzzy.IsExact())
}

func TestLevenshteinRatio_Basic(t *testing.T) {
	r := memory.LevenshteinRatio("hello", "hello")
	assert.Equal(t, 1.0, r)
	r = memory.LevenshteinRatio("hello", "world")
	assert.Less(t, r, 0.5)
}

// LookupSegment — AD-017/AD-009 (issue #417).

func TestInMemoryMemory_LookupSegment_ExactMatchOnSpecificSegment(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "Save the file.", "Enregistrer le fichier.", "Datei speichern.")))
	require.NoError(t, tm.Add(context.Background(), trilingual("e2", "It was successful.", "C'était un succès.", "Es war erfolgreich.")))

	// Two-segment source block mirroring a post-segmentation state: a flat
	// run sequence with a stand-off segmentation overlay marking the boundaries.
	block := &model.Block{
		ID: "u1",
		Source: []model.Run{
			{Text: &model.TextRun{Text: "Save the file."}},
			{Text: &model.TextRun{Text: "It was successful."}},
		},
	}
	block.SetSegmentation(nil, []model.Span{
		{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "s2", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	})

	// Segment 0 matches entry e1.
	matches, err := tm.LookupSegment(context.Background(), block, 0, "en", "fr", memory.LookupOptions{MinScore: 0.9, MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Enregistrer le fichier.", matches[0].Entry.VariantText("fr"))

	// Segment 1 matches entry e2.
	matches, err = tm.LookupSegment(context.Background(), block, 1, "en", "fr", memory.LookupOptions{MinScore: 0.9, MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "C'était un succès.", matches[0].Entry.VariantText("fr"))
}

func TestInMemoryMemory_LookupSegment_OutOfRange(t *testing.T) {
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(context.Background(), trilingual("e1", "hi", "salut", "hallo")))
	block := &model.Block{
		ID:     "u1",
		Source: []model.Run{{Text: &model.TextRun{Text: "hi"}}},
	}
	matches, err := tm.LookupSegment(context.Background(), block, 5, "en", "fr", memory.DefaultLookupOptions())
	require.NoError(t, err)
	assert.Empty(t, matches)

	matches, err = tm.LookupSegment(context.Background(), block, -1, "en", "fr", memory.DefaultLookupOptions())
	require.NoError(t, err)
	assert.Empty(t, matches)

	matches, err = tm.LookupSegment(context.Background(), nil, 0, "en", "fr", memory.DefaultLookupOptions())
	require.NoError(t, err)
	assert.Empty(t, matches)
}
