package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
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

func listDecisions(t *testing.T, s *PostgresStore, projectID string) map[string]platstore.UnitDecision {
	t.Helper()
	got, err := s.ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	byKey := map[string]platstore.UnitDecision{}
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

	decision := platstore.UnitDecision{
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

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{decision})
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
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	// A newer decision (sign-off) replaces it.
	newer := decision
	newer.Status = string(model.TargetStatusSignedOff)
	newer.ReviewState = "signed-off"
	newer.DecidedAt = "2026-08-04T11:00:00Z"
	newer.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{newer})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)
	assert.Equal(t, model.TargetStatusSignedOff, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// Replaying the OLD record must not roll the sign-off back.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{decision})
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

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{{
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
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []platstore.UnitDecision{{
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
