package terms_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFuzzyLengthWindow pins the bounds each backend's full-scan fallback uses
// to pre-filter fuzzy candidates.
func TestFuzzyLengthWindow(t *testing.T) {
	cases := []struct {
		name             string
		queryLen         int
		minScore         float64
		wantMin, wantMax int
	}{
		{"default minScore", 13, 0.8, 10, 17},
		// The case the cross-backend parity suite exercises: "installations"
		// (13) against "install" (7) at a MinScore of 0.5. The window this
		// replaced was a fixed 0.7/1.3, which put minLen at 9 and dropped the
		// candidate before it could be scored.
		{"low minScore widens the window", 13, 0.5, 6, 26},
		{"exact-only minScore pins to the query length", 10, 1.0, 10, 10},
		{"empty query", 0, 0.8, 0, 0},
		{"unset minScore bounds nothing", 13, 0, 0, math.MaxInt32},
		{"impossible minScore bounds nothing", 13, 1.5, 0, math.MaxInt32},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			minLen, maxLen := terms.FuzzyLengthWindow(tc.queryLen, tc.minScore)
			assert.Equal(t, tc.wantMin, minLen, "minLen")
			assert.Equal(t, tc.wantMax, maxLen, "maxLen")
		})
	}
}

// TestFuzzyLengthWindow_AdmitsEveryScoringCandidate is the property the window
// exists to guarantee: it may over-admit (the score gate rejects those), but it
// must never exclude a candidate whose Levenshtein ratio reaches minScore. A
// window that does exclude one turns a scoring difference into a silent
// backend-dependent difference, which is what it did before.
func TestFuzzyLengthWindow_AdmitsEveryScoringCandidate(t *testing.T) {
	const query = "installations"
	candidates := []string{
		"install", "installer", "installation", "installations",
		"installations of the thing", "uninstall", "i", "",
	}

	for _, minScore := range []float64{0.5, 0.6, 0.7, 0.8, 0.9} {
		minLen, maxLen := terms.FuzzyLengthWindow(len([]rune(query)), minScore)
		require.LessOrEqual(t, minLen, maxLen)

		for _, cand := range candidates {
			score := memory.LevenshteinRatio(query, cand)
			if score < minScore {
				continue // the score gate rejects it; the window may too
			}
			n := len([]rune(cand))
			assert.GreaterOrEqual(t, n, minLen,
				"minScore=%v: %q scores %v but the window excludes it", minScore, cand, score)
			assert.LessOrEqual(t, n, maxLen,
				"minScore=%v: %q scores %v but the window excludes it", minScore, cand, score)
		}
	}
}

func TestLookupUsesFirstEligibleTierAndProjectScope(t *testing.T) {
	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			for _, row := range []struct{ id, project, text string }{
				{"shared", "", "alert"}, {"local", "here", "alert"}, {"other", "there", "alerts"},
			} {
				require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
					ID: row.id, ProjectID: row.project, Terms: []terms.Term{{Text: row.text, Locale: "en"}},
				}))
			}
			for _, tc := range []struct {
				scope             terms.ProjectScope
				ids               map[string]bool
				query             string
				tier              model.MatchStrategy
				justTerm, passage []string
			}{
				{terms.ProjectScopeAll, nil, "alert", model.MatchStrategyExact, []string{"local", "shared"}, []string{"local", "other"}},
				{terms.ProjectScopeOnly, nil, "alert", model.MatchStrategyExact, []string{"local"}, []string{"local"}},
				{terms.ProjectScopeExclude, nil, "alert", model.MatchStrategyExact, []string{"shared"}, []string{"shared", "other"}},
				{terms.ProjectScopeAll, map[string]bool{"other": true}, "alert", model.MatchStrategyFuzzy, []string{"other"}, []string{"other"}},
				{terms.ProjectScopeAll, map[string]bool{}, "alert", "", nil, nil},
			} {
				opts := terms.LookupOptions{SourceLocale: "en", ProjectID: "here", ProjectScope: tc.scope, ConceptIDs: tc.ids}
				matches := mustLookup(t, tb, tc.query, opts)
				assert.Equal(t, tc.justTerm, terms.MatchedConceptIDs(matches), "%+v", opts)
				for _, m := range matches {
					assert.Equal(t, tc.tier, m.MatchType)
				}
				assert.Equal(t, tc.passage, terms.MatchedConceptIDs(mustLookupAll(t, tb, "alert and alerts", opts)), "%+v", opts)
			}
		})
	}
}

func TestLookupPropagatesUnreadableCandidateConcept(t *testing.T) {
	tb, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tb.Close()) })
	for _, id := range []string{"healthy", "unreadable"} {
		require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
			ID: id, Terms: []terms.Term{{Text: "alert", Locale: "en"}},
		}))
	}
	corruptProperties(t, tb, "unreadable")
	for _, mode := range []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized, model.MatchStrategyFuzzy} {
		t.Run(string(mode), func(t *testing.T) {
			matches, err := tb.Lookup(t.Context(), "alert", terms.LookupOptions{SourceLocale: "en", MatchModes: []model.MatchStrategy{mode}})
			require.ErrorContains(t, err, "unreadable")
			assert.Empty(t, matches)
		})
	}
}

// When more terms fall in the fuzzy length window than the tier scores, both
// stores keep the same candidates, nearest the query's length first, and so
// return the same matches.
func TestFuzzyCandidateLimitAgreesAcrossStores(t *testing.T) {
	ctx := t.Context()
	sqlite, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlite.Close() })
	inmem := terms.NewInMemoryStore()

	var concepts []terms.Concept
	// More rows than the limit, one character longer than the query, and too
	// far from it to score.
	for i := range terms.FuzzyCandidateLimit + 50 {
		concepts = append(concepts, terms.Concept{ID: fmt.Sprintf("n-%04d", i), Terms: []terms.Term{{Text: fmt.Sprintf("zz%04dqqq", i), Locale: "en"}}})
	}
	// A match of the query's length, which the order keeps.
	concepts = append(concepts, terms.Concept{ID: "z-same", Terms: []terms.Term{{Text: "notifyer", Locale: "en"}}})
	// A match one character longer, ordered after the rows above and so past
	// the limit.
	concepts = append(concepts, terms.Concept{ID: "z-longer", Terms: []terms.Term{{Text: "notifiers", Locale: "en"}}})
	for _, c := range concepts {
		require.NoError(t, sqlite.AddConcept(ctx, c))
		require.NoError(t, inmem.AddConcept(ctx, c))
	}

	opts := terms.LookupOptions{SourceLocale: "en", MinScore: 0.7, MatchModes: []model.MatchStrategy{model.MatchStrategyFuzzy}}
	want, err := inmem.Lookup(ctx, "notifier", opts)
	require.NoError(t, err)
	got, err := sqlite.Lookup(ctx, "notifier", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"z-same"}, terms.MatchedConceptIDs(want))
	assert.Equal(t, terms.MatchedConceptIDs(want), terms.MatchedConceptIDs(got))
}
