package termbase_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sqliteSoftwareConcepts() []termbase.Concept {
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
			Definition: "A storage location for code",
			Terms: []termbase.Term{
				{Text: "Repository", Locale: model.LocaleEnglish, Status: model.TermApproved},
				{Text: "Depot", Locale: model.LocaleFrench, Status: model.TermApproved},
			},
		},
	}
}

func sqlitePopulateTB(t *testing.T, tb termbase.TermBase) {
	t.Helper()
	for _, c := range sqliteSoftwareConcepts() {
		require.NoError(t, tb.AddConcept(c))
	}
}

func TestSQLiteTermBase_AddAndGet(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	sqlitePopulateTB(t, tb)
	assert.Equal(t, 3, tb.Count())

	c, ok := tb.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "software", c.Domain)
	assert.Len(t, c.Terms, 3)
}

func TestSQLiteTermBase_Delete(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	sqlitePopulateTB(t, tb)

	err = tb.DeleteConcept("c2")
	require.NoError(t, err)
	assert.Equal(t, 2, tb.Count())

	_, ok := tb.GetConcept("c2")
	assert.False(t, ok)
}

func TestSQLiteTermBase_Update(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	sqlitePopulateTB(t, tb)

	err = tb.AddConcept(termbase.Concept{
		ID:         "c1",
		Domain:     "software-ui",
		Definition: "Updated",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, tb.Count())

	c, ok := tb.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "software-ui", c.Domain)
	assert.Len(t, c.Terms, 1)
}

func TestSQLiteTermBase_Lookup(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	sqlitePopulateTB(t, tb)

	matches := tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "Save", matches[0].Term.Text)
	assert.Equal(t, 1.0, matches[0].Score)
}

func TestSQLiteTermBase_LookupAll(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	sqlitePopulateTB(t, tb)

	text := "Click Save or Cancel"
	matches := tb.LookupAll(text, termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.GreaterOrEqual(t, len(matches), 2)
}

func TestSQLiteTermBase_Search(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	sqlitePopulateTB(t, tb)

	results, total := tb.Search("save", "", "", 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)
}

// TestSQLiteTermBase_NoDeadSearchTable guards against reintroducing the
// previously-dead contentless tb_search FTS5 table (audit finding #39). The
// portable FTS path is the trigram index, which must exist and back Search;
// the never-populated/never-queried tb_search table must not.
func TestSQLiteTermBase_NoDeadSearchTable(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	tableExists := func(name string) bool {
		var n int
		err := tb.DB().QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name,
		).Scan(&n)
		require.NoError(t, err)
		return n > 0
	}

	assert.False(t, tableExists("tb_search"),
		"dead tb_search FTS5 table must not be created")
	assert.True(t, tableExists("tb_terms_trigram"),
		"trigram FTS5 index backing Search must exist")

	// The trigram path must still serve ranked search after the removal.
	sqlitePopulateTB(t, tb)
	results, total := tb.Search("repo", "", "", 0, 100)
	assert.Equal(t, 1, total)
	require.Len(t, results, 1)
	assert.Equal(t, "c3", results[0].ID)
}

func TestSQLiteTermBase_InterfaceCompliance(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)

	var _ termbase.TermBase = tb
	var _ termbase.TBStore = tb
	require.NoError(t, tb.Close())
}

func TestSQLiteTermBase_AddConceptWithStream(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	mainConcept := termbase.Concept{
		ID:     "c-main",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "File", Locale: "en-US", Status: model.TermApproved},
			{Text: "Datei", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConceptWithStream(mainConcept, "main"))

	featureConcept := termbase.Concept{
		ID:     "c-feat",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Document", Locale: "en-US", Status: model.TermApproved},
			{Text: "Dokument", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConceptWithStream(featureConcept, "feature/rebrand"))

	wsConcept := termbase.Concept{
		ID:     "c-ws",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Save", Locale: "en-US", Status: model.TermApproved},
			{Text: "Speichern", Locale: "de-DE", Status: model.TermApproved},
		},
	}
	require.NoError(t, tb.AddConcept(wsConcept))

	concepts, total := tb.SearchForStream("", "", "",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	assert.Equal(t, 3, total)
	assert.Len(t, concepts, 3)
	assert.Equal(t, "c-feat", concepts[0].ID)

	concepts, total = tb.SearchForStream("", "", "", "", nil, 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, concepts, 1)
	assert.Equal(t, "c-ws", concepts[0].ID)

	concepts, total = tb.SearchForStream("save", "", "",
		"feature/rebrand", []string{"main", ""}, 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, concepts, 1)
	assert.Equal(t, "c-ws", concepts[0].ID)
}

func TestSQLiteTermBase_FuzzyLookup(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(termbase.Concept{ID: "c1", Domain: "IT",
		Terms: []termbase.Term{{Text: "computer", Locale: "en-US", Status: "approved"}}}))
	require.NoError(t, tb.AddConcept(termbase.Concept{ID: "c2", Domain: "IT",
		Terms: []termbase.Term{{Text: "computing", Locale: "en-US", Status: "approved"}}}))
	require.NoError(t, tb.AddConcept(termbase.Concept{ID: "c3", Domain: "IT",
		Terms: []termbase.Term{{Text: "unrelated", Locale: "en-US", Status: "approved"}}}))

	matches := tb.Lookup("computers", termbase.LookupOptions{
		SourceLocale: "en-US",
		MinScore:     0.7,
	})
	assert.GreaterOrEqual(t, len(matches), 1)
	for _, m := range matches {
		assert.GreaterOrEqual(t, m.Score, 0.7)
		assert.NotEqual(t, "c3", m.Concept.ID)
	}
}

func TestSQLiteTermBase_ScaleTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	for i := range 500 {
		require.NoError(t, tb.AddConcept(termbase.Concept{
			ID:     fmt.Sprintf("c%d", i),
			Domain: "domain",
			Terms: []termbase.Term{
				{Text: fmt.Sprintf("terminology entry number %d", i), Locale: "en-US", Status: "approved"},
				{Text: fmt.Sprintf("entrée terminologique numéro %d", i), Locale: "fr-FR", Status: "approved"},
			},
		}))
	}

	assert.Equal(t, 500, tb.Count())

	matches := tb.Lookup("terminology entry number 42", termbase.LookupOptions{
		SourceLocale: "en-US",
		MinScore:     0.7,
	})
	assert.GreaterOrEqual(t, len(matches), 1)

	concepts, total := tb.Search("terminology", "", "", 0, 10)
	assert.GreaterOrEqual(t, total, 1)
	assert.GreaterOrEqual(t, len(concepts), 1)
}

func TestSQLiteTermBase_TermSourceField(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	// Add a terminology concept (default source).
	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "term-1",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	// Add a brand vocabulary concept.
	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "brand-1",
		Domain: "brand",
		Source: termbase.TermSourceBrandVocabulary,
		Terms: []termbase.Term{
			{Text: "Acme Cloud", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	// Verify source is persisted.
	c, ok := tb.GetConcept("term-1")
	assert.True(t, ok)
	assert.Equal(t, termbase.TermSourceTerminology, c.Source)

	c, ok = tb.GetConcept("brand-1")
	assert.True(t, ok)
	assert.Equal(t, termbase.TermSourceBrandVocabulary, c.Source)
}

func TestSQLiteTermBase_SourceFilterLookup(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "term-1",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Deploy", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "brand-1",
		Domain: "brand",
		Source: termbase.TermSourceBrandVocabulary,
		Terms: []termbase.Term{
			{Text: "Deploy", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	// No filter: both match.
	matches := tb.Lookup("Deploy", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	assert.Len(t, matches, 2)

	// Filter brand_vocabulary only.
	matches = tb.Lookup("Deploy", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		SourceFilter: []termbase.TermSource{termbase.TermSourceBrandVocabulary},
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "brand-1", matches[0].Concept.ID)

	// Filter terminology only.
	matches = tb.Lookup("Deploy", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		SourceFilter: []termbase.TermSource{termbase.TermSourceTerminology},
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "term-1", matches[0].Concept.ID)
}

func TestSQLiteTermBase_SourceFilterLookupAll(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "term-1",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "brand-1",
		Domain: "brand",
		Source: termbase.TermSourceBrandVocabulary,
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))

	matches := tb.LookupAll("Click Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
		SourceFilter: []termbase.TermSource{termbase.TermSourceBrandVocabulary},
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "brand-1", matches[0].Concept.ID)
}

func TestSQLiteTermBase_CompetitorTermField(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(termbase.Concept{
		ID:     "brand-1",
		Domain: "brand",
		Source: termbase.TermSourceBrandVocabulary,
		Terms: []termbase.Term{
			{Text: "Acme Cloud", Locale: model.LocaleEnglish, Status: model.TermPreferred, CompetitorTerm: false},
			{Text: "CompetitorX Cloud", Locale: model.LocaleEnglish, Status: model.TermForbidden, CompetitorTerm: true},
		},
	}))

	c, ok := tb.GetConcept("brand-1")
	assert.True(t, ok)
	require.Len(t, c.Terms, 2)
	assert.False(t, c.Terms[0].CompetitorTerm)
	assert.True(t, c.Terms[1].CompetitorTerm)
}

func TestConceptRelation_JSON(t *testing.T) {
	rel := termbase.ConceptRelation{
		SourceID:     "concept-1",
		TargetID:     "concept-2",
		RelationType: "related",
	}
	data, err := json.Marshal(rel)
	require.NoError(t, err)

	var decoded termbase.ConceptRelation
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, rel, decoded)
}

func TestTermDesignation_WithValidity(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	later := now.Add(24 * time.Hour)

	td := termbase.TermDesignation{
		Term: termbase.Term{
			Text:   "Acme Cloud",
			Locale: model.LocaleEnglish,
			Status: model.TermPreferred,
		},
		Validity: &graph.Validity{
			ValidFrom: &now,
			ValidTo:   &later,
			Tags:      map[string]string{"region": "US"},
		},
	}

	data, err := json.Marshal(td)
	require.NoError(t, err)

	var decoded termbase.TermDesignation
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, td.Term.Text, decoded.Term.Text)
	assert.NotNil(t, decoded.Validity)
	assert.True(t, decoded.Validity.ValidFrom.Equal(now))
	assert.True(t, decoded.Validity.ValidTo.Equal(later))
	assert.Equal(t, "US", decoded.Validity.Tags["region"])
}
