package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A decision outlives the content it judges.
//
// The record on a producer's machine is what decides; this ledger mirrors it.
// A file a checkout no longer holds is still in that record, so a removal that
// took the decision rows with it left the venue holding less than the record,
// with nothing to put them back: a producer sends the record only when its fold
// moves, so the next push that changed nothing removed the rows for good, and
// the file coming back found no approval.
//
// A removal therefore takes the content and leaves the ledger standing. The
// item row stays with it, because that is what the rows are keyed on.

// anchorItemID is the stored id of the item row a ledger row hangs from.
func anchorItemID(t *testing.T, s *PostgresStore, projectID, stream, name string) string {
	t.Helper()
	var id string
	require.NoError(t, s.db.QueryRowContext(t.Context(),
		`SELECT id FROM items WHERE project_id=$1 AND stream=$2 AND name=$3`,
		projectID, stream, name).Scan(&id))
	return id
}

func TestDeleteItem_KeepsTheDecisionsItHolds(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	f := seedOrphanFixture(t, s, p.ID, "main", "en.json")
	anchor := anchorItemID(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteItem(t.Context(), p.ID, "main", "en.json"))

	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=$1 AND block_id=$2`, p.ID, f.blockID),
			"%s: the content goes", table)
	}
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1 AND item_name=$2`, p.ID, "en.json"),
		"blocks: the content goes")

	assert.Equal(t, 1, countRows(t, s, "unit_decisions", `project_id=$1 AND item_name=$2`, p.ID, "en.json"),
		"the ledger row stands, because the producer's record still holds this decision")
	assert.Equal(t, anchor, anchorItemID(t, s, p.ID, "main", "en.json"),
		"the item row stays as the anchor its ledger rows are keyed on")
	assert.Contains(t, listDecisions(t, s, p.ID), "en.json|greeting|nb",
		"and the decision is still readable through the ledger")
}

func TestDeleteItem_HoldingNoDecisionTakesTheItemRow(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()

	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "plain.json", Format: "json"}))
	b := model.NewBlock("greeting", "Hello")
	b.Translatable = true
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "plain.json", []*model.Block{b}))

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "plain.json"))

	assert.Zero(t, countRows(t, s, "items", `project_id=$1 AND name=$2`, p.ID, "plain.json"),
		"an item holding no decision is removed outright, as it always was")
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1 AND item_name=$2`, p.ID, "plain.json"))
}

// Returning content lands on the item its decisions are keyed to, and the
// ledger rows are still hanging from it. Whether the decision reaches the
// returning target is the projection's business, and this asserts nothing
// about it.
func TestDeleteItem_ContentReturningLandsOnTheItemHoldingTheDecisions(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedOrphanFixture(t, s, p.ID, "main", "en.json")
	anchor := anchorItemID(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "en.json"))

	// The file comes back, the way a revert or a branch switch brings it back.
	b := model.NewBlock("greeting", "Hello")
	b.Translatable = true
	b.SetTargetText("nb", "Hei")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))

	assert.Equal(t, anchor, anchorItemID(t, s, p.ID, "main", "en.json"),
		"the returning content lands on the item the ledger is keyed on")
	assert.Contains(t, listDecisions(t, s, p.ID), "en.json|greeting|nb",
		"and the ledger still holds the decision, keyed to that item")
}
