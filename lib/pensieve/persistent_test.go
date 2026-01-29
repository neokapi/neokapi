package pensieve_test

import (
	"testing"
	"time"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/lib/pensieve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteTM_AddAndLookup(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	entry := pensieve.TMEntry{
		ID:           "entry-1",
		Source:       "Hello",
		Target:       "Bonjour",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Properties:   map[string]string{"domain": "general"},
	}

	err = tm.Add(entry)
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.Target)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, pensieve.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_ExactMatch(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "Save", Target: "Sauvegarder",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e2", Source: "Cancel", Target: "Annuler",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.Lookup("Save", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Sauvegarder", matches[0].Entry.Target)
	assert.Equal(t, pensieve.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_FuzzyMatch(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "The file was saved successfully",
		Target:       "Le fichier a ete sauvegarde avec succes",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.Lookup("The file was saved", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore: 0.5, MaxResults: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, pensieve.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
	assert.Less(t, matches[0].Score, 1.0)
}

func TestSQLiteTM_Delete(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "Hello", Target: "Bonjour",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e2", Source: "Goodbye", Target: "Au revoir",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	assert.Equal(t, 2, tm.Count())

	err = tm.Delete("e1")
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, matches)

	matches, err = tm.Lookup("Goodbye", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore: 1.0, MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.Target)

	err = tm.Delete("non-existent")
	assert.Error(t, err)
}

func TestSQLiteTM_EmptyIDError(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	err = tm.Add(pensieve.TMEntry{
		Source: "Hello", Target: "Bonjour",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	})
	assert.Error(t, err)
}

func TestSQLiteTM_UpdateExisting(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "Hello", Target: "Bonjour",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	// Update with same ID.
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "Hello", Target: "Salut",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	assert.Equal(t, 1, tm.Count())
	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Salut", matches[0].Entry.Target)
}

func TestSQLiteTM_LocaleFiltering(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "Hello", Target: "Bonjour",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e2", Source: "Hello", Target: "Hallo",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleGerman,
	}))

	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.Target)

	matches, err = tm.Lookup("Hello", model.LocaleEnglish, model.LocaleGerman, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Hallo", matches[0].Entry.Target)
}

func TestSQLiteTM_Entries(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e1", Source: "Hello", Target: "Bonjour",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
		Properties: map[string]string{"domain": "general"},
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID: "e2", Source: "Goodbye", Target: "Au revoir",
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	entries := tm.Entries()
	assert.Len(t, entries, 2)
}

func TestSQLiteTM_InterfaceCompliance(t *testing.T) {
	tm, err := pensieve.NewSQLiteTM(":memory:")
	require.NoError(t, err)

	// Verify it satisfies the TranslationMemory interface.
	var _ pensieve.TranslationMemory = tm

	err = tm.Close()
	assert.NoError(t, err)
}
