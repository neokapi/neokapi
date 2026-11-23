package jobs

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// A push that states a new decision with a note past the bound, or a reason
// that is not a reason code, loses that one decision: the rest of the push
// lands, and the report names the unit and the limit. A record the ledger
// already holds passes back through a push whatever note or reasons it
// carries, so one record stored before the bound cannot fail every push.

// rejectionRecord is a rejection of the unit's current translation.
func (r *rejectionRedraft) rejectionRecord(note string, reasons []string) venue.UnitDecision {
	at := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	return venue.UnitDecision{
		ItemName: r.item, Unit: redraftUnit, Variant: redraftLocale,
		Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
		Note: note, Reasons: reasons,
		Revision: frRevision(r.target()), Basis: srcRevision(redraftSource),
		DecidedBy: "u-translator", DecidedAt: at, Updated: at,
	}
}

// secondSource is a block the push adds beside the decision.
func secondSource() *model.Block {
	b := &model.Block{ID: "b2", Name: "b2", Translatable: true}
	b.SetSourceText("Sign out")
	return b
}

func TestPushRefusesOneDecisionPastTheBoundAndLandsTheRest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		note    string
		reasons []string
		reason  string
	}{
		{"a note past the bound", strings.Repeat("x", venue.MaxDecisionNote+1), nil, venue.RefusedNoteTooLong},
		{"a reason that is not a code", "Too formal.", []string{"tone", "made_up"}, venue.RefusedUnknownReason},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRejectionRedraft(t)
			require.Equal(t, 1, r.draft("job-draft-1"))
			r.deps.ReviewAuthority = pushAuthority{review: map[string]bool{}}

			require.NoError(t, governedPush{
				projectID: r.pid, actor: "u-translator", item: r.item,
				blocks:    []*model.Block{secondSource()},
				decisions: []venue.UnitDecision{r.rejectionRecord(tc.note, tc.reasons)},
			}.run(t, r.deps, "job-bound"), "the push lands")

			rows, err := r.deps.ContentStore.GetBlocks(t.Context(), bstoreQuery(r.pid, r.item))
			require.NoError(t, err)
			assert.Len(t, rows, 2, "the block the push added beside the decision is stored")
			_, held := heldDecision(t, r.deps, r.pid, redraftUnit, redraftLocale)
			assert.False(t, held, "the refused rejection is not recorded")

			report := jobGovernance(t, r.deps, "push-job-bound")
			require.Len(t, report.Refusals, 1)
			assert.Equal(t, venue.DecisionRefusal{
				Locale: redraftLocale, Kind: venue.VerdictDemotion, Reason: tc.reason, Count: 1,
			}, report.Refusals[0])
			require.Len(t, report.Units, 1)
			assert.Equal(t, redraftUnit, report.Units[0].Unit, "the report names the unit")
		})
	}
}

func TestPushEchoOfAStoredRecordPassesWhateverItCarries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		note    string
		reasons []string
	}{
		{"a note stored before the bound", strings.Repeat("x", venue.MaxDecisionNote+500), []string{"tone"}},
		{"a reason code the server no longer knows", "Too formal.", []string{"tone", "retired_code"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRejectionRedraft(t)
			require.Equal(t, 1, r.draft("job-draft-1"))
			r.decisionPush("job-reject", "u-translator", r.target(),
				venue.ReviewStateRejected, model.TargetStatusDraft, time.Hour, "Short.")
			stored := r.ledgerRow()

			// The row as an older server or client left it: written before the
			// bound, or with a code since retired.
			pg, ok := r.deps.ContentStore.(*bstore.PostgresStore)
			require.True(t, ok)
			reasons := `["` + strings.Join(tc.reasons, `","`) + `"]`
			_, err := pg.SQLDB().ExecContext(t.Context(),
				`UPDATE unit_decisions SET note=$1, reasons=$2::jsonb WHERE project_id=$3 AND unit=$4 AND variant=$5`,
				tc.note, reasons, r.pid, redraftUnit, redraftLocale)
			require.NoError(t, err)
			echo := r.ledgerRow()
			require.Equal(t, tc.note, echo.Note)

			// A client that pulled the record sends it back unchanged, with
			// a block beside it.
			r.deps.ReviewAuthority = pushAuthority{review: map[string]bool{}}
			require.NoError(t, governedPush{
				projectID: r.pid, actor: "u-translator", item: r.item,
				blocks:    []*model.Block{secondSource()},
				decisions: []venue.UnitDecision{echo},
			}.run(t, r.deps, "job-echo"))

			report := jobGovernance(t, r.deps, "push-job-echo")
			assert.Empty(t, report.Refusals, "an echo of the stored record is not refused")
			after := r.ledgerRow()
			assert.Equal(t, tc.note, after.Note, "and the stored record is left as it is")
			assert.Equal(t, stored.DecidedAt, after.DecidedAt)
			rows, err := r.deps.ContentStore.GetBlocks(t.Context(), bstoreQuery(r.pid, r.item))
			require.NoError(t, err)
			assert.Len(t, rows, 2, "the rest of the push lands")

			// The store, asked directly, accepts the echo too.
			n, err := pg.UpsertUnitDecisions(t.Context(), r.pid, "main", []venue.UnitDecision{echo})
			require.NoError(t, err)
			assert.Zero(t, n)
		})
	}
}

// bstoreQuery reads every block of the item on main.
func bstoreQuery(projectID, item string) store.BlockQuery {
	return store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: item, Limit: 10}
}
