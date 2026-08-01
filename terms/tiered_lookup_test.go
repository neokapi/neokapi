package terms_test

import (
	"math"
	"testing"

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
