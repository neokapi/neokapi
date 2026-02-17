package termbase_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/termbase"

	sqltb "github.com/gokapi/gokapi/bowrain/termbase"

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
			Definition: "A storage location for code",
			Terms: []termbase.Term{
				{Text: "Repository", Locale: model.LocaleEnglish, Status: model.TermApproved},
				{Text: "Depot", Locale: model.LocaleFrench, Status: model.TermApproved},
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

func TestSQLiteTermBase_AddAndGet(t *testing.T) {
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	populateTB(t, tb)
	assert.Equal(t, 3, tb.Count())

	c, ok := tb.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "software", c.Domain)
	assert.Len(t, c.Terms, 3)
}

func TestSQLiteTermBase_Delete(t *testing.T) {
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	populateTB(t, tb)

	err = tb.DeleteConcept("c2")
	require.NoError(t, err)
	assert.Equal(t, 2, tb.Count())

	_, ok := tb.GetConcept("c2")
	assert.False(t, ok)
}

func TestSQLiteTermBase_Update(t *testing.T) {
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	populateTB(t, tb)

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
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	populateTB(t, tb)

	matches := tb.Lookup("Save", termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.Len(t, matches, 1)
	assert.Equal(t, "Save", matches[0].Term.Text)
	assert.Equal(t, 1.0, matches[0].Score)
}

func TestSQLiteTermBase_LookupAll(t *testing.T) {
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	populateTB(t, tb)

	text := "Click Save or Cancel"
	matches := tb.LookupAll(text, termbase.LookupOptions{
		SourceLocale: model.LocaleEnglish,
	})
	require.GreaterOrEqual(t, len(matches), 2)
}

func TestSQLiteTermBase_Search(t *testing.T) {
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	populateTB(t, tb)

	results, total := tb.Search("save", "", "", 0, 100)
	assert.Equal(t, 1, total)
	assert.Len(t, results, 1)
}

func TestSQLiteTermBase_InterfaceCompliance(t *testing.T) {
	tb, err := sqltb.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)

	var _ termbase.TermBase = tb
	assert.NoError(t, tb.Close())
}
