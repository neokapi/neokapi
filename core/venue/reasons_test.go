package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A client that predates reasons computes a decision's identity without
// them, so the identity both ends compare leaves them out.
func TestDecisionIdentity_LeavesReasonsOut(t *testing.T) {
	d := UnitDecision{Unit: "u", Variant: "fr", ReviewState: ReviewStateRejected, DecidedAt: "2026-11-23T08:00:00Z"}
	withReasons := d
	withReasons.Reasons = []string{"tone"}
	assert.Equal(t, DecisionIdentity(d), DecisionIdentity(withReasons))
	assert.Equal(t, DecisionsComponent([]UnitDecision{d}), DecisionsComponent([]UnitDecision{withReasons}))
	assert.False(t, SameReasons(d, withReasons))
}

func TestNormalizeReasons(t *testing.T) {
	assert.Nil(t, NormalizeReasons(nil), "not said stays not said")
	assert.Equal(t, []string{}, NormalizeReasons([]string{}), "none stays none")
	assert.Equal(t, []string{"meaning", "tone"}, NormalizeReasons([]string{"tone", "bogus", "meaning", "tone"}))
	_, ok := ValidReasons([]string{"tone", "grammar"})
	assert.True(t, ok)
	bad, ok := ValidReasons([]string{"tone", "x"})
	assert.False(t, ok)
	assert.Equal(t, "x", bad)
}

func TestBoundNote(t *testing.T) {
	long := make([]rune, MaxDecisionNote+10)
	for i := range long {
		long[i] = 'é'
	}
	assert.Len(t, []rune(BoundNote(string(long))), MaxDecisionNote)
	assert.Equal(t, "short", BoundNote("short"))
}

// An echo of the same decision without reasons keeps the stored ones; a
// decision made at another time does not inherit them.
func TestCarryReasons(t *testing.T) {
	stored := UnitDecision{ReviewState: ReviewStateRejected, DecidedBy: "ada", DecidedAt: "t1", Revision: "r1", Reasons: []string{"tone"}}
	echo := stored
	echo.Reasons = nil
	assert.Equal(t, []string{"tone"}, CarryReasons(stored, echo).Reasons)

	later := echo
	later.DecidedAt = "t2"
	assert.Nil(t, CarryReasons(stored, later).Reasons)

	none := stored
	none.Reasons = []string{}
	assert.Equal(t, []string{}, CarryReasons(stored, none).Reasons, "a sender that says none is heard")
}
