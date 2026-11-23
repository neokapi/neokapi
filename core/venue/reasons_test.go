package venue

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, []string{"meaning", "newcode", "tone"}, NormalizeReasons([]string{"tone", "newcode", "meaning", "tone"}),
		"sorted and each once; nothing is dropped")
}

// A note or a reason no decision may carry is refused with the same words
// wherever it is checked, never cut or dropped.
func TestCheckDecisionInput(t *testing.T) {
	assert.Nil(t, CheckDecisionInput("Too formal.", []string{"tone", "grammar", ReasonReviewPolicy}))

	err := CheckDecisionInput("", []string{"tone", "x"})
	require.NotNil(t, err)
	assert.Equal(t, "reasons", err.Field)
	assert.Equal(t, `"x" is not a reason: want one of wrong_term, meaning, tone, placeholder, grammar, review_policy`, err.Message)

	long := strings.Repeat("é", MaxDecisionNote+1)
	err = CheckDecisionInput(long, nil)
	require.NotNil(t, err)
	assert.Equal(t, "note", err.Field)
	assert.Nil(t, CheckDecisionInput(strings.Repeat("é", MaxDecisionNote), nil), "the bound counts characters")
}

// Only what a record states anew is checked: a note or a reason the stored
// record already holds passes, however it breaks the bounds.
func TestCheckStatedInput(t *testing.T) {
	long := strings.Repeat("x", MaxDecisionNote+1)
	stored := UnitDecision{Note: long, Reasons: []string{"retired_code", "tone"}}

	assert.Nil(t, CheckStatedInput(&stored, stored), "an echo passes")
	assert.Nil(t, CheckStatedInput(&stored, UnitDecision{Note: long, Reasons: []string{"tone"}}))

	err := CheckStatedInput(&stored, UnitDecision{Note: long + "y"})
	require.NotNil(t, err, "a changed note is a statement")
	assert.Equal(t, "note", err.Field)
	err = CheckStatedInput(&stored, UnitDecision{Note: long, Reasons: []string{"tone", "made_up"}})
	require.NotNil(t, err, "a reason the record did not hold is a statement")
	assert.Equal(t, "reasons", err.Field)
	require.NotNil(t, CheckStatedInput(nil, UnitDecision{Note: long}), "a unit with no record states everything")
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
