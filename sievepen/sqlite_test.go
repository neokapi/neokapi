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

	require.NoError(t, tm.Close())
}

func TestSQLiteTM_FacetStats(t *testing.T) {
	t.Run("empty TM", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		data := tm.FacetStats()
		assert.Empty(t, data.LocalePairs)
		assert.Empty(t, data.Projects)
		assert.Empty(t, data.EntityTypes)
		assert.Equal(t, 0, data.HasCodes)
		assert.Equal(t, 0, data.NoCodes)
	})

	t.Run("locale pairs and projects", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		// Add entries across different locales and projects.
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
			SourceLocale: "en-US", TargetLocale: "fr-FR", ProjectID: "proj-a",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e2", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
			SourceLocale: "en-US", TargetLocale: "de-DE", ProjectID: "proj-a",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e3", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
			SourceLocale: "en-US", TargetLocale: "fr-FR", ProjectID: "proj-b",
		}))

		data := tm.FacetStats()

		// Two locale pairs: en-US→fr-FR (2 entries), en-US→de-DE (1 entry).
		assert.Len(t, data.LocalePairs, 2)

		// Two projects: proj-a (2 entries), proj-b (1 entry).
		assert.Len(t, data.Projects, 2)
		assert.Equal(t, "proj-a", data.Projects[0].ProjectID)
		assert.Equal(t, 2, data.Projects[0].Count)
		assert.Equal(t, "proj-b", data.Projects[1].ProjectID)
		assert.Equal(t, 1, data.Projects[1].Count)

		// No inline codes — all plain text.
		assert.Equal(t, 0, data.HasCodes)
		assert.Equal(t, 3, data.NoCodes)
	})

	t.Run("inline code detection", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		// Plain text entry.
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "plain", Source: model.NewFragment("plain text"), Target: model.NewFragment("texte brut"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))

		// Entry with inline spans (coded text).
		frag := model.NewFragment("Click ")
		frag.AppendSpan(&model.Span{SpanType: model.SpanOpening, ID: "1", Type: "bold"})
		frag.AppendText("here")
		frag.AppendSpan(&model.Span{SpanType: model.SpanClosing, ID: "1", Type: "bold"})
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "coded", Source: frag, Target: model.NewFragment("Cliquez ici"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))

		data := tm.FacetStats()
		assert.Equal(t, 1, data.HasCodes)
		assert.Equal(t, 1, data.NoCodes)
	})
}

func TestSQLiteTM_SearchEntriesGrouped(t *testing.T) {
	t.Run("empty TM", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		groups, total := tm.SearchEntriesGrouped("", "", 0, 10)
		assert.Nil(t, groups)
		assert.Equal(t, 0, total)
	})

	t.Run("groups by source text", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		// Same source "Hello" translated to two locales.
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e2", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
			SourceLocale: "en-US", TargetLocale: "de-DE",
		}))
		// Different source.
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e3", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))

		groups, total := tm.SearchEntriesGrouped("", "", 0, 10)
		assert.Equal(t, 2, total, "two distinct source texts")
		require.Len(t, groups, 2)

		// Find the "Hello" group — it should have 2 targets.
		var helloGroup *sievepen.TMEntryGroup
		for i := range groups {
			if groups[i].SourceText == "hello" || groups[i].SourceText == "Hello" {
				helloGroup = &groups[i]
				break
			}
		}
		require.NotNil(t, helloGroup, "should find the Hello group")
		assert.Len(t, helloGroup.Targets, 2)
	})

	t.Run("pagination on groups", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		// Create 5 distinct source texts.
		for i := 0; i < 5; i++ {
			require.NoError(t, tm.Add(sievepen.TMEntry{
				ID: fmt.Sprintf("e%d", i), Source: model.NewFragment(fmt.Sprintf("Sentence %d", i)),
				Target: model.NewFragment(fmt.Sprintf("Phrase %d", i)),
				SourceLocale: "en-US", TargetLocale: "fr-FR",
			}))
		}

		// Get first page of 2 groups.
		groups, total := tm.SearchEntriesGrouped("", "", 0, 2)
		assert.Equal(t, 5, total)
		assert.Len(t, groups, 2)

		// Get second page.
		groups2, total2 := tm.SearchEntriesGrouped("", "", 2, 2)
		assert.Equal(t, 5, total2)
		assert.Len(t, groups2, 2)

		// Third page should have 1 group.
		groups3, total3 := tm.SearchEntriesGrouped("", "", 4, 2)
		assert.Equal(t, 5, total3)
		assert.Len(t, groups3, 1)
	})

	t.Run("search query filters groups", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e1", Source: model.NewFragment("Hello world"), Target: model.NewFragment("Bonjour le monde"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e2", Source: model.NewFragment("Goodbye world"), Target: model.NewFragment("Au revoir le monde"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e3", Source: model.NewFragment("Something else"), Target: model.NewFragment("Autre chose"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))

		groups, total := tm.SearchEntriesGrouped("world", "", 0, 10)
		assert.Equal(t, 2, total)
		assert.Len(t, groups, 2)
	})

	t.Run("count reflects distinct sources not entries", func(t *testing.T) {
		tm, err := sievepen.NewSQLiteTM(":memory:")
		require.NoError(t, err)
		defer tm.Close()

		// Same source text, 3 different target locales = 1 group.
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
			SourceLocale: "en-US", TargetLocale: "fr-FR",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e2", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hallo"),
			SourceLocale: "en-US", TargetLocale: "de-DE",
		}))
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID: "e3", Source: model.NewFragment("Hello"), Target: model.NewFragment("Hola"),
			SourceLocale: "en-US", TargetLocale: "es-ES",
		}))

		groups, total := tm.SearchEntriesGrouped("", "", 0, 10)
		assert.Equal(t, 1, total, "one distinct source = one group")
		require.Len(t, groups, 1)
		assert.Len(t, groups[0].Targets, 3)
	})
}

func TestSQLiteTM_EntityValueFilter(t *testing.T) {
	tm, err := sievepen.NewSQLiteTM(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	mkWithEntity := func(id, src, tgt, entityValue, entityType string) sievepen.TMEntry {
		return sievepen.TMEntry{
			ID:           id,
			Source:       model.NewFragment(src),
			Target:       model.NewFragment(tgt),
			SourceLocale: "en-US",
			TargetLocale: "fr-FR",
			Entities: []sievepen.EntityMapping{
				{
					PlaceholderID: "e1",
					Type:          model.EntityType(entityType),
					SourceValue:   entityValue,
					TargetValue:   entityValue,
				},
			},
		}
	}

	require.NoError(t, tm.Add(mkWithEntity("e1", "John works here", "Jean travaille ici", "John", "entity:person")))
	require.NoError(t, tm.Add(mkWithEntity("e2", "Acme Corp released", "Acme Corp a publié", "Acme Corp", "entity:organization")))
	require.NoError(t, tm.Add(mkWithEntity("e3", "John met Acme Corp", "Jean a rencontré Acme Corp", "John", "entity:person")))
	require.NoError(t, tm.Add(makeEntry("e4", "Hello world", "Bonjour le monde")))

	t.Run("filter by entity value + type", func(t *testing.T) {
		entries, total := tm.SearchEntriesFiltered("", "en-US", "fr-FR",
			sievepen.SearchFilter{
				EntityValues: []sievepen.EntityValueFilter{
					{Value: "John", Type: "entity:person"},
				},
			}, 0, 10)
		assert.Equal(t, 2, total, "two entries have John as person")
		assert.Len(t, entries, 2)
		ids := map[string]bool{}
		for _, e := range entries {
			ids[e.ID] = true
		}
		assert.True(t, ids["e1"])
		assert.True(t, ids["e3"])
	})

	t.Run("filter by organization entity value", func(t *testing.T) {
		entries, total := tm.SearchEntriesFiltered("", "en-US", "fr-FR",
			sievepen.SearchFilter{
				EntityValues: []sievepen.EntityValueFilter{
					{Value: "Acme Corp", Type: "entity:organization"},
				},
			}, 0, 10)
		assert.Equal(t, 1, total)
		require.Len(t, entries, 1)
		assert.Equal(t, "e2", entries[0].ID)
	})

	t.Run("multiple entity values OR-ed", func(t *testing.T) {
		entries, total := tm.SearchEntriesFiltered("", "en-US", "fr-FR",
			sievepen.SearchFilter{
				EntityValues: []sievepen.EntityValueFilter{
					{Value: "John", Type: "entity:person"},
					{Value: "Acme Corp", Type: "entity:organization"},
				},
			}, 0, 10)
		assert.Equal(t, 3, total, "any match — three entries have John or Acme")
		assert.Len(t, entries, 3)
	})

	t.Run("value without matching type returns nothing", func(t *testing.T) {
		entries, total := tm.SearchEntriesFiltered("", "en-US", "fr-FR",
			sievepen.SearchFilter{
				EntityValues: []sievepen.EntityValueFilter{
					{Value: "John", Type: "entity:organization"},
				},
			}, 0, 10)
		assert.Equal(t, 0, total)
		assert.Empty(t, entries)
	})

	t.Run("entities round-trip correctly after scanEntries", func(t *testing.T) {
		entry, found := tm.GetEntry("e1")
		require.True(t, found)
		require.Len(t, entry.Entities, 1)
		assert.Equal(t, "John", entry.Entities[0].SourceValue)
		assert.Equal(t, model.EntityType("entity:person"), entry.Entities[0].Type)
		assert.Equal(t, "e1", entry.Entities[0].PlaceholderID)
	})
}
