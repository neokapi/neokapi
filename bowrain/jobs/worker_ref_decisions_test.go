package jobs

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/venue"
)

// The worker makes the push's decisions assertion again, over the ledger in the
// push's own transaction. A basis the platform drafted after the client read the
// ref is not a decision the client missed; another reviewer's approval is.
func TestAssertDecisionsHeld_DraftingIsNotAMove(t *testing.T) {
	decided := venue.UnitDecision{
		ItemName: "en.json", Unit: "u1", Variant: "nb",
		Status: "established", ReviewState: "approved", DecidedBy: "ana",
	}
	expected := ref.Ref{Decisions: venue.DecisionsComponent([]venue.UnitDecision{decided})}

	held := []venue.UnitDecision{
		decided,
		{ItemName: "en.json", Unit: "u2", Variant: "nb", TargetHash: "t2", ContentHash: "s2"},
	}
	require.NoError(t, assertDecisionsHeld(expected, held),
		"a basis drafted since the push read the ref is not a decision it missed")

	approvedElsewhere := append(slices.Clone(held), venue.UnitDecision{
		ItemName: "en.json", Unit: "u3", Variant: "nb",
		Status: "established", ReviewState: "approved", DecidedBy: "ben",
	})
	require.Error(t, assertDecisionsHeld(expected, approvedElsewhere),
		"another reviewer's approval still refuses the push")
}
