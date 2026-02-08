package sievepen_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryTM_AddAndLookup(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

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

	err := tm.Add(entry)
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.TargetText())
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestInMemoryTM_ExactMatch(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Save"),
		Target:       model.NewFragment("Sauvegarder"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e2",
		Source:       model.NewFragment("Cancel"),
		Target:       model.NewFragment("Annuler"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.LookupText("Save", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Sauvegarder", matches[0].Entry.TargetText())
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestInMemoryTM_FuzzyMatch(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("The file was saved successfully"),
		Target:       model.NewFragment("Le fichier a ete sauvegarde avec succes"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Search with slightly different text.
	matches, err := tm.LookupText("The file was saved", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   0.5,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, sievepen.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
	assert.Less(t, matches[0].Score, 1.0)
}

func TestInMemoryTM_Delete(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e2",
		Source:       model.NewFragment("Goodbye"),
		Target:       model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	assert.Equal(t, 2, tm.Count())

	err := tm.Delete("e1")
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	// Should not find deleted entry.
	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, matches)

	// Should still find remaining entry.
	matches, err = tm.LookupText("Goodbye", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.TargetText())

	// Deleting non-existent entry returns error.
	err = tm.Delete("non-existent")
	assert.Error(t, err)
}

func TestInMemoryTM_EmptyIDError(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	err := tm.Add(sievepen.TMEntry{
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	assert.Error(t, err)
}

func TestInMemoryTM_NilSourceError(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	err := tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	assert.Error(t, err)
}

func TestInMemoryTM_UpdateExisting(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Update with same ID.
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Salut"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	assert.Equal(t, 1, tm.Count())
	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Salut", matches[0].Entry.TargetText())
}

func TestInMemoryTM_LocaleFiltering(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e2",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Hallo"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleGerman,
	}))

	// Search for French.
	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.TargetText())

	// Search for German.
	matches, err = tm.LookupText("Hello", model.LocaleEnglish, model.LocaleGerman, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Hallo", matches[0].Entry.TargetText())
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"Saturday", "Sunday", 3},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			dist := sievepen.LevenshteinDistance(tt.a, tt.b)
			assert.Equal(t, tt.expected, dist)
		})
	}
}

func TestLevenshteinRatio(t *testing.T) {
	tests := []struct {
		a, b     string
		minRatio float64
		maxRatio float64
	}{
		{"", "", 1.0, 1.0},
		{"abc", "abc", 1.0, 1.0},
		{"abc", "abd", 0.6, 0.7},
		{"kitten", "sitting", 0.5, 0.6},
		{"abc", "xyz", 0.0, 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			ratio := sievepen.LevenshteinRatio(tt.a, tt.b)
			assert.GreaterOrEqual(t, ratio, tt.minRatio, "ratio %f below minimum %f", ratio, tt.minRatio)
			assert.LessOrEqual(t, ratio, tt.maxRatio, "ratio %f above maximum %f", ratio, tt.maxRatio)
		})
	}
}

func TestTMLeverageTool(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Hello World"),
		Target:       model.NewFragment("Bonjour le monde"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	leverageTool := sievepen.NewTMLeverageTool(tm, sievepen.TMLeverageConfig{
		MinScore:     0.7,
		MaxResults:   5,
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	assert.Equal(t, "tm-leverage", leverageTool.Name())
	assert.NotEmpty(t, leverageTool.Description())

	// Create parts with a translatable block.
	block := model.NewBlock("tu1", "Hello World")
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: block},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	ctx := context.Background()
	err := leverageTool.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	require.Len(t, results, 3)

	// Check that the block got the exact match applied.
	resultBlock := results[1].Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))

	// Check annotation.
	alt, ok := resultBlock.Annotations["alt-translation"]
	require.True(t, ok)
	altTrans, ok := alt.(*model.AltTranslation)
	require.True(t, ok)
	assert.Equal(t, "tm:sievepen", altTrans.Origin)
	assert.Equal(t, 1.0, altTrans.Score)
	// Plain text blocks match at generalized-exact tier (generalized key == plain key).
	assert.Equal(t, "generalized-exact", altTrans.MatchType)
}

func TestTMLeverageTool_FuzzyMatch(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("The document was saved successfully"),
		Target:       model.NewFragment("Le document a ete sauvegarde avec succes"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	leverageTool := sievepen.NewTMLeverageTool(tm, sievepen.TMLeverageConfig{
		MinScore:     0.5,
		MaxResults:   5,
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("tu1", "The document was saved")
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	ctx := context.Background()
	err := leverageTool.Process(ctx, in, out)
	close(out)
	require.NoError(t, err)

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	require.Len(t, results, 1)

	resultBlock := results[0].Resource.(*model.Block)
	// Fuzzy match should NOT set target text directly.
	assert.Empty(t, resultBlock.TargetText(model.LocaleFrench))

	// But should have annotation.
	alt, ok := resultBlock.Annotations["alt-translation"]
	require.True(t, ok)
	altTrans, ok := alt.(*model.AltTranslation)
	require.True(t, ok)
	// Plain text blocks match at generalized-fuzzy tier (generalized key == plain key).
	assert.Equal(t, "generalized-fuzzy", altTrans.MatchType)
	assert.Greater(t, altTrans.Score, 0.5)
	assert.Less(t, altTrans.Score, 1.0)
}

func TestTMXImportExport(t *testing.T) {
	tmxContent := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" segtype="sentence" adminlang="en" srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en">
        <seg>Hello World</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Bonjour le monde</seg>
      </tuv>
    </tu>
    <tu tuid="tu2">
      <tuv xml:lang="en">
        <seg>Goodbye</seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg>Au revoir</seg>
      </tuv>
    </tu>
    <tu tuid="tu3">
      <tuv xml:lang="en">
        <seg>Only English</seg>
      </tuv>
    </tu>
  </body>
</tmx>`

	// Import.
	tm := sievepen.NewInMemoryTM()
	count, err := sievepen.ImportTMX(tm, strings.NewReader(tmxContent), model.LocaleEnglish, model.LocaleFrench)
	require.NoError(t, err)
	assert.Equal(t, 2, count) // tu3 should be skipped (no French)
	assert.Equal(t, 2, tm.Count())

	// Verify entries were imported correctly.
	matches, err := tm.LookupText("Hello World", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour le monde", matches[0].Entry.TargetText())

	matches, err = tm.LookupText("Goodbye", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.TargetText())

	// Export.
	var buf bytes.Buffer
	err = sievepen.ExportTMX(tm, &buf, model.LocaleEnglish, model.LocaleFrench)
	require.NoError(t, err)

	exported := buf.String()
	assert.Contains(t, exported, "<?xml version=")
	assert.Contains(t, exported, "<tmx")
	assert.Contains(t, exported, "Hello World")
	assert.Contains(t, exported, "Bonjour le monde")
	assert.Contains(t, exported, "Goodbye")
	assert.Contains(t, exported, "Au revoir")

	// Roundtrip: re-import the exported TMX.
	tm2 := sievepen.NewInMemoryTM()
	count2, err := sievepen.ImportTMX(tm2, strings.NewReader(exported), model.LocaleEnglish, model.LocaleFrench)
	require.NoError(t, err)
	assert.Equal(t, 2, count2)

	matches2, err := tm2.LookupText("Hello World", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches2, 1)
	assert.Equal(t, "Bonjour le monde", matches2[0].Entry.TargetText())
}

func TestMatchTypeString(t *testing.T) {
	assert.Equal(t, "exact", sievepen.MatchExact.String())
	assert.Equal(t, "fuzzy", sievepen.MatchFuzzy.String())
	assert.Equal(t, "generalized-exact", sievepen.MatchGeneralizedExact.String())
	assert.Equal(t, "structural-exact", sievepen.MatchStructuralExact.String())
	assert.Equal(t, "generalized-fuzzy", sievepen.MatchGeneralizedFuzzy.String())
	assert.Equal(t, "structural-fuzzy", sievepen.MatchStructuralFuzzy.String())
}

func TestMatchType_IsExact(t *testing.T) {
	assert.True(t, sievepen.MatchExact.IsExact())
	assert.True(t, sievepen.MatchGeneralizedExact.IsExact())
	assert.True(t, sievepen.MatchStructuralExact.IsExact())
	assert.False(t, sievepen.MatchFuzzy.IsExact())
	assert.False(t, sievepen.MatchGeneralizedFuzzy.IsExact())
	assert.False(t, sievepen.MatchStructuralFuzzy.IsExact())
}

func TestInMemoryTM_MaxResults(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	// Add many similar entries.
	for i := 0; i < 20; i++ {
		require.NoError(t, tm.Add(sievepen.TMEntry{
			ID:           strings.Replace("e-NN", "NN", strings.Repeat("x", i+1), 1),
			Source:       model.NewFragment("Hello"),
			Target:       model.NewFragment("Bonjour"),
			SourceLocale: model.LocaleEnglish,
			TargetLocale: model.LocaleFrench,
		}))
	}

	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   0.5,
		MaxResults: 5,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(matches), 5)
}

func TestInMemoryTM_Close(t *testing.T) {
	tm := sievepen.NewInMemoryTM()
	err := tm.Close()
	assert.NoError(t, err)
}

func TestInMemoryTM_SearchEntries(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

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

	// Filter by source locale
	entries, total = tm.SearchEntries("", "en", "", 0, 100)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 3)

	// Pagination
	entries, total = tm.SearchEntries("", "", "", 0, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 2)

	entries, total = tm.SearchEntries("", "", "", 2, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, entries, 1)

	// Offset beyond total
	entries, total = tm.SearchEntries("", "", "", 10, 2)
	assert.Equal(t, 3, total)
	assert.Nil(t, entries)
}

func TestInMemoryTM_GetEntry(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

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

// --- Content-aware matching tests ---

func TestInMemoryTM_BlockLookup(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Click the Save button"),
		Target:       model.NewFragment("Cliquez sur le bouton Enregistrer"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Lookup using a Block (the primary content-aware path).
	block := model.NewBlock("tu1", "Click the Save button")
	matches, err := tm.Lookup(block, model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	// Plain text blocks match at generalized-exact tier (all keys identical).
	assert.True(t, matches[0].MatchType.IsExact())
}

func TestInMemoryTM_NilBlockLookup(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	matches, err := tm.Lookup(nil, model.LocaleEnglish, model.LocaleFrench, sievepen.DefaultLookupOptions())
	require.NoError(t, err)
	assert.Nil(t, matches)
}

func TestInMemoryTM_TieredMatchPriority(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	// Add entries that match at different tiers.
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "plain-match",
		Source:       model.NewFragment("Hello World"),
		Target:       model.NewFragment("Bonjour le monde"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.LookupText("Hello World", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   0.5,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	// When all tiers match the same text, the highest priority tier wins
	// (generalized-exact has priority 0, exact has priority 2 for plain text).
	// For plain fragments (no spans), all three tiers produce the same key,
	// so the first hit (generalized-exact) should appear first.
	assert.Equal(t, 1.0, matches[0].Score)
}

func TestInMemoryTM_PlainOnlyMatchMode(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID:           "e1",
		Source:       model.NewFragment("Hello"),
		Target:       model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Restrict to plain matching only.
	matches, err := tm.LookupText("Hello", model.LocaleEnglish, model.LocaleFrench, sievepen.LookupOptions{
		MinScore:   0.7,
		MaxResults: 10,
		MatchModes: []sievepen.MatchMode{sievepen.MatchModePlain},
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, sievepen.MatchExact, matches[0].MatchType)
}

func TestInMemoryTM_Entries(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e1", Source: model.NewFragment("Hello"), Target: model.NewFragment("Bonjour"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(sievepen.TMEntry{
		ID: "e2", Source: model.NewFragment("Goodbye"), Target: model.NewFragment("Au revoir"),
		SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench,
	}))

	entries := tm.Entries()
	assert.Len(t, entries, 2)
}

func TestInMemoryTM_InterfaceCompliance(t *testing.T) {
	tm := sievepen.NewInMemoryTM()

	// Verify it satisfies the TranslationMemory interface.
	var _ sievepen.TranslationMemory = tm
	var _ sievepen.EntryProvider = tm

	err := tm.Close()
	assert.NoError(t, err)
}

func TestTMEntry_HelperMethods(t *testing.T) {
	entry := sievepen.TMEntry{
		ID:     "e1",
		Source: model.NewFragment("Hello"),
		Target: model.NewFragment("Bonjour"),
	}

	assert.Equal(t, "Hello", entry.SourceText())
	assert.Equal(t, "Bonjour", entry.TargetText())

	// Nil fragments return empty strings.
	nilEntry := sievepen.TMEntry{ID: "e2"}
	assert.Equal(t, "", nilEntry.SourceText())
	assert.Equal(t, "", nilEntry.TargetText())
	assert.Equal(t, "", nilEntry.SourceStructural())
	assert.Equal(t, "", nilEntry.SourceGeneralized())
}
