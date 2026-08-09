//go:build integration

package terms_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"

	pgtb "github.com/neokapi/neokapi/bowrain/terms"
)

// aggregateFixture is the same shape the pure helpers are pinned to: en-US in
// every concept, de-DE and fr-FR in one each, one concept with no terms.
func aggregateFixture(t *testing.T, tb *pgtb.PostgresStore) {
	t.Helper()
	add := func(id string, locales ...model.LocaleID) {
		cp := terms.Concept{ID: id, Domain: "commerce"}
		for _, locale := range locales {
			cp.Terms = append(cp.Terms, terms.Term{
				Text: id + "-" + string(locale), Locale: locale, Status: model.TermPreferred,
			})
		}
		require.NoError(t, tb.AddConcept(t.Context(), cp))
	}
	add("a", "en-US", "de-DE")
	add("b", "en-US")
	add("c", "en-US", "fr-FR")
	add("d")
}

// TestPgConceptLocaleCoverageMatchesGoAggregate is the guard that keeps the SQL
// aggregate and the in-Go fallback answering the same number: the panel reading
// them must not shift when the store behind it changes.
func TestPgConceptLocaleCoverageMatchesGoAggregate(t *testing.T) {
	tb := openTestPostgresTerms(t)
	aggregateFixture(t, tb)

	got, err := tb.ConceptLocaleCoverage(t.Context())
	require.NoError(t, err)

	all, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, pgtb.CoverageFromConcepts(all), got)

	require.NotEmpty(t, got)
	assert.Equal(t, pgtb.LocaleCoverage{Locale: "en-US", Present: 3, Total: 4, Pct: 75}, got[0])
}

// TestPgConceptCountsMatchesGoAggregate pins the status breakdown to the same
// equivalence, and to the total including a concept with no terms.
func TestPgConceptCountsMatchesGoAggregate(t *testing.T) {
	tb := openTestPostgresTerms(t)
	aggregateFixture(t, tb)

	got, err := tb.ConceptCounts(t.Context())
	require.NoError(t, err)

	all, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, pgtb.CountsFromConcepts(all), got)

	assert.Equal(t, 4, got.Total)
	assert.Equal(t, 3, got.ByStatus[model.TermPreferred])
}

// TestPgSearchRecentOrdersByLastUpdate proves the recency page is ordered in
// the database, so "recently changed" spans the workspace and not one page.
func TestPgSearchRecentOrdersByLastUpdate(t *testing.T) {
	tb := openTestPostgresTerms(t)
	now := time.Now().UTC()
	for i, id := range []string{"oldest", "middle", "newest"} {
		require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
			ID:        id,
			Terms:     []terms.Term{{Text: id, Locale: "en", Status: model.TermApproved}},
			CreatedAt: now,
			UpdatedAt: now.Add(time.Duration(i) * time.Hour),
		}))
	}

	page, total, err := tb.SearchRecent(t.Context(), "", "", "", 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, page, 2)
	assert.Equal(t, "newest", page[0].ID)
	assert.Equal(t, "middle", page[1].ID)
}
