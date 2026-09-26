package convergence

import (
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A scope's ship state takes one of three values. A matched gate decides
// between shippable and withheld. A scope no gate matches is not_gated, whatever
// its coverage, unless something withholds it.

// TestRollupGates_ShipStateFollowsTheGate reads the state off each kind of
// scope, including an ungated one at 0% translated, which must not read as
// shippable.
func TestRollupGates_ShipStateFollowsTheGate(t *testing.T) {
	gated := NewCoverageTally()
	gated.Add(Scope{Locale: "fr"}, string(model.TargetStatusTranslated))
	gated.Add(Scope{Locale: "de"}, "")
	ship := gate.RuleSet{Rules: []gate.Rule{{Gate: gate.Gate{"translated": {Pct: 100}}}}}
	by := rollupBy(gated.RollupGates(ship, gate.RuleSet{}))

	assert.Equal(t, ShipStateShippable, by["fr"].ShipState, "gated and clears the gate")
	assert.True(t, by["fr"].Gated)
	assert.Equal(t, ShipStateWithheld, by["de"].ShipState, "gated and short of the gate")
	assert.False(t, by["de"].Shippable)

	ungated := NewCoverageTally()
	ungated.Add(Scope{Locale: "nb"}, "")
	ungated.Add(Scope{Locale: "sv"}, string(model.TargetStatusEstablished))
	by = rollupBy(ungated.RollupGates(gate.RuleSet{}, gate.RuleSet{}))

	for _, loc := range []string{"nb", "sv"} {
		lc := by[loc]
		assert.Equal(t, ShipStateNotGated, lc.ShipState, "%s: no gate matched, at any coverage", loc)
		assert.False(t, lc.Gated, loc)
		assert.True(t, lc.Shippable, "%s: nothing withholds it, so the two-field reading holds", loc)
	}
	assert.Equal(t, 0, by["nb"].Pct["translated"], "not gated at 0% translated")
}

// TestRollupGates_UngatedScopeIsWithheldByAWithhold: each withhold that applies
// without a gate makes an ungated scope withheld, never not_gated.
func TestRollupGates_UngatedScopeIsWithheldByAWithhold(t *testing.T) {
	withholds := map[string]func(*CoverageTally, Scope){
		"failing check": func(tl *CoverageTally, s Scope) {
			tl.Add(s, string(model.TargetStatusTranslated))
			tl.NoteFailingCheck(s)
		},
		"stale": func(tl *CoverageTally, s Scope) { tl.AddStale(s, false) },
		"rejected": func(tl *CoverageTally, s Scope) {
			tl.Add(s, string(model.TargetStatusDraft))
			tl.NoteRejectedAwaitingDraft(s)
		},
		"terms not checked": func(tl *CoverageTally, s Scope) {
			tl.Add(s, string(model.TargetStatusTranslated))
			tl.NoteTermsNotChecked(s)
		},
	}
	for name, withhold := range withholds {
		t.Run(name, func(t *testing.T) {
			tally := NewCoverageTally()
			withhold(tally, Scope{Locale: "nb"})
			rows := tally.RollupGates(gate.RuleSet{}, gate.RuleSet{})
			require.Len(t, rows, 1)
			assert.False(t, rows[0].Gated)
			assert.False(t, rows[0].Shippable)
			assert.Equal(t, ShipStateWithheld, rows[0].ShipState)
		})
	}
}
