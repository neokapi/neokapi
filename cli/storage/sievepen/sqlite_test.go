package sievepen

import (
	"fmt"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	fw "github.com/gokapi/gokapi/core/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeEntry(id, sourceText, targetText string) fw.TMEntry {
	return fw.TMEntry{
		ID:           id,
		Source:       model.NewFragment(sourceText),
		Target:       model.NewFragment(targetText),
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
	}
}

func TestSQLiteTM_BasicOperations(t *testing.T) {
	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	// Add entries.
	require.NoError(t, tm.Add(makeEntry("e1", "Hello World", "Bonjour le monde")))
	require.NoError(t, tm.Add(makeEntry("e2", "Good morning", "Bonjour")))
	require.NoError(t, tm.Add(makeEntry("e3", "Good evening", "Bonsoir")))
	assert.Equal(t, 3, tm.Count())

	// Get by ID.
	entry, ok := tm.GetEntry("e1")
	assert.True(t, ok)
	assert.Equal(t, "e1", entry.ID)

	// Delete.
	require.NoError(t, tm.Delete("e1"))
	assert.Equal(t, 2, tm.Count())
}

func TestSQLiteTM_ExactLookup(t *testing.T) {
	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "Hello World", "Bonjour le monde")))

	// Exact match via LookupText.
	matches, err := tm.LookupText("Hello World", "en-US", "fr-FR", fw.LookupOptions{})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, fw.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_FuzzyLookup(t *testing.T) {
	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "The quick brown fox jumps over the lazy dog", "Le renard brun rapide")))
	require.NoError(t, tm.Add(makeEntry("e2", "The quick brown cat jumps over the lazy dog", "Le chat brun rapide")))
	require.NoError(t, tm.Add(makeEntry("e3", "Something completely different and unrelated", "Quelque chose")))

	// Fuzzy lookup should find e1 and e2 (similar to query) but not e3.
	matches, err := tm.LookupText("The quick brown fox jumps over the lazy cat", "en-US", "fr-FR", fw.LookupOptions{
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1, "should find at least one fuzzy match")

	for _, m := range matches {
		assert.GreaterOrEqual(t, m.Score, 0.7)
		assert.NotEqual(t, "e3", m.Entry.ID, "should not match unrelated entry")
	}
}

func TestSQLiteTM_FuzzyLookup_CJK(t *testing.T) {
	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "今日は良い天気です", "Today is good weather")))
	require.NoError(t, tm.Add(makeEntry("e2", "昨日は良い天気でした", "Yesterday was good weather")))
	require.NoError(t, tm.Add(makeEntry("e3", "全く関係のない文章", "Unrelated sentence")))

	// Fuzzy lookup for CJK content.
	matches, err := tm.LookupText("今日は良い天気でした", "en-US", "fr-FR", fw.LookupOptions{
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1, "should find CJK fuzzy matches")
}

func TestSQLiteTM_SearchFTS5(t *testing.T) {
	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "Hello World", "Bonjour le monde")))
	require.NoError(t, tm.Add(makeEntry("e2", "Hello there", "Bonjour là")))
	require.NoError(t, tm.Add(makeEntry("e3", "Goodbye World", "Au revoir le monde")))

	// Search should find entries containing "Hello".
	results, total := tm.SearchEntries("Hello", "", "", 0, 10)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)

	// Search with locale filter.
	results, total = tm.SearchEntries("monde", "en-US", "fr-FR", 0, 10)
	assert.GreaterOrEqual(t, total, 1)
}

func TestSQLiteTM_TrigramFallback(t *testing.T) {
	// Test that length-based fallback works when we can't use trigrams.
	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "short", "court")))
	require.NoError(t, tm.Add(makeEntry("e2", "a very long sentence that should not match a short query at all", "longue phrase")))

	// The length filter should exclude the long entry.
	candidates, err := tm.queryLengthFiltered("shirt", "en-US", "fr-FR")
	require.NoError(t, err)
	// Only "short" (5 chars) should be within range of "shirt" (5 chars).
	for _, e := range candidates {
		assert.NotEqual(t, "e2", e.ID, "long entry should be filtered by length")
	}
}

func TestSQLiteTM_ScaleTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	tm, err := NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	// Insert 1000 entries.
	for i := 0; i < 1000; i++ {
		text := fmt.Sprintf("This is test sentence number %d for translation memory", i)
		target := fmt.Sprintf("Ceci est la phrase test numéro %d", i)
		require.NoError(t, tm.Add(makeEntry(fmt.Sprintf("e%d", i), text, target)))
	}

	assert.Equal(t, 1000, tm.Count())

	// Fuzzy lookup should complete quickly.
	matches, err := tm.LookupText("This is test sentence number 42 for translation memory", "en-US", "fr-FR", fw.LookupOptions{
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1, "should find fuzzy matches in 1K entries")

	// Search should work.
	results, total := tm.SearchEntries("sentence", "", "", 0, 10)
	assert.Equal(t, 1000, total)
	assert.Len(t, results, 10)
}
