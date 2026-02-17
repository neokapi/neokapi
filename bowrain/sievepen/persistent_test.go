package sievepen_test

import (
	"testing"
	"time"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/sievepen"

	sqltm "github.com/gokapi/gokapi/bowrain/sievepen"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteTM_AddAndLookup(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	entry := sievepen.TMEntry{
		ID:           "entry-1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Properties:   map[string]string{"domain": "general"},
	}

	err = tm.Add(entry)
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.TargetText())
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_ExactMatch(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Save"), Target: model.NewFragment("Sauvegarder"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Cancel"), Target: model.NewFragment("Annuler"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.LookupText("Save", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Sauvegarder", matches[0].Entry.TargetText())
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_FuzzyMatch(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("The file was saved successfully"),
		Target:       model.NewFragment("Le fichier a ete sauvegarde avec succes"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.LookupText("The file was saved", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 0.5, MaxResults: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, sievepen.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
	assert.Less(t, matches[0].Score, 1.0)
}

func TestSQLiteTM_Delete(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	assert.Equal(t, 2, tm.Count())

	err = tm.Delete("e1")
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, matches)

	matches, err = tm.LookupText("Goodbye", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.TargetText())

	err = tm.Delete("non-existent")
	assert.Error(t, err)
}

func TestSQLiteTM_EmptyIDError(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	err = tm.Add(sievepen.TMEntry{
		Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	})
	assert.Error(t, err)
}

func TestSQLiteTM_UpdateExisting(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	// Update with same ID.
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Salut"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	assert.Equal(t, 1, tm.Count())
	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Salut", matches[0].Entry.TargetText())
}

func TestSQLiteTM_LocaleFiltering(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleGerman,
	}))

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.TargetText())

	matches, err = tm.LookupText("Hello", model.LocaleEnglish, model.LocaleGerman, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Hallo", matches[0].Entry.TargetText())
}

func TestSQLiteTM_Entries(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
		Properties: map[string]string{"domain": "general"},
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	entries := tm.Entries()
	assert.Len(t, entries, 2)
}

func TestSQLiteTM_SearchEntries(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello World"), Target: model.NewFragment("Bonjour le monde"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e3", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleGerman,
	}))

	// No filter returns all entries
	entries, total := tm.SearchEntries("", "", "", 0, 100)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 3)

	// Search by query (case-insensitive, matches source)
	entries, total = tm.SearchEntries("hello", "", "", 0, 100)
	assert.Equal(t, 2, total)
	assert.Len(t, entries, 2)

	// Search by query matches target
	entries, total = tm.SearchEntries("revoir", "", "", 0, 100)
	assert.Equal(t, 1, total)
	assert.Equal(t, "e2", entries[0].ID)

	// Filter by target locale
	entries, total = tm.SearchEntries("", "", "de", 0, 100)
	assert.Equal(t, 1, total)
	assert.Equal(t, "e3", entries[0].ID)

	// Pagination
	entries, total = tm.SearchEntries("", "", "", 0, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 2)

	entries, total = tm.SearchEntries("", "", "", 2, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 1)
}

func TestSQLiteTM_GetEntry(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	entry, ok := tm.GetEntry("e1")
	assert.True(t, ok)
	assert.Equal(t, "Hello", entry.SourceText())
	assert.Equal(t, "Bonjour", entry.TargetText())

	_, ok = tm.GetEntry("nonexistent")
	assert.False(t, ok)
}

func TestSQLiteTM_InterfaceCompliance(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)

	// Verify it satisfies the TranslationMemory and EntryProvider interfaces.
	var _ sievepen.TranslationMemory = tm
	var _ sievepen.EntryProvider = tm

	err = tm.Close()
	assert.NoError(t, err)
}

func TestSQLiteTM_BlockLookup(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Click the Save button"),
		Target:       model.NewFragment("Cliquez sur le bouton Enregistrer"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Block-based lookup.
	block := model.NewBlock("tu1", "Click the Save button")
	matches, err := tm.Lookup(block, model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, "Cliquez sur le bouton Enregistrer", matches[0].Entry.TargetText())
}

func TestSQLiteTM_FragmentRoundtrip(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	// Create a fragment with inline span.
	frag := model.NewFragment("Click ")
	frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, ID: "1", Type: "bold"})
	frag.AppendText("here")
	frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, ID: "1", Type: "bold"})
	frag.AppendText(" to continue")

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       frag,
		Target:       model.NewFragment("Cliquez ici pour continuer"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Retrieve and verify the Fragment survived serialization.
	entry, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.Equal(t, "Click here to continue", entry.SourceText())
	assert.True(t, entry.Source.HasSpans())
	assert.Len(t, entry.Source.Spans, 2)
}

func TestSQLiteTM_TimestampPreservation(t *testing.T) {
	tm, err := sqltm.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	now := time.Now().Truncate(time.Second) // SQLite stores second precision
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
		CreatedAt: now, UpdatedAt: now,
	}))

	entry, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.WithinDuration(t, now, entry.CreatedAt, time.Second)
	assert.WithinDuration(t, now, entry.UpdatedAt, time.Second)
}
