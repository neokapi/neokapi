package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockWithTarget builds a translatable block carrying an nb target at the
// given rung, the shape a translated block holds after a convergence run.
func blockWithTarget(id, source, target string, status model.TargetStatus) *model.Block {
	b := blockWithText(id, source)
	b.SetTargetText("nb", target)
	b.Target("nb").Status = status
	return b
}

func listDecisions(t *testing.T, s *PostgresStore, projectID string) map[string]venue.UnitDecision {
	t.Helper()
	got, err := s.ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	byKey := map[string]venue.UnitDecision{}
	for _, d := range got {
		byKey[d.ItemName+"|"+d.Unit+"|"+d.Variant] = d
	}
	return byKey
}

func targetStatus(t *testing.T, s *PostgresStore, projectID, itemName, unit string) model.TargetStatus {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: itemName, Limit: 10,
	})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == unit {
			if tgt := sb.Block.Target("nb"); tgt != nil {
				return tgt.Status
			}
			return ""
		}
	}
	t.Fatalf("unit %s not found in %s", unit, itemName)
	return ""
}

// TestUnitDecisions_UpsertProjectsAndIsIdempotent covers the ledger's write
// contract: a fresh decision lands and projects its status onto the stored
// target; replaying the identical record changes nothing; a NEWER record
// replaces it and an OLDER replay never rolls it back.
func TestUnitDecisions_UpsertProjectsAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))

	decision := venue.UnitDecision{
		ItemName:    "en.json",
		Unit:        "greeting",
		Variant:     "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ReviewState: "approved",
		DecidedBy:   "reviewer@example.com",
		DecidedAt:   "2026-08-04T10:00:00Z",
		Updated:     "2026-08-04T10:00:00Z",
	}

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a fresh approval must project onto the stored target")

	ledger := listDecisions(t, s, p.ID)
	require.Len(t, ledger, 1)
	got := ledger["en.json|greeting|nb"]
	assert.Equal(t, "reviewer@example.com", got.DecidedBy)
	assert.Equal(t, "approved", got.ReviewState)

	// Identical replay: the idempotency the full-set-every-push wire relies on.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	// A newer decision (sign-off) replaces it.
	newer := decision
	newer.Status = string(model.TargetStatusSignedOff)
	newer.ReviewState = "signed-off"
	newer.DecidedAt = "2026-08-04T11:00:00Z"
	newer.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{newer})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)
	assert.Equal(t, model.TargetStatusSignedOff, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// Replaying the OLD record must not roll the sign-off back.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed, "an older record never rolls a newer decision back")
	assert.Equal(t, model.TargetStatusSignedOff, targetStatus(t, s, p.ID, "en.json", "greeting"))
}

// TestUnitDecisions_StaleOnArrivalDoesNotProject pins the freshness rule: a
// decision blessing a translation the store does not currently hold is
// recorded in the ledger but moves no status.
func TestUnitDecisions_StaleOnArrivalDoesNotProject(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei der", model.TargetStatusTranslated),
	}))

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:     string(model.TargetStatusReviewed),
		TargetHash: state.TargetHash("Hei"), // blesses a DIFFERENT translation
		DecidedBy:  "reviewer@example.com",
		Updated:    "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the fact is recorded")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a stale-on-arrival decision must not move the status")
	assert.Len(t, listDecisions(t, s, p.ID), 1)
}

// TestUnitDecisions_SourceEditDemotesApproval is use case 2 at the store
// level: an approved unit whose SOURCE changes drops back to the presence
// baseline instead of shipping an approval nobody gave for the new text.
func TestUnitDecisions_SourceEditDemotesApproval(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:     string(model.TargetStatusReviewed),
		TargetHash: state.TargetHash("Hei"),
		DecidedBy:  "reviewer@example.com",
		Updated:    "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	require.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// The source edit arrives — the same shape a push produces: source only,
	// no targets riding along.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello there"),
	}))

	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"an approval must not survive an edit to the source it was made against")

	ledger := listDecisions(t, s, p.ID)
	require.Len(t, ledger, 1)
	assert.Equal(t, "reviewer@example.com", ledger["en.json|greeting|nb"].DecidedBy,
		"the decision stays in the ledger — it is a fact about an older text")
}

// TestUnitDecisions_RestoredSourceFindsItsApproval: the decision is a fact, so
// a source that comes back to the wording it blessed converges on it — the
// projection returns to the decided rung with nobody reviewing anything twice,
// and both flips are in the history.
func TestUnitDecisions_RestoredSourceFindsItsApproval(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"),
		DecidedBy:   "reviewer@example.com",
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	require.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// The source moves: the approval no longer describes the project.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello there"),
	}))
	require.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"))
	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, 1, tallies[0].Stale)

	// The source comes back — the recorded decision applies again.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello"),
	}))
	assert.Equal(t, model.TargetStatusReviewed, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a restored source converges on the decision already recorded — no re-review")
	tallies, err = s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Zero(t, tallies[0].Stale, "the basis matches the source again")

	blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	history, err := s.GetBlockHistory(ctx, p.ID, "main", blocks[0].Block.ID, "nb", 50)
	require.NoError(t, err)
	var kinds []string
	for _, h := range history {
		kinds = append(kinds, h.ChangeType)
	}
	assert.Contains(t, kinds, DecisionStaleEvent, "the demotion is auditable")
	assert.Contains(t, kinds, DecisionRestoredEvent, "so is the recovery")
}

// TestTallyDecisionBasis pins the Postgres grading of recorded decisions
// against the source the project holds now — the equality join between a
// decision's recorded basis and the block's content hash.
func TestTallyDecisionBasis(t *testing.T) {
	tests := []struct {
		name string
		// basis is the content hash the decision records; "current" means the
		// hash of the stored source, "" a record written before the basis was
		// tracked.
		basis           string
		rewriteSourceTo string
		translatable    bool
		unit            string
		wantStale       int
		wantUnknown     int
		wantOwed        int
	}{
		{name: "basis matches the current source", basis: "current", translatable: true, unit: "greeting"},
		{
			name: "source rewritten under the decision", basis: "current",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting", wantStale: 1, wantOwed: 1,
		},
		{name: "decision recorded before the basis was tracked", basis: "", translatable: true, unit: "greeting", wantUnknown: 1},
		{
			name: "an unknown basis stays unknown when the source moves", basis: "",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting", wantUnknown: 1,
		},
		{name: "decision for a unit this store holds no block for", basis: "current", translatable: true, unit: "absent"},
		{
			name: "a non-translatable block is outside every denominator", basis: "current",
			rewriteSourceTo: "Hello there", translatable: false, unit: "greeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			src := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)
			src.Translatable = tt.translatable
			require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

			basis := tt.basis
			if basis == "current" {
				basis = state.SourceHash("Hello")
			}
			_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
				ItemName: "en.json", Unit: tt.unit, Variant: "nb",
				Status:      string(model.TargetStatusReviewed),
				TargetHash:  state.TargetHash("Hei"),
				ContentHash: basis,
				Updated:     "2026-08-04T10:00:00Z",
			}})
			require.NoError(t, err)

			if tt.rewriteSourceTo != "" {
				edited := blockWithText("greeting", tt.rewriteSourceTo)
				edited.Translatable = tt.translatable
				require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
			}

			tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
			require.NoError(t, err)
			if tt.wantStale == 0 && tt.wantUnknown == 0 {
				for _, got := range tallies {
					assert.Zero(t, got.Stale, "nothing is stale")
					assert.Zero(t, got.BasisUnknown, "no basis is unknown")
					assert.Zero(t, got.Owed, "nothing is owed")
				}
				return
			}
			require.Len(t, tallies, 1)
			assert.Equal(t, "en.json", tallies[0].ItemName)
			assert.Equal(t, "nb", tallies[0].Variant)
			assert.Equal(t, tt.wantStale, tallies[0].Stale)
			assert.Equal(t, tt.wantUnknown, tallies[0].BasisUnknown)
			assert.Equal(t, tt.wantOwed, tallies[0].Owed)
		})
	}
}

// TestRecordDraftBases pins the draft mark beside the decision: a stale unit is
// owed a draft until the platform stamps the source it drafted against, the
// stamp never touches the decision, a unit the ledger does not hold gets no
// row, a stale decision on a unit with no target is not owed (it is pending as
// untranslated already), and a second source rewrite makes the unit owed again.
func TestRecordDraftBases(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	decided := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusReviewed)
	undecided := blockWithTarget("farewell", "Goodbye", "Ha det", model.TargetStatusTranslated)
	bare := blockWithText("untranslated", "See you")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{decided, undecided, bare}))

	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{
		{
			ItemName: "en.json", Unit: "greeting", Variant: "nb",
			Status: string(model.TargetStatusReviewed), ReviewState: "approved", DecidedBy: "reviewer-1",
			TargetHash: state.TargetHash("Hei"), ContentHash: state.SourceHash("Hello"),
			Updated: "2026-08-04T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "farewell", Variant: "nb",
			TargetHash: state.TargetHash("Ha det"), ContentHash: state.SourceHash("Goodbye"),
			Updated: "2026-08-04T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "untranslated", Variant: "nb",
			Status: string(model.TargetStatusReviewed), ReviewState: "approved", DecidedBy: "reviewer-1",
			ContentHash: state.SourceHash("See you"),
			Updated:     "2026-08-04T10:00:00Z",
		},
	})
	require.NoError(t, err)

	// Every source moves. All three records read stale; the two units that
	// carry a target are owed a draft.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello there"),
		blockWithText("farewell", "Goodbye then"),
		blockWithText("untranslated", "See you soon"),
	}))
	tally := func() platstore.DecisionBasisTally {
		t.Helper()
		tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
		require.NoError(t, err)
		require.Len(t, tallies, 1, "one (item, variant) scope")
		return tallies[0]
	}
	got := tally()
	assert.Equal(t, 3, got.Stale)
	assert.Equal(t, 2, got.Owed, "a stale decision on a unit with no target is not owed")

	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "greeting", Variant: "nb", SourceHash: state.SourceHash("Hello there")},
		{ItemName: "en.json", Unit: "farewell", Variant: "nb", SourceHash: state.SourceHash("Goodbye then")},
		{ItemName: "en.json", Unit: "absent", Variant: "nb", SourceHash: state.SourceHash("nothing")},
	}))
	got = tally()
	assert.Equal(t, 3, got.Stale, "the decisions stay stale until a person replaces them")
	assert.Zero(t, got.Owed, "drafted against the current source: the loop owes nothing")

	drafts, err := s.ListDraftBases(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, drafts, 2, "a unit the ledger does not hold gets no row")
	assert.Equal(t, platstore.DraftBasis{ItemName: "en.json", Unit: "farewell", Variant: "nb", SourceHash: state.SourceHash("Goodbye then")}, drafts[0])
	assert.Equal(t, platstore.DraftBasis{ItemName: "en.json", Unit: "greeting", Variant: "nb", SourceHash: state.SourceHash("Hello there")}, drafts[1])

	records, err := s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	byUnit := map[string]venue.UnitDecision{}
	for _, d := range records {
		byUnit[d.Unit] = d
	}
	require.Len(t, byUnit, 3)
	assert.Equal(t, "approved", byUnit["greeting"].ReviewState, "the stamp never touches the decision")
	assert.Equal(t, "reviewer-1", byUnit["greeting"].DecidedBy)
	assert.Equal(t, state.SourceHash("Hello"), byUnit["greeting"].ContentHash)
	assert.Equal(t, state.TargetHash("Hei"), byUnit["greeting"].TargetHash)

	// The source moves again, away from the mark: owed once more.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello again"),
	}))
	got = tally()
	assert.Equal(t, 3, got.Stale)
	assert.Equal(t, 1, got.Owed, "a second rewrite is owed a second draft")
}

// TestUnitDecisions_StaleBasisDoesNotProject: a decision arriving against
// source this store has since rewritten is recorded and projects nothing — the
// approval was for wording the project no longer has.
func TestUnitDecisions_StaleBasisDoesNotProject(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello there", "Hei", model.TargetStatusTranslated),
	}))

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"), // the wording the reviewer saw
		DecidedBy:   "reviewer@example.com",
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the fact is recorded")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a decision blessing source the store has rewritten must not project an approval")
	assert.Len(t, listDecisions(t, s, p.ID), 1)
}

// TestUnitDecisions_GoverningFingerprintRoundTrips: the fingerprint of the
// context a decision was made under is stored beside it and read back, a record
// that gains one is a change the store writes, and one made under a moved
// context replaces the one before it. The SQLite store pins the same contract.
func TestUnitDecisions_GoverningFingerprintRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	legacy := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusReviewed), TargetHash: state.TargetHash("Hei"),
		ReviewState: "approved", DecidedBy: "reviewer@example.com", Updated: "2026-08-04T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{legacy})
	require.NoError(t, err)
	require.Equal(t, 1, changed)

	stamped := legacy
	stamped.GoverningFingerprint = "fp-governing"
	stamped.Updated = "2026-08-04T10:30:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{stamped})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "a record that gains a fingerprint is a change")

	got, err := s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "fp-governing", got.GoverningFingerprint)
	assert.Equal(t, "fp-governing", listDecisions(t, s, p.ID)["en.json|greeting|nb"].GoverningFingerprint)

	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{stamped})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	moved := stamped
	moved.GoverningFingerprint = "fp-moved"
	moved.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{moved})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the same verdict under a moved context is a new decision")
	got, err = s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	assert.Equal(t, "fp-moved", got.GoverningFingerprint)
}
