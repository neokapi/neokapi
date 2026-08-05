package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
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

	decision := platstore.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:     string(model.TargetStatusReviewed),
		TargetHash: state.TargetHash("Hei"),
		DecidedBy:  "reviewer@example.com",
		Updated:    "2026-08-04T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{decision})
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
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{decision})
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
