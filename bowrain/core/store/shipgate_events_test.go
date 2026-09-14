package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// shipInputs is what applyShipStates hands DeriveShipState for one scope.
func shipInputs(ls LocaleTranslationStats) ShipStateInputs {
	return ShipStateInputs{
		TranslatedBlocks:      ls.TranslatedBlocks,
		TotalBlocks:           ls.TotalBlocks,
		ApprovedBlocks:        ls.ApprovedBlocks,
		FailingChecks:         ls.FailingChecks,
		StaleBlocks:           ls.StaleBlocks,
		RejectedBlocks:        ls.RejectedAwaitingDraftBlocks,
		TermsNotCheckedBlocks: ls.TermsNotCheckedBlocks,
		TermsGoverned:         ls.ComplianceBasis.GovernsTerms(),
	}
}

// TestShipGateResults names the gates a scope is evaluated on, and holds the
// rule the quality gate events rest on: a scope is pending exactly when one of
// its gates is unmet.
func TestShipGateResults(t *testing.T) {
	governed := ComplianceBasisFor(false, true)
	all := []string{"translated", "checks", "stale", "rejected"}
	allWithTerms := []string{"translated", "checks", "terms", "stale", "rejected"}
	uncovered := []string{"translated", "stale", "rejected"}
	cases := []struct {
		name       string
		ls         LocaleTranslationStats
		evaluated  []string
		unmet      []string
		notChecked []string
	}{
		{"an empty scope has nothing to ship", LocaleTranslationStats{}, uncovered, []string{"translated"}, []string{"translated"}},
		{"partial coverage leaves checks and terms unevaluated",
			LocaleTranslationStats{TranslatedBlocks: 1, TotalBlocks: 2, ComplianceBasis: governed}, uncovered, []string{"translated"}, nil},
		{"covered and clean where terms govern nothing",
			LocaleTranslationStats{TranslatedBlocks: 2, TotalBlocks: 2, ApprovedBlocks: 2, ComplianceBasis: ComplianceBasisChecks}, all, nil, nil},
		{"covered and unreviewed meets every gate",
			LocaleTranslationStats{TranslatedBlocks: 2, TotalBlocks: 2, ComplianceBasis: governed}, allWithTerms, nil, nil},
		{"a failing block",
			LocaleTranslationStats{TranslatedBlocks: 2, TotalBlocks: 2, FailingChecks: 1}, all, []string{"checks"}, nil},
		{"governed terminology with no result",
			LocaleTranslationStats{TranslatedBlocks: 2, TotalBlocks: 2, TermsNotCheckedBlocks: 1, ComplianceBasis: governed}, allWithTerms, []string{"terms"}, []string{"terms"}},
		{"a stale decision below full coverage",
			LocaleTranslationStats{TranslatedBlocks: 1, TotalBlocks: 2, StaleBlocks: 1}, uncovered, []string{"translated", "stale"}, nil},
		{"a rejection waiting for a draft",
			LocaleTranslationStats{TranslatedBlocks: 2, TotalBlocks: 2, RejectedAwaitingDraftBlocks: 1}, all, []string{"rejected"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var evaluated, unmet, notChecked []string
			for _, g := range ShipGateResults(tc.ls) {
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
			assert.Equal(t, len(unmet) > 0, DeriveShipState(shipInputs(tc.ls)) == ShipStatePending,
				"a scope is pending exactly when a gate is unmet")
		})
	}
}
