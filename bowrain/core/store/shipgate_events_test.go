package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEvaluateShipState names the gates a scope is evaluated on, and holds the
// rule the quality gate events rest on: the state and the gates come from one
// evaluation, and a scope is pending exactly when one of its gates is unmet.
func TestEvaluateShipState(t *testing.T) {
	all := []string{"translated", "checks", "stale", "rejected"}
	allWithTerms := []string{"translated", "checks", "terms", "stale", "rejected"}
	uncovered := []string{"translated", "stale", "rejected"}
	cases := []struct {
		name       string
		in         ShipStateInputs
		evaluated  []string
		unmet      []string
		notChecked []string
	}{
		{"an empty scope has nothing to ship", ShipStateInputs{}, uncovered, []string{"translated"}, []string{"translated"}},
		{"partial coverage leaves checks and terms unevaluated",
			ShipStateInputs{TranslatedBlocks: 1, TotalBlocks: 2, TermsGoverned: true}, uncovered, []string{"translated"}, nil},
		{"covered and clean where terms govern nothing",
			ShipStateInputs{TranslatedBlocks: 2, TotalBlocks: 2, ApprovedBlocks: 2}, all, nil, nil},
		{"covered and unreviewed meets every gate",
			ShipStateInputs{TranslatedBlocks: 2, TotalBlocks: 2, TermsGoverned: true}, allWithTerms, nil, nil},
		{"a failing block",
			ShipStateInputs{TranslatedBlocks: 2, TotalBlocks: 2, FailingChecks: 1}, all, []string{"checks"}, nil},
		{"governed terminology with no result",
			ShipStateInputs{TranslatedBlocks: 2, TotalBlocks: 2, TermsNotCheckedBlocks: 1, TermsGoverned: true}, allWithTerms, []string{"terms"}, []string{"terms"}},
		{"a terminology result missing where nothing governs still withholds",
			ShipStateInputs{TranslatedBlocks: 2, TotalBlocks: 2, TermsNotCheckedBlocks: 1}, allWithTerms, []string{"terms"}, []string{"terms"}},
		{"a stale decision below full coverage",
			ShipStateInputs{TranslatedBlocks: 1, TotalBlocks: 2, StaleBlocks: 1}, uncovered, []string{"translated", "stale"}, nil},
		{"a failing block below full coverage is not evaluated",
			ShipStateInputs{TranslatedBlocks: 1, TotalBlocks: 2, FailingChecks: 1}, uncovered, []string{"translated"}, nil},
		{"a rejection waiting for a draft",
			ShipStateInputs{TranslatedBlocks: 2, TotalBlocks: 2, RejectedBlocks: 1}, all, []string{"rejected"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, gates := EvaluateShipState(tc.in)
			var evaluated, unmet, notChecked []string
			for _, g := range gates {
				evaluated = append(evaluated, g.Gate)
				if !g.Met {
					unmet = append(unmet, g.Gate)
				}
				if g.NotChecked {
					notChecked = append(notChecked, g.Gate)
				}
			}
			assert.Equal(t, tc.evaluated, evaluated)
			assert.Equal(t, tc.unmet, unmet)
			assert.Equal(t, tc.notChecked, notChecked)
			assert.Equal(t, len(unmet) > 0, state == ShipStatePending, "a scope is pending exactly when a gate is unmet")
			assert.Equal(t, DeriveShipState(tc.in), state, "the served state is the evaluated one")
		})
	}
}
