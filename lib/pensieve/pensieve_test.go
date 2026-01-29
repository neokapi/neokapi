package pensieve_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/lib/pensieve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryTM_AddAndLookup(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

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

	err := tm.Add(entry)
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.Target)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, pensieve.MatchExact, matches[0].MatchType)
}

func TestInMemoryTM_ExactMatch(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "Save",
		Target:       "Sauvegarder",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e2",
		Source:       "Cancel",
		Target:       "Annuler",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	matches, err := tm.Lookup("Save", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Sauvegarder", matches[0].Entry.Target)
	assert.Equal(t, pensieve.MatchExact, matches[0].MatchType)
}

func TestInMemoryTM_FuzzyMatch(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "The file was saved successfully",
		Target:       "Le fichier a ete sauvegarde avec succes",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Search with slightly different text.
	matches, err := tm.Lookup("The file was saved", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   0.5,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, pensieve.MatchFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.5)
	assert.Less(t, matches[0].Score, 1.0)
}

func TestInMemoryTM_Delete(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "Hello",
		Target:       "Bonjour",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e2",
		Source:       "Goodbye",
		Target:       "Au revoir",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	assert.Equal(t, 2, tm.Count())

	err := tm.Delete("e1")
	require.NoError(t, err)
	assert.Equal(t, 1, tm.Count())

	// Should not find deleted entry.
	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, matches)

	// Should still find remaining entry.
	matches, err = tm.Lookup("Goodbye", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.Target)

	// Deleting non-existent entry returns error.
	err = tm.Delete("non-existent")
	assert.Error(t, err)
}

func TestInMemoryTM_EmptyIDError(t *testing.T) {
	tm := pensieve.NewInMemoryTM()
	err := tm.Add(pensieve.TMEntry{
		Source:       "Hello",
		Target:       "Bonjour",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	assert.Error(t, err)
}

func TestInMemoryTM_UpdateExisting(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "Hello",
		Target:       "Bonjour",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	// Update with same ID.
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "Hello",
		Target:       "Salut",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	assert.Equal(t, 1, tm.Count())
	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Salut", matches[0].Entry.Target)
}

func TestInMemoryTM_LocaleFiltering(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "Hello",
		Target:       "Bonjour",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e2",
		Source:       "Hello",
		Target:       "Hallo",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleGerman,
	}))

	// Search for French.
	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour", matches[0].Entry.Target)

	// Search for German.
	matches, err = tm.Lookup("Hello", model.LocaleEnglish, model.LocaleGerman, pensieve.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Hallo", matches[0].Entry.Target)
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
			dist := pensieve.LevenshteinDistance(tt.a, tt.b)
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
			ratio := pensieve.LevenshteinRatio(tt.a, tt.b)
			assert.GreaterOrEqual(t, ratio, tt.minRatio, "ratio %f below minimum %f", ratio, tt.minRatio)
			assert.LessOrEqual(t, ratio, tt.maxRatio, "ratio %f above maximum %f", ratio, tt.maxRatio)
		})
	}
}

func TestTMLeverageTool(t *testing.T) {
	tm := pensieve.NewInMemoryTM()
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "Hello World",
		Target:       "Bonjour le monde",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	leverageTool := pensieve.NewTMLeverageTool(tm, pensieve.TMLeverageConfig{
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
	assert.Equal(t, "tm:pensieve", altTrans.Origin)
	assert.Equal(t, 1.0, altTrans.Score)
	assert.Equal(t, "exact", altTrans.MatchType)
}

func TestTMLeverageTool_FuzzyMatch(t *testing.T) {
	tm := pensieve.NewInMemoryTM()
	require.NoError(t, tm.Add(pensieve.TMEntry{
		ID:           "e1",
		Source:       "The document was saved successfully",
		Target:       "Le document a ete sauvegarde avec succes",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	}))

	leverageTool := pensieve.NewTMLeverageTool(tm, pensieve.TMLeverageConfig{
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
	assert.Equal(t, "fuzzy", altTrans.MatchType)
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
	tm := pensieve.NewInMemoryTM()
	count, err := pensieve.ImportTMX(tm, strings.NewReader(tmxContent), model.LocaleEnglish, model.LocaleFrench)
	require.NoError(t, err)
	assert.Equal(t, 2, count) // tu3 should be skipped (no French)
	assert.Equal(t, 2, tm.Count())

	// Verify entries were imported correctly.
	matches, err := tm.Lookup("Hello World", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bonjour le monde", matches[0].Entry.Target)

	matches, err = tm.Lookup("Goodbye", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Au revoir", matches[0].Entry.Target)

	// Export.
	var buf bytes.Buffer
	err = pensieve.ExportTMX(tm, &buf, model.LocaleEnglish, model.LocaleFrench)
	require.NoError(t, err)

	exported := buf.String()
	assert.Contains(t, exported, "<?xml version=")
	assert.Contains(t, exported, "<tmx")
	assert.Contains(t, exported, "Hello World")
	assert.Contains(t, exported, "Bonjour le monde")
	assert.Contains(t, exported, "Goodbye")
	assert.Contains(t, exported, "Au revoir")

	// Roundtrip: re-import the exported TMX.
	tm2 := pensieve.NewInMemoryTM()
	count2, err := pensieve.ImportTMX(tm2, strings.NewReader(exported), model.LocaleEnglish, model.LocaleFrench)
	require.NoError(t, err)
	assert.Equal(t, 2, count2)

	matches2, err := tm2.Lookup("Hello World", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   1.0,
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, matches2, 1)
	assert.Equal(t, "Bonjour le monde", matches2[0].Entry.Target)
}

func TestMatchTypeString(t *testing.T) {
	assert.Equal(t, "exact", pensieve.MatchExact.String())
	assert.Equal(t, "fuzzy", pensieve.MatchFuzzy.String())
	assert.Equal(t, "unknown", pensieve.MatchType(99).String())
}

func TestInMemoryTM_MaxResults(t *testing.T) {
	tm := pensieve.NewInMemoryTM()

	// Add many similar entries.
	for i := 0; i < 20; i++ {
		require.NoError(t, tm.Add(pensieve.TMEntry{
			ID:           strings.Replace("e-NN", "NN", strings.Repeat("x", i+1), 1),
			Source:       "Hello",
			Target:       "Bonjour",
			SourceLocale: model.LocaleEnglish,
			TargetLocale: model.LocaleFrench,
		}))
	}

	matches, err := tm.Lookup("Hello", model.LocaleEnglish, model.LocaleFrench, pensieve.LookupOptions{
		MinScore:   0.5,
		MaxResults: 5,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(matches), 5)
}

func TestInMemoryTM_Close(t *testing.T) {
	tm := pensieve.NewInMemoryTM()
	err := tm.Close()
	assert.NoError(t, err)
}
