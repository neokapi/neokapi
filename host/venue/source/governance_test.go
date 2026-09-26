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
// not the verdict, so the two folds differ, and they go on differing push after
// push until the project's own record follows. Every one of those pushes sends
// the same refused approvals, is refused again, and reports it again.
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
		Status:      model.TargetStatusEstablished,
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

		sent, err := c.projectDecisions(t.Context())
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

		after, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent(held), venue.DecisionsComponent(after),
			"the next push has nothing left to send")

		diff, err := st.RecordDiff(t.Context())
		require.NoError(t, err)
		assert.Zero(t, diff.Changed(),
			"the retired record was written out, so the shards already carry it")
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

		after, err := c.projectDecisions(t.Context())
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

		after, err := c.projectDecisions(t.Context())
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

		before, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		retired, err := c.retireRefusedVerdicts(t.Context(), nil)
		require.NoError(t, err)
		assert.Zero(t, retired)

		after, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent(before), venue.DecisionsComponent(after))
	})
}

// A sign-off the venue kept is written back as the venue holds it. The local
// record took the sign-off back, the venue refused to, and without this step
// every push would take it back again, be refused again, and report it again.
func TestRetireRefusedVerdicts_RestoresAKeptSignOff(t *testing.T) {
	withdrawn := approvedUnit("greeting", "fr")
	withdrawn.Status = model.TargetStatusTranslated
	withdrawn.Decision = state.Decision{}
	withdrawn.Updated = "2026-09-04T10:00:00Z"
	held := venue.UnitDecision{
		ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		TargetHash: withdrawn.TargetHash, ContentHash: withdrawn.ContentHash,
		DecidedBy: "reviewer@example.com", DecidedAt: "2026-09-03T10:00:00Z",
		Updated: "2026-09-03T10:00:00Z",
	}
	refusal := venue.DecisionRefusal{
		Locale: "fr", Kind: venue.VerdictDemotion, Reason: venue.RefusedEstablishedWithdrawal, Count: 1,
	}

	t.Run("the project's record ends where the venue's ledger is", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, st := committedProject(t, a, withdrawn)

		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{refusal},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
				Reason: venue.RefusedEstablishedWithdrawal, Held: &held,
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, 1, retired)

		after, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent([]venue.UnitDecision{held}), venue.DecisionsComponent(after),
			"the project's record folds to what the venue holds, so the next push has nothing to send")
		require.Len(t, after, 1)
		assert.Equal(t, venue.ReviewStateApproved, after[0].ReviewState)
		assert.Equal(t, "reviewer@example.com", after[0].DecidedBy, "the sign-off still names the person who made it")
		assert.Equal(t, string(model.TargetStatusEstablished), after[0].Status)

		diff, err := st.RecordDiff(t.Context())
		require.NoError(t, err)
		assert.Zero(t, diff.Changed(),
			"the retired record was written out, so the shards already carry it")
	})

	t.Run("a refusal that names no record changes nothing", func(t *testing.T) {
		a := &host.App{}
		defer a.Shutdown()
		c, _ := committedProject(t, a, withdrawn)

		before, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
			Refusals: []venue.DecisionRefusal{refusal},
			Units: []venue.RefusedUnit{{
				ItemName: "locales/en.json", Unit: "greeting", Variant: "fr",
				Reason: venue.RefusedEstablishedWithdrawal,
			}},
		})
		require.NoError(t, err)
		assert.Zero(t, retired, "the local record cannot invent the sign-off's decider; a pull settles it")

		after, err := c.projectDecisions(t.Context())
		require.NoError(t, err)
		assert.Equal(t, venue.DecisionsComponent(before), venue.DecisionsComponent(after))
	})
}

// The push sends every decision this checkout holds, so the venue's answer
// covers all of them: a language-wide refusal retires each verdict for that
// language, not only the ones the record on disk happened to carry.
func TestRetireRefusedVerdicts_CoversEveryDecisionThePushSent(t *testing.T) {
	a := &host.App{}
	defer a.Shutdown()
	c, st := committedProject(t, a, approvedUnit("published", "fr"))

	// A second approval, recorded and not yet written into the shards. The push
	// read the ledger, so this one went to the venue as well.
	require.NoError(t, st.Put(t.Context(), approvedUnit("recorded", "fr")))

	retired, err := c.retireRefusedVerdicts(t.Context(), &venue.PushGovernance{
		Refusals: []venue.DecisionRefusal{{
			Locale: "fr", Kind: venue.VerdictApproval,
			Reason: venue.RefusedNoReviewPermission, Count: 2,
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, retired, "both verdicts the push carried are retired")

	for _, unitID := range []string{"published", "recorded"} {
		us, found := st.Get(t.Context(), state.Key{
			Scope: "locales/en.json", Unit: unitID, Variant: model.Variant("fr"),
		})
		require.True(t, found, "unit %s keeps its record", unitID)
		assert.Empty(t, us.Decision.ReviewState,
			"the verdict the venue would not take is gone from %s", unitID)
	}

	diff, err := st.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Zero(t, diff.Changed(), "and the shards carry what the checkout now holds")
}
