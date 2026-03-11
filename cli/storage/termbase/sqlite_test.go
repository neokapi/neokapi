package termbase

import (
	"fmt"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	fw "github.com/gokapi/gokapi/core/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeConcept(id, domain string, terms ...fw.Term) fw.Concept {
	return fw.Concept{
		ID:     id,
		Domain: domain,
		Terms:  terms,
	}
}

func TestSQLiteTermBase_BasicOperations(t *testing.T) {
	tb, err := NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	concept := makeConcept("c1", "IT",
		fw.Term{Text: "computer", Locale: "en-US", Status: "approved"},
		fw.Term{Text: "ordinateur", Locale: "fr-FR", Status: "approved"},
	)
	require.NoError(t, tb.AddConcept(concept))
	assert.Equal(t, 1, tb.Count())

	c, ok := tb.GetConcept("c1")
	assert.True(t, ok)
	assert.Equal(t, "IT", c.Domain)
	assert.Len(t, c.Terms, 2)

	require.NoError(t, tb.DeleteConcept("c1"))
	assert.Equal(t, 0, tb.Count())
}

func TestSQLiteTermBase_ExactLookup(t *testing.T) {
	tb, err := NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(makeConcept("c1", "IT",
		fw.Term{Text: "computer", Locale: "en-US", Status: "approved"},
		fw.Term{Text: "ordinateur", Locale: "fr-FR", Status: "approved"},
	)))

	matches := tb.Lookup("computer", fw.LookupOptions{
		SourceLocale: "en-US",
	})
	require.Len(t, matches, 1)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, model.MatchStrategyExact, matches[0].MatchType)
}

func TestSQLiteTermBase_FuzzyLookup(t *testing.T) {
	tb, err := NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(makeConcept("c1", "IT",
		fw.Term{Text: "computer", Locale: "en-US", Status: "approved"},
	)))
	require.NoError(t, tb.AddConcept(makeConcept("c2", "IT",
		fw.Term{Text: "computing", Locale: "en-US", Status: "approved"},
	)))
	require.NoError(t, tb.AddConcept(makeConcept("c3", "IT",
		fw.Term{Text: "unrelated", Locale: "en-US", Status: "approved"},
	)))

	matches := tb.Lookup("computers", fw.LookupOptions{
		SourceLocale: "en-US",
		MinScore:     0.7,
	})
	assert.GreaterOrEqual(t, len(matches), 1, "should find fuzzy matches")
	for _, m := range matches {
		assert.GreaterOrEqual(t, m.Score, 0.7)
		assert.NotEqual(t, "c3", m.Concept.ID, "should not match unrelated term")
	}
}

func TestSQLiteTermBase_Search(t *testing.T) {
	tb, err := NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(makeConcept("c1", "IT",
		fw.Term{Text: "computer", Locale: "en-US", Status: "approved"},
		fw.Term{Text: "ordinateur", Locale: "fr-FR", Status: "approved"},
	)))
	require.NoError(t, tb.AddConcept(makeConcept("c2", "IT",
		fw.Term{Text: "keyboard", Locale: "en-US", Status: "approved"},
		fw.Term{Text: "clavier", Locale: "fr-FR", Status: "approved"},
	)))

	concepts, total := tb.Search("computer", "", "", 0, 10)
	assert.GreaterOrEqual(t, total, 1)
	assert.GreaterOrEqual(t, len(concepts), 1)

	// Search with locale filter.
	concepts, _ = tb.Search("clavier", "", "fr-FR", 0, 10)
	assert.GreaterOrEqual(t, len(concepts), 1)
}

func TestSQLiteTermBase_ScaleTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}

	tb, err := NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	for i := 0; i < 500; i++ {
		require.NoError(t, tb.AddConcept(makeConcept(
			fmt.Sprintf("c%d", i), "domain",
			fw.Term{Text: fmt.Sprintf("terminology entry number %d", i), Locale: "en-US", Status: "approved"},
			fw.Term{Text: fmt.Sprintf("entrée terminologique numéro %d", i), Locale: "fr-FR", Status: "approved"},
		)))
	}

	assert.Equal(t, 500, tb.Count())

	// Fuzzy lookup.
	matches := tb.Lookup("terminology entry number 42", fw.LookupOptions{
		SourceLocale: "en-US",
		MinScore:     0.7,
	})
	assert.GreaterOrEqual(t, len(matches), 1, "should find fuzzy matches in 500 concepts")

	// Search.
	concepts, total := tb.Search("terminology", "", "", 0, 10)
	assert.GreaterOrEqual(t, total, 1)
	assert.GreaterOrEqual(t, len(concepts), 1)
}
