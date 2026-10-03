package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// TestTieredLookup_ExactOnlyAsksForNoFuzzyCandidates pins that a lookup asking
// for exact answers alone (MinScore 1.0) ends at the exact tiers, whether or
// not they found anything. A fuzzy score reaches 1.0 only for a key the exact
// tiers already compared, so the candidate pool and its scoring would be spent
// on an answer that cannot change; `kapi up` asks this question of every unit
// it plans, and scoring the pool for each one made a run with nothing to do
// cost minutes.
func TestTieredLookup_ExactOnlyAsksForNoFuzzyCandidates(t *testing.T) {
	nb := func(text string) Entry {
		return Entry{ID: "e-" + text, Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: text}}},
			"nb": {{Text: &model.TextRun{Text: "NB " + text}}},
		}}
	}
	tests := []struct {
		name     string
		minScore float64
		exact    []Entry
		fuzzy    []Entry
		answers  int
		pools    int
	}{
		{name: "exact-only, an exact answer", minScore: 1.0, exact: []Entry{nb("Install")}, answers: 1},
		{name: "exact-only, no exact answer", minScore: 1.0, fuzzy: []Entry{nb("Install it")}},
		{name: "fuzzy allowed, no exact answer", minScore: 0.5, fuzzy: []Entry{nb("Install it")}, answers: 1, pools: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pools := 0
			src := CandidateSource{
				Exact: func(_ context.Context, column, _ string, _ model.LocaleID, _ LookupOptions) ([]Entry, error) {
					if column != "plain" {
						return nil, nil
					}
					return tt.exact, nil
				},
				FuzzyCandidates: func(context.Context, string, string, string, model.LocaleID, LookupOptions) ([]Entry, error) {
					pools++
					return tt.fuzzy, nil
				},
			}
			key := NormalizeText("Install")
			matches, err := TieredLookup(context.Background(), key, key, key, nil, "en", "nb",
				LookupOptions{MinScore: tt.minScore, MaxResults: 5}, src)
			require.NoError(t, err)
			assert.Len(t, matches, tt.answers)
			assert.Equal(t, tt.pools, pools, "fuzzy candidate pools asked for")
		})
	}
}
