package sievepen_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeEntry(id, sourceText, targetText string) sievepen.TMEntry {
	return sievepen.TMEntry{
		ID:           id,
		Source:       model.NewFragment(sourceText),
		Target:       model.NewFragment(targetText),
		SourceLocale: "en-US",
		TargetLocale: "fr-FR",
	}
}

func TestSQLiteTM_BasicOperations(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "Hello World", "Bonjour le monde")))
	require.NoError(t, tm.Add(makeEntry("e2", "Good morning", "Bonjour")))
	require.NoError(t, tm.Add(makeEntry("e3", "Good evening", "Bonsoir")))
	assert.Equal(t, 3, tm.Count())

	entry, ok := tm.GetEntry("e1")
	assert.True(t, ok)
	assert.Equal(t, "e1", entry.ID)

	require.NoError(t, tm.Delete("e1"))
	assert.Equal(t, 2, tm.Count())
}

func TestSQLiteTM_ExactLookup(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "Hello World", "Bonjour le monde")))

	matches, err := tm.LookupText("Hello World", "en-US", "fr-FR", sievepen.LookupOptions{})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestSQLiteTM_FuzzyLookup(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "The quick brown fox jumps over the lazy dog", "Le renard brun rapide")))
	require.NoError(t, tm.Add(makeEntry("e2", "The quick brown cat jumps over the lazy dog", "Le chat brun rapide")))
	require.NoError(t, tm.Add(makeEntry("e3", "Something completely different and unrelated", "Quelque chose")))

	matches, err := tm.LookupText("The quick brown fox jumps over the lazy cat", "en-US", "fr-FR", sievepen.LookupOptions{
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1)
	for _, m := range matches {
		assert.GreaterOrEqual(t, m.Score, 0.7)
		assert.NotEqual(t, "e3", m.Entry.ID)
	}
}

func TestSQLiteTM_FuzzyLookup_CJK(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "今日は良い天気です", "Today is good weather")))
	require.NoError(t, tm.Add(makeEntry("e2", "昨日は良い天気でした", "Yesterday was good weather")))
	require.NoError(t, tm.Add(makeEntry("e3", "全く関係のない文章", "Unrelated sentence")))

	matches, err := tm.LookupText("今日は良い天気でした", "en-US", "fr-FR", sievepen.LookupOptions{
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1)
}

func TestSQLiteTM_SearchFTS5(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "Hello World", "Bonjour le monde")))
	require.NoError(t, tm.Add(makeEntry("e2", "Hello there", "Bonjour là")))
	require.NoError(t, tm.Add(makeEntry("e3", "Goodbye World", "Au revoir le monde")))

	results, total := tm.SearchEntries("Hello", "", "", 0, 10)
	assert.Equal(t, 2, total)
	assert.Len(t, results, 2)

	results, total = tm.SearchEntries("monde", "en-US", "fr-FR", 0, 10)
	assert.GreaterOrEqual(t, total, 1)
	assert.NotEmpty(t, results)
}

func TestSQLiteTM_TrigramFallback(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(makeEntry("e1", "short", "court")))
	require.NoError(t, tm.Add(makeEntry("e2", "a very long sentence that should not match a short query at all", "longue phrase")))

	// Fuzzy lookup for "shirt" should find "short" but not the long entry.
	matches, err := tm.LookupText("shirt", "en-US", "fr-FR", sievepen.LookupOptions{MinScore: 0.5})
	require.NoError(t, err)
	for _, m := range matches {
		assert.NotEqual(t, "e2", m.Entry.ID, "long entry should be filtered")
	}
}

func TestSQLiteTM_ScaleTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	for i := range 1000 {
		text := fmt.Sprintf("This is test sentence number %d for translation memory", i)
		target := fmt.Sprintf("Ceci est la phrase test numéro %d", i)
		require.NoError(t, tm.Add(makeEntry(fmt.Sprintf("e%d", i), text, target)))
	}

	assert.Equal(t, 1000, tm.Count())

	matches, err := tm.LookupText("This is test sentence number 42 for translation memory", "en-US", "fr-FR", sievepen.LookupOptions{
		MinScore: 0.7,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(matches), 1)

	results, total := tm.SearchEntries("sentence", "", "", 0, 10)
	assert.Equal(t, 1000, total)
	assert.Len(t, results, 10)
}

func TestSQLiteTM_AddWithStream(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	mainEntry := sievepen.TMEntry{
		ID:           "main-1",
		Source:       model.NewFragment("Hello world"),
		Target:       model.NewFragment("Hallo Welt"),
		SourceLocale: "en-US",
		TargetLocale: "de-DE",
	}
	require.NoError(t, tm.AddWithStream(mainEntry, "main"))

	featureEntry := sievepen.TMEntry{
		ID:           "feat-1",
		Source:       model.NewFragment("Hello world"),
		Target:       model.NewFragment("Hallo Welt (Rebrand)"),
		SourceLocale: "en-US",
		TargetLocale: "de-DE",
	}
	require.NoError(t, tm.AddWithStream(featureEntry, "feature/rebrand"))

	workspaceEntry := sievepen.TMEntry{
		ID:           "ws-1",
		Source:       model.NewFragment("Goodbye"),
		Target:       model.NewFragment("Auf Wiedersehen"),
		SourceLocale: "en-US",
		TargetLocale: "de-DE",
	}
	require.NoError(t, tm.Add(workspaceEntry))

	entries, total := tm.SearchEntriesForStream("", "en-US", "de-DE",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 3)
	assert.Equal(t, "feat-1", entries[0].ID)

	entries, total = tm.SearchEntriesForStream("", "en-US", "de-DE", "", nil, 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, "ws-1", entries[0].ID)

	entries, total = tm.SearchEntriesForStream("goodbye", "en-US", "de-DE",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, entries, 1)
	assert.Equal(t, "ws-1", entries[0].ID)
}

func TestSQLiteTM_BlockLookup(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Click the Save button"),
		Target:       model.NewFragment("Cliquez sur le bouton Enregistrer"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	block := model.NewBlock("tu1", "Click the Save button")
	matches, err := tm.Lookup(block, model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
}

func TestSQLiteTM_FragmentRoundtrip(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

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

	entry, ok := tm.GetEntry("e1")
	require.True(t, ok)
	assert.Equal(t, "Click here to continue", entry.SourceText())
	assert.True(t, entry.Source.HasSpans())
	assert.Len(t, entry.Source.Spans, 2)
}

func TestSQLiteTM_TimestampPreservation(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	now := time.Now().Truncate(time.Second)
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

func TestSQLiteTM_InterfaceCompliance(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)

	var _ sievepen.TranslationMemory = tm
	var _ sievepen.EntryProvider = tm
	var _ sievepen.TMStore = tm

	assert.NoError(t, tm.Close())
}
