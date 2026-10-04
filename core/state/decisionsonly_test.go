package state_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// basis is a record of what a producer made and nothing more: a translation,
// the source it was made from, the rung its producer put it on.
func basis(id string) state.UnitState {
	return state.UnitState{
		Unit:        id,
		Variant:     model.EditionKey{Locale: "nb"},
		Scope:       "d-intro",
		Status:      model.TargetStatusTranslated,
		TargetHash:  "t-" + id,
		ContentHash: model.ComputeContentHash("Source " + id),
		Origin:      model.Origin{Kind: model.OriginAI, Engine: "claude"},
	}
}

// The ledger holds decisions only. What a producer made, and from which
// source, is the block history's (C-03); every kind of decision records, and
// a record that decides nothing is refused.
func TestTheLedgerHoldsDecisionsOnly(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(*state.UnitState)
		decides bool
	}{
		{name: "an approval", decides: true, edit: func(u *state.UnitState) {
			u.Status, u.Decision.ReviewState = model.TargetStatusEstablished, "approved"
		}},
		{name: "a rejection", decides: true, edit: func(u *state.UnitState) {
			u.Status, u.Decision.ReviewState = model.TargetStatusDraft, "rejected"
		}},
		{name: "a parked unit", decides: true, edit: func(u *state.UnitState) { u.Decision.Parked = true }},
		{name: "an assignee", decides: true, edit: func(u *state.UnitState) { u.Decision.Assignee = "ada" }},
		{name: "a note", decides: true, edit: func(u *state.UnitState) { u.Decision.Note = "check the tone" }},
		{name: "an agent's pre-review", decides: true, edit: func(u *state.UnitState) {
			u.AIReview = &state.AIReview{Model: "claude", Score: 80}
		}},
		{name: "a source approval", decides: true, edit: func(u *state.UnitState) {
			u.Status, u.SourceStatus = "", model.SourceStatusEstablished
		}},
		{name: "a basis", decides: false, edit: func(*state.UnitState) {}},
		{name: "a basis with its governing context", decides: false, edit: func(u *state.UnitState) {
			u.GoverningFingerprint = "fp-1"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := basis("u1")
			tc.edit(&u)
			assert.Equal(t, tc.decides, u.Decides())

			w, _ := openWork(t)
			for _, record := range []func() error{
				func() error { return w.Put(t.Context(), u) },
				func() error { return w.RecordEntry(t.Context(), u, "", state.OriginVenue) },
				func() error { return w.RecordEntry(t.Context(), u, "", state.OriginImport) },
			} {
				err := record()
				if tc.decides {
					require.NoError(t, err)
					continue
				}
				require.ErrorIs(t, err, state.ErrDecidesNothing)
			}
			_, held := w.Get(t.Context(), u.Key())
			assert.Equal(t, tc.decides, held)
		})
	}
}

// A line of the committed record that decides nothing, which an earlier
// release wrote for a loop pass, is not read into the ledger.
func TestImport_LeavesOutALineThatDecidesNothing(t *testing.T) {
	w, committed := openWork(t)
	decided := basis("decided")
	decided.Status, decided.Decision.ReviewState = model.TargetStatusEstablished, "approved"
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{basis("produced"), decided}))

	require.NoError(t, w.Import(t.Context()))
	all, err := w.All(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "decided", all[0].Unit)
}

// A log an earlier release wrote can carry an entry that decides nothing; the
// projection leaves it out, so a rebuild holds decisions only too.
func TestApplyEntries_LeavesOutAnEntryThatDecidesNothing(t *testing.T) {
	db := sharedDB(t)
	ledger, err := state.OpenLedger(t.Context(), db)
	require.NoError(t, err)
	decided := basis("decided")
	decided.Status, decided.Decision.ReviewState = model.TargetStatusEstablished, "approved"
	now := time.Now().UTC()
	entries := []state.JournalEntry{
		{ID: "e1", State: basis("produced"), Origin: state.OriginImport, Recorded: now},
		{ID: "e2", State: decided, Origin: state.OriginImport, Recorded: now},
	}
	require.NoError(t, state.ApplyEntries(t.Context(), db, entries))
	all, err := ledger.Ledger(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "decided", all[0].Unit)
}
