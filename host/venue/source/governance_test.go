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

// The convergence the whole exchange turns on.
//
// A push sends the project's committed decision record and compares its fold
// against the venue's. When the venue refuses a verdict it keeps the basis and
// not the verdict, so the two folds differ — and they will go on differing,
// push after push, until the project's own record follows. Every one of those
// pushes would send the same refused approvals, be refused again, and report it
// again.
//
// So the property under test is the one the loop needs: after the refusal, the
// project's record folds to exactly what the venue holds, and `kapi status` has
// nothing pending to show for it.

// committedProject writes a project's committed record and returns the
// connector reading it.
func committedProject(t *testing.T, a *host.App, units ...state.UnitState) (*BowrainSourceConnector, *state.WorkStore) {
	t.Helper()
	c := newDecisionsConnector(t, a)
	st, err := a.OpenProjectState(t.Context(), c.project.Root)
	require.NoError(t, err)
	for _, u := range units {
		require.NoError(t, st.Put(t.Context(), u))
	}
	require.NoError(t, st.Commit(t.Context()))
	return c, st
}

// venueLedger is what the venue holds after refusing every verdict in a push:
// the basis each record carried, and nothing more.
func venueLedger(sent []venue.UnitDecision) []venue.UnitDecision {
	out := make([]venue.UnitDecision, 0, len(sent))
	for _, d := range sent {
		if d.CarriesVerdict() {
			d = d.AsBasis(model.TargetStatusTranslated)
		}
		out = append(out, d)
	}
	return out
}

func approvedUnit(unit, locale string) state.UnitState {
	return state.UnitState{
		Scope:       "locales/en.json",
		Unit:        unit,
		Variant:     model.Variant(model.LocaleID(locale)),
		Status:      model.TargetStatusReviewed,
		TargetHash:  "target-" + unit,
		ContentHash: "source-" + unit,
		Decision: state.Decision{
			ReviewState: venue.ReviewStateApproved,
			By:          "me@example.com",
			At:          "2026-09-03T10:00:00Z",
		},
		Updated: "2026-09-03T10:00:00Z",
	}
}

func TestRetireRefusedVerdicts(t *testing.T) {
	t.Run("the project's record ends where the venue's ledger is", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, st := committedProject(t, a, approvedUnit("greeting", "fr"))

		sent, err := c.committedDecisions(t.Context())
		require.NoError(t, err)
		require.Len(t, sent, 1)
		held := venueLedger(sent)
		require.NotEqual(t, venue.DecisionsComponent(sent), venue.DecisionsComponent(held),
			"the fixture must actually diverge, or the test proves nothing")

		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{{
				Locale: "fr", Kind: venue.VerdictApproval,
				Reason: venue.RefusedNoReviewPermission, Count: 1,
			}},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
				Reason: venue.RefusedNoReviewPermission,
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, retired)

		after, err := c.committedDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent(held), venue.DecisionsComponent(after),
			"the next push has nothing left to send")

		pending, err := st.Pending(t.Context())
		require.NoError(t, err)
		assert.Zero(t, pending,
			"the venue's answer about published work is not the person's pending decision")
	})

	t.Run("a language refusal reaches the units the bounded list could not name", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, _ := committedProject(t, a, approvedUnit("one", "fr"), approvedUnit("two", "fr"))

		// The venue named one unit and refused two: the pusher holds no review
		// permission for the language, so every verdict it carried for that
		// language was refused whether or not the list reached it.
		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{{
				Locale: "fr", Kind: venue.VerdictApproval,
				Reason: venue.RefusedNoReviewPermission, Count: 2,
			}},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "one", Variant: "fr",
				Reason: venue.RefusedNoReviewPermission,
			}},
			UnitsTruncated: true,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, retired)

		after, err := c.committedDecisions(t.Context())
		require.NoError(t, err)
		for _, d := range after {
			assert.False(t, d.CarriesVerdict(), "no verdict survives a language-wide refusal: %+v", d)
		}
	})

	t.Run("a verdict the venue accepted is left alone", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, _ := committedProject(t, a, approvedUnit("kept", "de"), approvedUnit("refused", "fr"))

		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{{
				Locale: "fr", Kind: venue.VerdictApproval,
				Reason: venue.RefusedSeparationOfDuties, Count: 1,
			}},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "refused", Variant: "fr",
				Reason: venue.RefusedSeparationOfDuties,
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, retired, "separation of duties is about the unit, not the language")

		after, err := c.committedDecisions(t.Context())
		require.NoError(t, err)
		byUnit := map[string]venue.UnitDecision{}
		for _, d := range after {
			byUnit[d.Unit] = d
		}
		assert.Equal(t, venue.ReviewStateApproved, byUnit["kept"].ReviewState)
		assert.Empty(t, byUnit["refused"].ReviewState)
	})

	t.Run("a push the venue accepted whole retires nothing", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, _ := committedProject(t, a, approvedUnit("greeting", "fr"))

		before, err := c.committedDecisions(t.Context())
		require.NoError(t, err)
		retired, err := c.retireRefusedVerdicts(t.Context(), nil)
		require.NoError(t, err)
		assert.Zero(t, retired)

		after, err := c.committedDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent(before), venue.DecisionsComponent(after))
	})
}
