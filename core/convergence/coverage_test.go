package convergence

import (
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rollupBy(rows []LocaleCoverage) map[string]LocaleCoverage {
	m := map[string]LocaleCoverage{}
	for _, r := range rows {
		m[r.Locale] = r
	}
	return m
}

// TestRollupGates_VerifiedIndependentOfShippable exercises the two-gate model:
// the verified gate is evaluated the same way as the ship gate, over the same
// distribution, but a locale can ship without being verified.
func TestRollupGates_VerifiedIndependentOfShippable(t *testing.T) {
	tally := NewCoverageTally()
	// fr: fully reviewed → clears both gates.
	tally.Add(Scope{Locale: "fr"}, string(model.TargetStatusReviewed))
	// de: translated but not reviewed → ships, NOT verified (flagged AI).
	tally.Add(Scope{Locale: "de"}, string(model.TargetStatusTranslated))
	// ja: untranslated → neither ships nor verifies.
	tally.Add(Scope{Locale: "ja"}, "")

	ship := gate.RuleSet{Rules: []gate.Rule{{Gate: gate.Gate{"translated": {Pct: 100}}}}}
	verified := gate.RuleSet{Rules: []gate.Rule{{Gate: gate.Gate{"reviewed": {Pct: 100}}}}}

	by := rollupBy(tally.RollupGates(ship, verified))

	assert.True(t, by["fr"].Shippable)
	assert.True(t, by["fr"].Verified, "100% reviewed clears the verified gate")

	assert.True(t, by["de"].Shippable, "100% translated clears the ship gate")
	assert.False(t, by["de"].Verified, "not reviewed → not verified (ships flagged AI)")

	assert.False(t, by["ja"].Shippable)
	assert.False(t, by["ja"].Verified)
}

// TestRollupGates_NoVerifiedGate_NothingVerified is the default: with an empty
// verified ruleset, every locale reads unverified even when fully reviewed.
func TestRollupGates_NoVerifiedGate_NothingVerified(t *testing.T) {
	tally := NewCoverageTally()
	tally.Add(Scope{Locale: "fr"}, string(model.TargetStatusReviewed))

	ship := gate.RuleSet{Rules: []gate.Rule{{Gate: gate.Gate{"translated": {Pct: 100}}}}}
	rows := tally.RollupGates(ship, gate.RuleSet{})
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Shippable)
	assert.False(t, rows[0].Verified, "no verified gate → nothing is verified, even fully reviewed content")
}

// TestRollup_BackCompat_VerifiedFalse confirms the single-gate Rollup (existing
// callers) leaves Verified false: it is RollupGates with no verified gate.
func TestRollup_BackCompat_VerifiedFalse(t *testing.T) {
	tally := NewCoverageTally()
	tally.Add(Scope{Locale: "fr"}, string(model.TargetStatusReviewed))
	rows := tally.Rollup(gate.RuleSet{Rules: []gate.Rule{{Gate: gate.Gate{"reviewed": {Pct: 100}}}}})
	require.Len(t, rows, 1)
	assert.True(t, rows[0].Shippable)
	assert.False(t, rows[0].Verified)
}
