package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The SQLite ledger honors the same contract the Postgres store's
// decisions_test.go pins — one contract, two backends. Compact rather than
// exhaustive: the shared semantics (idempotency, last-writer-wins, freshness,
// the use-case-2 demotion) are asserted once here to catch backend drift.
func TestUnitDecisions_SQLiteContract(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello")
	src.SetTargetText("nb", "Hei")
	src.Target("nb").Status = model.TargetStatusTranslated
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	decision := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:     string(model.TargetStatusReviewed),
		TargetHash: state.TargetHash("Hei"),
		DecidedBy:  "reviewer@example.com",
		Updated:    "2026-08-04T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)

	status := func() model.TargetStatus {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10,
		})
		require.NoError(t, err)
		for _, sb := range rows {
			if sb.SourceID == "greeting" {
				if tgt := sb.Block.Target("nb"); tgt != nil {
					return tgt.Status
				}
			}
		}
		return ""
	}
	assert.Equal(t, model.TargetStatusReviewed, status(), "approval projects onto the stored target")

	// Idempotent replay.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed)

	// Ledger round-trips.
	got, err := s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "reviewer@example.com", got[0].DecidedBy)

	// Use case 2: a source edit demotes the approval; the fact stays.
	edited := &model.Block{ID: "greeting", Translatable: true}
	edited.SetSourceText("Hello there")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
	assert.Equal(t, model.TargetStatusTranslated, status(),
		"an approval must not survive an edit to the source it was made against")
	got, err = s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Len(t, got, 1, "the decision stays in the ledger")
}

// TestUnitDecisions_RestoredSourceFindsItsApproval_SQLite: a source that comes
// back to the wording a decision blessed converges on that decision — the
// projection returns to the decided rung with no second review.
func TestUnitDecisions_RestoredSourceFindsItsApproval_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello")
	src.SetTargetText("nb", "Hei")
	src.Target("nb").Status = model.TargetStatusTranslated
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"),
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)

	status := func() model.TargetStatus {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		return rows[0].Block.Target("nb").Status
	}
	require.Equal(t, model.TargetStatusReviewed, status())

	rewrite := func(text string) {
		edited := &model.Block{ID: "greeting", Translatable: true}
		edited.SetSourceText(text)
		require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
	}
	rewrite("Hello there")
	assert.Equal(t, model.TargetStatusTranslated, status(), "the approval stops applying")

	rewrite("Hello")
	assert.Equal(t, model.TargetStatusReviewed, status(),
		"a restored source converges on the decision already recorded — no re-review")

	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Zero(t, tallies[0].Stale)
}

// TestTallyDecisionBasis_SQLite pins the grading of recorded decisions against
// the source the project holds now: the basis a decision names and the block's
// content hash are the same value, so the verdict is an equality join.
func TestTallyDecisionBasis_SQLite(t *testing.T) {
	tests := []struct {
		name string
		// basis is the content hash the decision records; "current" means the
		// hash of the stored source, "" means a record written before the basis
		// was tracked.
		basis            string
		rewriteSourceTo  string
		translatable     bool
		unit             string
		wantStale        int
		wantUnknown      int
		wantOwed         int
		wantNoTallyRow   bool
		wantTallyForUnit string
	}{
		{
			name: "basis matches the current source", basis: "current",
			translatable: true, unit: "greeting", wantNoTallyRow: true,
		},
		{
			name: "source rewritten under the decision", basis: "current",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting",
			wantStale: 1, wantOwed: 1,
		},
		{
			name: "decision recorded before the basis was tracked", basis: "",
			translatable: true, unit: "greeting", wantUnknown: 1,
		},
		{
			name: "an unknown basis stays unknown when the source moves", basis: "",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting",
			wantUnknown: 1,
		},
		{
			name: "decision for a unit this store holds no block for", basis: "current",
			translatable: true, unit: "absent", wantNoTallyRow: true,
		},
		{
			name: "a non-translatable block is outside every denominator", basis: "current",
			rewriteSourceTo: "Hello there", translatable: false, unit: "greeting",
			wantNoTallyRow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			src := &model.Block{ID: "greeting", Translatable: tt.translatable}
			src.SetSourceText("Hello")
			src.SetTargetText("nb", "Hei")
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
				edited := &model.Block{ID: "greeting", Translatable: tt.translatable}
				edited.SetSourceText(tt.rewriteSourceTo)
				require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
			}

			tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
			require.NoError(t, err)
			if tt.wantNoTallyRow || (tt.wantStale == 0 && tt.wantUnknown == 0) {
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

// TestRecordDraftBases_SQLite mirrors the Postgres store's TestRecordDraftBases:
// the draft mark beside the decision, never over it, on rows the ledger holds
// and on no others, and the owed count that follows the mark.
func TestRecordDraftBases_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	block := func(id, source, target string) *model.Block {
		b := &model.Block{ID: id, Translatable: true}
		b.SetSourceText(source)
		if target != "" {
			b.SetTargetText("nb", target)
		}
		return b
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("greeting", "Hello", "Hei"),
		block("farewell", "Goodbye", "Ha det"),
		block("untranslated", "See you", ""),
	}))
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

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("greeting", "Hello there", ""),
		block("farewell", "Goodbye then", ""),
		block("untranslated", "See you soon", ""),
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

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("greeting", "Hello again", ""),
	}))
	got = tally()
	assert.Equal(t, 1, got.Owed, "a second rewrite is owed a second draft")
}

// TestUpsertUnitDecisions_StaleBasisDoesNotProject_SQLite: a decision arriving
// against source this store has since rewritten is recorded in the ledger and
// projects nothing — the approval was for wording the project no longer has.
func TestUpsertUnitDecisions_StaleBasisDoesNotProject_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello there")
	src.SetTargetText("nb", "Hei")
	src.Target("nb").Status = model.TargetStatusTranslated
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"), // the wording the reviewer saw
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the decision is a fact and is recorded")

	rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, model.TargetStatusTranslated, rows[0].Block.Target("nb").Status,
		"a decision blessing source the store has rewritten must not project an approval")
}
