package jobs

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rejection judges one translation. When a push carries a rejection of a
// translation the platform has since replaced, the translation the platform
// holds now keeps its rung, its ledger record and its draft mark: the rejection
// is about text nobody is looking at any more.

// decisionPush sends one decision record for the unit and no content, as a push
// does when only the committed record changed.
func (r *rejectionRedraft) decisionPush(jobID, actor, recorded, reviewState string, rung model.TargetStatus, updated time.Duration, note string) {
	r.t.Helper()
	r.deps.ReviewAuthority = pushAuthority{review: map[string]bool{}}
	at := time.Now().UTC().Add(updated).Format(time.RFC3339)
	record := venue.UnitDecision{
		ItemName: r.item, Unit: redraftUnit, Variant: redraftLocale,
		Status: string(rung), ReviewState: reviewState, Note: note,
		TargetHash: state.TargetHash(recorded), ContentHash: state.SourceHash(redraftSource),
		DecidedBy: actor, DecidedAt: at, Updated: at,
	}
	require.NoError(r.t, governedPush{
		projectID: r.pid, actor: actor, item: r.item, decisions: []venue.UnitDecision{record},
	}.run(r.t, r.deps, jobID))
}

// ledgerRow is the unit's ledger record.
func (r *rejectionRedraft) ledgerRow() venue.UnitDecision {
	r.t.Helper()
	d, ok := heldDecision(r.t, r.deps, r.pid, redraftUnit, redraftLocale)
	require.True(r.t, ok)
	return d
}

const olderTranslation = "Slett kontoen (the translation before the last draft)"

func TestPushedRejectionOfAnOlderTranslation(t *testing.T) {
	t.Run("an approved translation keeps its verdict, rung and mark", func(t *testing.T) {
		r := newRejectionRedraft(t)
		require.Equal(t, 1, r.draft("job-draft-1"))
		current := r.target()
		r.verdictPush("job-approve", "u-reviewer", map[string]bool{redraftLocale: true}, current, current,
			model.TargetStatusEstablished, venue.ReviewStateApproved, time.Hour)
		require.Equal(t, model.TargetStatusEstablished, storedTarget(t, r.deps, r.pid, r.item, redraftLocale))
		before, mark := r.ledgerRow(), r.mark()
		require.Equal(t, venue.ReviewStateApproved, before.ReviewState)

		r.decisionPush("job-reject-older", "u-translator", olderTranslation,
			venue.ReviewStateRejected, model.TargetStatusDraft, 2*time.Hour, "Renders check as sjekk")

		assert.Equal(t, model.TargetStatusEstablished, storedTarget(t, r.deps, r.pid, r.item, redraftLocale),
			"the translation the platform holds keeps its rung")
		after := r.ledgerRow()
		assert.Equal(t, venue.ReviewStateApproved, after.ReviewState, "and its verdict")
		assert.Equal(t, before.TargetHash, after.TargetHash, "the ledger still names the translation it approved")
		assert.Equal(t, before.DecidedBy, after.DecidedBy)
		assert.Equal(t, mark, r.mark(), "the draft mark stands")
		assert.Zero(t, r.draft("job-draft-2"), "nothing is re-drafted")

		report := jobGovernance(t, r.deps, "push-job-reject-older")
		require.Len(t, report.Refusals, 1, "the push reports the rejection it did not apply")
		assert.Equal(t, venue.DecisionRefusal{
			Locale: redraftLocale, Kind: venue.VerdictDemotion, Reason: venue.RefusedStaleRejection, Count: 1,
		}, report.Refusals[0])
		require.Len(t, report.Units, 1)
		require.NotNil(t, report.Units[0].Held, "with the record the platform holds, for the producer to take")
		assert.Equal(t, venue.ReviewStateApproved, report.Units[0].Held.ReviewState)
	})

	t.Run("an unreviewed translation keeps its record, rung and mark", func(t *testing.T) {
		r := newRejectionRedraft(t)
		require.Equal(t, 1, r.draft("job-draft-1"))
		rung := storedTarget(t, r.deps, r.pid, r.item, redraftLocale)
		before, mark := r.ledgerRow(), r.mark()
		require.Empty(t, before.ReviewState)

		r.decisionPush("job-reject-older", "u-translator", olderTranslation,
			venue.ReviewStateRejected, model.TargetStatusDraft, time.Hour, "Renders check as sjekk")

		assert.Equal(t, rung, storedTarget(t, r.deps, r.pid, r.item, redraftLocale))
		after := r.ledgerRow()
		assert.Empty(t, after.ReviewState, "no rejection is recorded against the translation the platform holds")
		assert.Equal(t, before.TargetHash, after.TargetHash)
		assert.Equal(t, mark, r.mark())
		assert.Zero(t, r.draft("job-draft-2"))
	})

	t.Run("a rejection for a dropped placeholder re-drafts like any other", func(t *testing.T) {
		r := newRejectionRedraft(t)
		require.Equal(t, 1, r.draft("job-draft-1"))
		current := r.target()

		r.decisionPush("job-reject-placeholder", "u-translator", current,
			venue.ReviewStateRejected, model.TargetStatusDraft, time.Hour, "placeholder dropped")

		d := r.ledgerRow()
		assert.Equal(t, venue.ReviewStateRejected, d.ReviewState)
		assert.Equal(t, "placeholder dropped", d.Note)
		assert.Empty(t, r.mark(), "the rejection clears the mark whatever its reason")
		assert.Equal(t, 1, r.draft("job-draft-2"), "and the next run drafts the unit")
	})
}
