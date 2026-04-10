package sievepen_test

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryTM_MultilingualAdd(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1", "Save", "Enregistrer", "Speichern")))
	assert.Equal(t, 1, tm.Count())
	got, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.True(t, got.HasLocale("fr"))
}

func TestInMemoryTM_LookupExact(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1", "Save", "Enregistrer", "Speichern")))
	matches, err := tm.LookupText("Save", "en", "fr", sievepen.LookupOptions{MinScore: 1.0, MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Enregistrer", matches[0].Entry.VariantText("fr"))
}

func TestInMemoryTM_LookupCrossDirection(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1", "Save", "Enregistrer", "Speichern")))
	matches, err := tm.LookupText("Enregistrer", "fr", "de", sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Speichern", matches[0].Entry.VariantText("de"))
}

func TestInMemoryTM_LookupMissingTarget(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("Save"),
			"fr": model.NewFragment("Enregistrer"),
		},
	}))
	matches, _ := tm.LookupText("Save", "en", "de", sievepen.DefaultLookupOptions())
	assert.Empty(t, matches)
}

func TestInMemoryTM_LookupFuzzy(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1",
		"The file was saved successfully",
		"Le fichier a été sauvegardé",
		"Die Datei wurde gespeichert")))
	matches, err := tm.LookupText("The file was saved", "en", "fr",
		sievepen.LookupOptions{MinScore: 0.5, MaxResults: 5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, sievepen.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
}

func TestInMemoryTM_SearchAnyLocale(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1", "hello world", "bonjour monde", "hallo welt")))
	require.NoError(t, tm.Add(trilingual("e2", "goodbye", "au revoir", "auf wiedersehen")))
	entries, total := tm.SearchEntries("monde", "", "", 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestInMemoryTM_SearchRequireLocale(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("hello"),
			"fr": model.NewFragment("bonjour"),
		},
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("hello"),
		},
	}))
	entries, total := tm.SearchEntries("hello", "en", "fr", 0, 10)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, "e1", entries[0].ID)
}

func TestInMemoryTM_Delete(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Delete("e1"))
	assert.Equal(t, 0, tm.Count())
}

func TestInMemoryTM_FacetStats(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(trilingual("e1", "a", "b", "c")))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2",
		Variants: map[model.LocaleID]*model.Fragment{
			"en": model.NewFragment("d"),
			"fr": model.NewFragment("e"),
		},
	}))
	facets := tm.FacetStats()
	counts := map[string]int{}
	for _, l := range facets.Locales {
		counts[l.Locale] = l.Count
	}
	assert.Equal(t, 2, counts["en"])
	assert.Equal(t, 2, counts["fr"])
	assert.Equal(t, 1, counts["de"])
}

func TestInMemoryTM_ImportSessionCRUD(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.CreateImportSession(sievepen.ImportSession{
		ID: "s1", FileKey: "a.tmx", FileHash: "deadbeef",
		ImportedAt: time.Now(),
	}))
	s, ok := tm.GetImportSession("s1")
	require.True(t, ok)
	assert.Equal(t, "a.tmx", s.FileKey)

	require.NoError(t, tm.UpdateImportSessionCount("s1", 7))
	s, _ = tm.GetImportSession("s1")
	assert.Equal(t, 7, s.EntryCount)

	hit, ok := tm.FindImportSessionByHash("deadbeef")
	require.True(t, ok)
	assert.Equal(t, "s1", hit.ID)

	require.NoError(t, tm.DeleteImportSession("s1"))
	_, ok = tm.GetImportSession("s1")
	assert.False(t, ok)
}

// --- Interface compliance ---

var _ sievepen.TMStore = (*sievepen.InMemoryTM)(nil)
var _ sievepen.TMStore = (*sievepen.SQLiteTM)(nil)

func TestInMemoryTM_InterfaceCompliance(t *testing.T) {
	var _ sievepen.TranslationMemory = sievepen.NewInMemoryTM()
}

// --- Basic helpers ---

func TestMatchType_IsExact(t *testing.T) {
	assert.True(t, sievepen.MatchGeneralizedExact.IsExact())
	assert.True(t, sievepen.MatchExact.IsExact())
	assert.True(t, sievepen.MatchStructuralExact.IsExact())
	assert.False(t, sievepen.MatchFuzzy.IsExact())
}

func TestLevenshteinRatio_Basic(t *testing.T) {
	r := sievepen.LevenshteinRatio("hello", "hello")
	assert.Equal(t, 1.0, r)
	r = sievepen.LevenshteinRatio("hello", "world")
	assert.Less(t, r, 0.5)
}
