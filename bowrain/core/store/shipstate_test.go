package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeriveShipState covers the boundary cases of the ship-state rule:
// governed = coverage 100% AND no failing checks AND no stale unit AND no
// rejected unit AND a terminology result for every block AND terminology
// governs the locale AND every block approved; approved = the same in a locale
// terminology does not govern; ai_shippable = the same without full approval;
// pending = anything less (empty scope, partial coverage, failing checks, stale
// content, refused content, governed terminology with no result).
func TestDeriveShipState(t *testing.T) {
	// governed is a scope that clears every dimension, which a case narrows.
	governed := func(edit func(*ShipStateInputs)) ShipStateInputs {
		in := ShipStateInputs{TranslatedBlocks: 10, TotalBlocks: 10, ApprovedBlocks: 10, TermsGoverned: true}
		if edit != nil {
			edit(&in)
		}
		return in
	}
	tests := []struct {
		name string
		in   ShipStateInputs
		want ShipState
	}{
		{"empty locale (nothing to ship)", ShipStateInputs{TermsGoverned: true}, ShipStatePending},
		{"untranslated", governed(func(in *ShipStateInputs) { in.TranslatedBlocks, in.ApprovedBlocks = 0, 0 }), ShipStatePending},
		{"partial coverage", governed(func(in *ShipStateInputs) { in.TranslatedBlocks, in.ApprovedBlocks = 9, 0 }), ShipStatePending},
		{"partial coverage all approved so far", governed(func(in *ShipStateInputs) { in.TranslatedBlocks, in.ApprovedBlocks = 9, 9 }), ShipStatePending},
		{"full coverage, no review, checks pass", governed(func(in *ShipStateInputs) { in.ApprovedBlocks = 0 }), ShipStateAIShippable},
		{"full coverage, mixed approval", governed(func(in *ShipStateInputs) { in.ApprovedBlocks = 5 }), ShipStateAIShippable},
		{"full coverage, one short of full approval", governed(func(in *ShipStateInputs) { in.ApprovedBlocks = 9 }), ShipStateAIShippable},
		{"full coverage, fully approved", governed(nil), ShipStateGoverned},
		{"single block approved", ShipStateInputs{TranslatedBlocks: 1, TotalBlocks: 1, ApprovedBlocks: 1, TermsGoverned: true}, ShipStateGoverned},
		{"failing checks demote below the gate", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.FailingChecks = 0, 1 }), ShipStatePending},
		{"failing checks demote even a fully approved locale", governed(func(in *ShipStateInputs) { in.FailingChecks = 2 }), ShipStatePending},
		// A stale unit is not a shortfall of quantity: the locale is fully
		// covered and clean, and still cannot ship, because one target renders
		// wording the project has rewritten.
		{"one stale unit withholds an unreviewed locale", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.StaleBlocks = 0, 1 }), ShipStatePending},
		{"one stale unit withholds a fully approved locale", governed(func(in *ShipStateInputs) { in.StaleBlocks = 1 }), ShipStatePending},
		{"stale and failing together stay pending", governed(func(in *ShipStateInputs) { in.FailingChecks, in.StaleBlocks = 3, 2 }), ShipStatePending},
		// A rejection is not a shortfall of quantity either: the wording is
		// there and a person refused it.
		{"one rejected unit withholds an unreviewed locale", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.RejectedBlocks = 0, 1 }), ShipStatePending},
		{"one rejected unit withholds an otherwise clean locale", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.RejectedBlocks = 9, 1 }), ShipStatePending},
		{"rejected and stale together stay pending", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.StaleBlocks, in.RejectedBlocks = 0, 2, 1 }), ShipStatePending},
		// Governed terminology with no result is a check that did not run, and
		// it withholds the scope as a failing check does.
		{"a governed block with no terminology result withholds a fully approved locale", governed(func(in *ShipStateInputs) { in.TermsNotCheckedBlocks = 1 }), ShipStatePending},
		{"a governed block with no terminology result withholds an unreviewed locale", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.TermsNotCheckedBlocks = 0, 1 }), ShipStatePending},
		// Terminology that governs nothing withholds nothing, and a fully
		// approved locale with nothing bound beyond the checks is approved, not
		// governed.
		{"ungoverned terminology leaves an unreviewed locale shippable", governed(func(in *ShipStateInputs) { in.ApprovedBlocks, in.TermsGoverned = 0, false }), ShipStateAIShippable},
		{"a fully approved locale nothing governs is approved", governed(func(in *ShipStateInputs) { in.TermsGoverned = false }), ShipStateApproved},
		{"ungoverned terminology still leaves failing checks pending", governed(func(in *ShipStateInputs) { in.FailingChecks, in.TermsGoverned = 1, false }), ShipStatePending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DeriveShipState(tt.in))
		})
	}
}
