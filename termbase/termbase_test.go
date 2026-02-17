package termbase_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func softwareConcepts() []termbase.Concept {
	return []termbase.Concept{
		{
			ID:         "c1",
			Domain:     "software",
			Definition: "To store data persistently",
			Terms: []termbase.Term{
				{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Sauvegarder", Locale: model.LocaleFrench, Status: model.TermPreferred},
				{Text: "Speichern", Locale: model.LocaleGerman, Status: model.TermPreferred},
			},
		},
		{
			ID:         "c2",
			Domain:     "software",
			Definition: "To abort the current operation",
			Terms: []termbase.Term{
				{Text: "Cancel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Annuler", Locale: model.LocaleFrench, Status: model.TermPreferred},
				{Text: "Abbrechen", Locale: model.LocaleGerman, Status: model.TermPreferred},
			},
		},
		{
			ID:         "c3",
			Domain:     "software",
			Definition: "A repository for source code",
			Terms: []termbase.Term{
				{Text: "Repository", Locale: model.LocaleEnglish, Status: model.TermPreferred},
				{Text: "Repo", Locale: model.LocaleEnglish, Status: model.TermAdmitted},
				{Text: "Depot", Locale: model.LocaleFrench, Status: model.TermPreferred, Note: "Use depot, not repository"},
				{Text: "Repository", Locale: model.LocaleGerman, Status: model.TermPreferred},
			},
		},
	}
}

func populateTB(t *testing.T, tb termbase.TermBase) {
	t.Helper()
	for _, c := range softwareConcepts() {
		require.NoError(t, tb.AddConcept(c))
	}
}

// --- In-memory tests ---

func TestInMemoryTermBase_AddAndGet(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	assert.Equal(t, 3, tb.Count())

	c, ok := tb.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "software", c.Domain)
	assert.Equal(t, "To store data persistently", c.Definition)
	assert.Len(t, c.Terms, 3)
}

func TestInMemoryTermBase_Delete(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	err := tb.DeleteConcept("c2")
	require.NoError(t, err)
	assert.Equal(t, 2, tb.Count())

	_, ok := tb.GetConcept("c2")
	assert.False(t, ok)

	err = tb.DeleteConcept("nonexistent")
	assert.Error(t, err)
}

func TestInMemoryTermBase_Update(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	// Update concept c1.
	err := tb.AddConcept(termbase.Concept{
		ID:         "c1",
		Domain:     "software-ui",
		Definition: "Updated definition",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, tb.Count()) // count unchanged

	c, ok := tb.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "software-ui", c.Domain)
	assert.Equal(t, "Updated definition", c.Definition)
	assert.Len(t, c.Terms, 1)
}

func TestInMemoryTermBase_LookupExact(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	matches := tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "Save", matches[0].Term.Text)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, model.MatchStrategyExact, matches[0].MatchType)
	assert.Equal(t, "c1", matches[0].Concept.ID)
}

func TestInMemoryTermBase_LookupCaseInsensitive(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	matches := tb.Lookup("save", termbase.LookupOptions{
		SourceLocale:  model.LocaleEnglish,
		CaseSensitive: false,
	})
	require.NotEmpty(t, matches)
	assert.Equal(t, "Save", matches[0].Term.Text)
}

func TestInMemoryTermBase_LookupCaseSensitive(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	// Should not match with wrong case.
	matches := tb.Lookup("save", termbase.LookupOptions{
		SourceLocale:  model.LocaleEnglish,
		CaseSensitive: true,
		MatchModes:    []model.MatchStrategy{model.MatchStrategyExact},
	})
	assert.Empty(t, matches)

	// Should match with correct case.
	matches = tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale:  model.LocaleEnglish,
		CaseSensitive: true,
		MatchModes:    []model.MatchStrategy{model.MatchStrategyExact},
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "Save", matches[0].Term.Text)
}

func TestInMemoryTermBase_LookupFuzzy(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	matches := tb.Lookup("Repositor", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		MinScore:     0.7,
		MatchModes:   []model.MatchStrategy{model.MatchStrategyFuzzy},
	})
	require.NotEmpty(t, matches)
	assert.Equal(t, model.MatchStrategyFuzzy, matches[0].MatchType)
	assert.Greater(t, matches[0].Score, 0.7)
}

func TestInMemoryTermBase_LookupAll(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	text := "Click Save to save changes or Cancel to abort"
	matches := tb.LookupAll(text, termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})

	// Should find "Save" (twice) and "Cancel".
	require.GreaterOrEqual(t, len(matches), 3)

	// Verify positions are sorted.
	for i := 1; i < len(matches); i++ {
		assert.GreaterOrEqual(t, matches[i].Position.Start, matches[i-1].Position.Start)
	}
}

func TestInMemoryTermBase_LookupWithDomainFilter(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)
	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "med-1",
		Domain: "medical",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	// Without domain filter, find both.
	matches := tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	assert.Len(t, matches, 2)

	// With domain filter, find only software.
	matches = tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		Domains:      []string{"software"},
	})
	assert.Len(t, matches, 1)
	assert.Equal(t, "c1", matches[0].Concept.ID)
}

func TestInMemoryTermBase_LookupWithStatusFilter(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	// Should find both "Repository" (preferred) and "Repo" (admitted).
	matches := tb.Lookup("Repository", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	assert.Len(t, matches, 1)

	// Filter to preferred only should still find "Repository".
	matches = tb.Lookup("Repository", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		StatusFilter: []model.TermStatus{model.TermPreferred},
	})
	assert.Len(t, matches, 1)

	// Filter to admitted only should not find "Repository".
	matches = tb.Lookup("Repository", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		StatusFilter: []model.TermStatus{model.TermAdmitted},
	})
	assert.Empty(t, matches)
}

func TestInMemoryTermBase_Search(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	// Search by term text.
	results, total := tb.Search("save", "", "", 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)

	// Search by definition.
	results, total = tb.Search("abort", "", "", 0, 100)
	assert.Equal(t, 1, total)
	assert.Equal(t, "c2", results[0].ID)

	// Search by domain.
	_, total = tb.Search("software", "", "", 0, 100)
	assert.Equal(t, 3, total)

	// Filter by locale.
	_, total = tb.Search("", "de", "", 0, 100)
	assert.Equal(t, 3, total)

	// Pagination.
	results, total = tb.Search("", "", "", 0, 2)
	assert.Equal(t, 3, total)
	assert.Len(t, results, 2)
}

func TestInMemoryTermBase_Concepts(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	concepts := tb.Concepts()
	assert.Len(t, concepts, 3)
}

func TestInMemoryTermBase_ConceptHelpers(t *testing.T) {
	concept := softwareConcepts()[2] // Repository concept

	// SourceTerm
	en := concept.SourceTerm(model.LocaleEnglish)
	require.NotNil(t, en)
	assert.Equal(t, "Repository", en.Text)

	// TargetTerms
	enTerms := concept.TargetTerms(model.LocaleEnglish)
	assert.Len(t, enTerms, 2) // Repository + Repo

	// PreferredTerm
	pref := concept.PreferredTerm(model.LocaleFrench)
	require.NotNil(t, pref)
	assert.Equal(t, "Depot", pref.Text)

	// No preferred term for unknown locale.
	missing := concept.PreferredTerm("ja")
	assert.Nil(t, missing)
}

func TestInMemoryTermBase_InterfaceCompliance(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	var _ termbase.TermBase = tb
	assert.NoError(t, tb.Close())
}

// --- CSV import/export tests ---

func TestCSVImportExport(t *testing.T) {
	csvContent := `Save,Sauvegarder,software,To store data,preferred
Cancel,Annuler,software,To abort,preferred
Repository,Depot,software,Code repository,approved
`

	tb := termbase.NewInMemoryTermBase()
	count, err := termbase.ImportCSV(tb, strings.NewReader(csvContent), termbase.CSVImportOptions{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		IDPrefix:     "csv",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Equal(t, 3, tb.Count())

	// Verify imported data.
	matches := tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "software", matches[0].Concept.Domain)

	// Export.
	var buf bytes.Buffer
	err = termbase.ExportCSV(tb, &buf, model.LocaleEnglish, model.LocaleFrench, true)
	require.NoError(t, err)

	exported := buf.String()
	assert.Contains(t, exported, "Save")
	assert.Contains(t, exported, "Sauvegarder")
	assert.Contains(t, exported, "source,target")
}

func TestCSVImportWithHeader(t *testing.T) {
	csvContent := `source,target,domain
Save,Sauvegarder,software
Cancel,Annuler,software
`

	tb := termbase.NewInMemoryTermBase()
	count, err := termbase.ImportCSV(tb, strings.NewReader(csvContent), termbase.CSVImportOptions{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		HasHeader:    true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// --- JSON import/export tests ---

func TestJSONImportExport(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	// Export.
	var buf bytes.Buffer
	err := termbase.ExportJSON(tb, &buf, "test-termbase")
	require.NoError(t, err)

	exported := buf.String()
	assert.Contains(t, exported, "test-termbase")
	assert.Contains(t, exported, "Repository")

	// Re-import.
	tb2 := termbase.NewInMemoryTermBase()
	count, err := termbase.ImportJSON(tb2, strings.NewReader(exported))
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	c, ok := tb2.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "software", c.Domain)
}

// --- Pipeline tool tests ---

func processBlock(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, block *model.Block) *model.Block {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := tl.Process(context.Background(), in, out)
	require.NoError(t, err)

	result := <-out
	return result.Resource.(*model.Block)
}

func TestTermLookupTool_Basic(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("b1", "Click Save to save your work")
	result := processBlock(t, tl, block)

	// Should find "Save" and "save" (case-insensitive).
	count := result.Properties["term-count"]
	require.NotEmpty(t, count, "should have term annotations")

	// Verify at least one term annotation exists.
	var found bool
	for key, ann := range result.Annotations {
		if strings.HasPrefix(key, "term:") {
			ta, ok := ann.(*model.TermAnnotation)
			require.True(t, ok, "annotation should be TermAnnotation")
			assert.Equal(t, "c1", ta.ConceptID)
			assert.Equal(t, model.MatchStrategyExact, ta.MatchType)
			assert.NotEmpty(t, ta.TargetTerms)
			assert.Equal(t, "Sauvegarder", ta.TargetTerms[0].Text)
			found = true
			break
		}
	}
	assert.True(t, found, "should have at least one term annotation")
}

func TestTermLookupTool_MultipleTerms(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleGerman,
	})

	block := model.NewBlock("b1", "Click Save or Cancel")
	result := processBlock(t, tl, block)

	count := result.Properties["term-count"]
	assert.Equal(t, "2", count, "should find Save and Cancel")
}

func TestTermLookupTool_NonTranslatable(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
	})

	block := model.NewBlock("b1", "Click Save")
	block.Translatable = false
	result := processBlock(t, tl, block)

	// Should pass through without annotations.
	assert.Empty(t, result.Annotations)
}

func TestTermLookupTool_DomainFilter(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)
	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "med-1",
		Domain: "medical",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	// With domain filter, only software terms.
	tl := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
		SourceLocale: model.LocaleEnglish,
		Domains:      []string{"software"},
	})

	block := model.NewBlock("b1", "Click Save")
	result := processBlock(t, tl, block)

	count := result.Properties["term-count"]
	assert.Equal(t, "1", count, "should find only software 'Save'")
}

func TestTermEnforceTool_Pass(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("b1", "Click Save")
	block.SetTargetText(model.LocaleFrench, "Cliquez sur Sauvegarder")
	result := processBlock(t, te, block)

	assert.Equal(t, "true", result.Properties["term-enforce-passed"])
	assert.Equal(t, "0", result.Properties["term-enforce-violations"])
}

func TestTermEnforceTool_Violation(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("b1", "Click Save")
	block.SetTargetText(model.LocaleFrench, "Cliquez sur Enregistrer") // wrong term
	result := processBlock(t, te, block)

	assert.Equal(t, "false", result.Properties["term-enforce-passed"])
	assert.Equal(t, "1", result.Properties["term-enforce-violations"])
	assert.Contains(t, result.Properties["term-enforce-errors"], "Save")
	assert.Contains(t, result.Properties["term-enforce-errors"], "Sauvegarder")

	// Should have violation annotation.
	var hasViolation bool
	for key := range result.Annotations {
		if strings.HasPrefix(key, "term-violation:") {
			hasViolation = true
			break
		}
	}
	assert.True(t, hasViolation, "should have term-violation annotation")
}

func TestTermEnforceTool_NoTarget(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("b1", "Click Save")
	// No target set — should pass through without enforcement.
	result := processBlock(t, te, block)

	assert.Empty(t, result.Properties["term-enforce-passed"])
}

func TestTermEnforceTool_MultipleTerms(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	block := model.NewBlock("b1", "Click Save or Cancel")
	block.SetTargetText(model.LocaleFrench, "Cliquez sur Sauvegarder ou Annuler")
	result := processBlock(t, te, block)

	assert.Equal(t, "true", result.Properties["term-enforce-passed"])
	assert.Equal(t, "0", result.Properties["term-enforce-violations"])
}

func TestTermEnforceTool_PartialViolation(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	populateTB(t, tb)

	te := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
	})

	// "Save" correct, "Cancel" wrong.
	block := model.NewBlock("b1", "Click Save or Cancel")
	block.SetTargetText(model.LocaleFrench, "Cliquez sur Sauvegarder ou Supprimer")
	result := processBlock(t, te, block)

	assert.Equal(t, "false", result.Properties["term-enforce-passed"])
	assert.Equal(t, "1", result.Properties["term-enforce-violations"])
	assert.Contains(t, result.Properties["term-enforce-errors"], "Cancel")
}
