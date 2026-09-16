package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The SQLite store honors the same rule the Postgres store's
// decisions_removal_test.go pins: a removal takes the content and leaves the
// ledger standing, with the item row as the anchor its rows are keyed on.

// anchorItemID is the stored id of the item row a ledger row hangs from.
func anchorItemID(t *testing.T, s *SQLiteStore, projectID, stream, name string) string {
	t.Helper()
	var id string
	require.NoError(t, s.db.QueryRowContext(t.Context(),
		`SELECT id FROM items WHERE project_id=? AND stream=? AND name=?`,
		projectID, stream, name).Scan(&id))
	return id
}

func TestDeleteItem_KeepsTheDecisionsItHolds_SQLite(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	blockID := seedOrphanFixture(t, s, p.ID, "main", "en.json")
	anchor := anchorItemID(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteItem(t.Context(), p.ID, "main", "en.json"))

	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=? AND block_id=?`, p.ID, blockID),
			"%s: the content goes", table)
	}
	assert.Zero(t, countRows(t, s, "blocks", `project_id=? AND item_name=?`, p.ID, "en.json"),
		"blocks: the content goes")

	assert.Equal(t, 1, countRows(t, s, "unit_decisions", `project_id=? AND item_name=?`, p.ID, "en.json"),
		"the ledger row stands, because the producer's record still holds this decision")
	assert.Equal(t, anchor, anchorItemID(t, s, p.ID, "main", "en.json"),
		"the item row stays as the anchor its ledger rows are keyed on")
}

func TestDeleteItem_HoldingNoDecisionTakesTheItemRow_SQLite(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()

	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "plain.json", Format: "json"}))
	b := model.NewBlock("greeting", "Hello")
	b.Translatable = true
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "plain.json", []*model.Block{b}))

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "plain.json"))

	assert.Zero(t, countRows(t, s, "items", `project_id=? AND name=?`, p.ID, "plain.json"),
		"an item holding no decision is removed outright, as it always was")
	assert.Zero(t, countRows(t, s, "blocks", `project_id=? AND item_name=?`, p.ID, "plain.json"))
}

func TestDeleteItem_ContentReturningFindsItsDecisions_SQLite(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedOrphanFixture(t, s, p.ID, "main", "en.json")
	anchor := anchorItemID(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "en.json"))

	b := model.NewBlock("greeting", "Hello")
	b.Translatable = true
	b.SetTargetText("nb", "Hei")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))

	assert.Equal(t, anchor, anchorItemID(t, s, p.ID, "main", "en.json"),
		"the returning content lands on the item the ledger is keyed on")
	assert.Equal(t, 1, countRows(t, s, "unit_decisions", `project_id=? AND item_name=?`, p.ID, "en.json"),
		"so the approval is there to be found")
}
