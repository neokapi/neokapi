package source

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A venue refuses a rejection of a translation it has since replaced, and the
// project's record follows: it takes the record the venue holds when the venue
// sends one back, and otherwise keeps the basis without the rejection, so the
// same rejection is not sent again on every push.
func TestRetireRefusedVerdicts_StaleRejection(t *testing.T) {
	rejected := state.UnitState{
		Scope:       "locales/en.json",
		Unit:        "greeting",
		Variant:     model.Variant(model.LocaleID("fr")),
		Status:      model.TargetStatusDraft,
		TargetHash:  "target-older",
		ContentHash: "source-greeting",
		Decision: state.Decision{
			ReviewState: venue.ReviewStateRejected,
			By:          "me@example.com",
			At:          "2026-09-14T10:00:00Z",
			Note:        "Renders check as sjekk",
		},
		Updated: "2026-09-14T10:00:00Z",
	}
	held := venue.UnitDecision{
		ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		TargetHash: "target-current", ContentHash: "source-greeting",
		DecidedBy: "reviewer@example.com", DecidedAt: "2026-09-10T10:00:00Z",
		Updated: "2026-09-10T10:00:00Z",
	}
	refusal := venue.DecisionRefusal{
		Locale: "fr", Kind: venue.VerdictDemotion, Reason: venue.RefusedStaleRejection, Count: 1,
	}

	t.Run("the project's record takes the record the venue holds", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, _ := committedProject(t, a, rejected)

		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{refusal},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
				Reason: venue.RefusedStaleRejection, Held: &held,
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, retired)

		after, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent([]venue.UnitDecision{held}), venue.DecisionsComponent(after),
			"the project's record folds to what the venue holds, so the rejection is not sent again")
	})

	t.Run("with no record at the venue the rejection leaves only its basis", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, _ := committedProject(t, a, rejected)

		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{refusal},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
				Reason: venue.RefusedStaleRejection,
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, retired)

		after, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		require.Len(t, after, 1)
		assert.Empty(t, after[0].ReviewState, "the rejection is retired")
		assert.Equal(t, string(model.TargetStatusTranslated), after[0].Status)
		assert.Equal(t, rejected.TargetHash, after[0].TargetHash, "the basis still names the translation it was written for")
	})
}
