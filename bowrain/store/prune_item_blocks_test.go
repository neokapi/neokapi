package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PruneItemBlocks is what lets a push say a string is GONE.
//
// A push carries what CHANGED, so a deleted paragraph or a removed `t()` call
// sends nothing at all: the store, upserting what arrives, kept the block for
// good — counted in the item's totals, listed in its content, queued for
// review, dragging the coverage a ship gate reads. The producer declares what
// each item it read now holds; these tests are the other half.

// seedItem stores one item with the named units and returns each unit's stored
// block id, keyed by the unit name the producer sent.
func seedItem(t *testing.T, s *PostgresStore, projectID, stream, itemName string, units ...string) map[string]string {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, s.StoreItem(ctx, projectID, stream, &platstore.Item{Name: itemName, Format: "json"}))
	blocks := make([]*model.Block, 0, len(units))
	for _, u := range units {
		b := model.NewBlock(u, "text of "+u)
		b.Translatable = true
		b.SetTargetText("nb", "tekst for "+u)
		blocks = append(blocks, b)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, projectID, stream, itemName, blocks))

	rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projectID, Stream: stream, ItemName: itemName, Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, rows, len(units))

	// Keyed by the id the PRODUCER sent, which is what a declaration names and
	// what the store records as source_id. The row's own id is the internal one
	// the store minted, and is not a key any caller holds.
	byUnit := map[string]string{}
	for _, r := range rows {
		require.NotEmpty(t, r.SourceID, "an item's blocks record the producer's id")
		byUnit[r.SourceID] = r.Block.ID
	}
	require.Len(t, byUnit, len(units), "each unit stored under its own key")
	return byUnit
}

func itemBlockCount(t *testing.T, s *PostgresStore, projectID, itemName string) int {
	t.Helper()
	return countRows(t, s, "blocks", `project_id=$1 AND item_name=$2`, projectID, itemName)
}

func TestPruneItemBlocks_RemovesWhatTheSourceNoLongerDeclares(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ids := seedItem(t, s, p.ID, "main", "en.json", "greeting", "farewell", "retired")

	// The source dropped `retired`; the other two are declared as still held.
	n, err := s.PruneItemBlocks(t.Context(), p.ID, "main", "en.json", []string{"greeting", "farewell"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 2, itemBlockCount(t, s, p.ID, "en.json"))

	// Every row describing the pruned block goes with it — the rule the other
	// deletion verbs are held to in delete_orphans_test.go.
	require.NotEmpty(t, ids["retired"], "the fixture keys by the producer's id")
	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=$1 AND block_id=$2`, p.ID, ids["retired"]),
			"%s: a pruned block leaves nothing behind", table)
	}
	// And it took only what it was told to.
	assert.Equal(t, 1, countRows(t, s, "translations", `project_id=$1 AND block_id=$2`, p.ID, ids["greeting"]),
		"a block the source still declares keeps its target")
}

// Silence is not deletion — the rule keepUndeclaredProperties follows one level
// down. Nothing here declares anything about `other.json`, so nothing happens
// to it, and a declaration naming every block is a no-op.
func TestPruneItemBlocks_LeavesADeclaredItemWhole(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	seedItem(t, s, p.ID, "main", "en.json", "greeting", "farewell")

	n, err := s.PruneItemBlocks(t.Context(), p.ID, "main", "en.json", []string{"greeting", "farewell"})
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, 2, itemBlockCount(t, s, p.ID, "en.json"))
}

// An empty declaration is a real answer, not an absent one: an item whose last
// translatable string was deleted holds nothing, and is exactly the case this
// exists for. A caller meaning "say nothing" omits the item instead.
func TestPruneItemBlocks_EmptyDeclarationEmptiesTheItem(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	seedItem(t, s, p.ID, "main", "en.json", "greeting", "farewell")

	n, err := s.PruneItemBlocks(t.Context(), p.ID, "main", "en.json", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Zero(t, itemBlockCount(t, s, p.ID, "en.json"))
}

func TestPruneItemBlocks_TouchesNoOtherItem(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	seedItem(t, s, p.ID, "main", "en.json", "greeting", "retired")
	seedItem(t, s, p.ID, "main", "other.json", "greeting", "retired")

	n, err := s.PruneItemBlocks(t.Context(), p.ID, "main", "en.json", []string{"greeting"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, itemBlockCount(t, s, p.ID, "en.json"))
	assert.Equal(t, 2, itemBlockCount(t, s, p.ID, "other.json"),
		"an item this push said nothing about is untouched")
}

// The dashboard is what the bug was visible in: a removed string went on being
// counted, so an item read as larger and less translated than it was.
func TestPruneItemBlocks_ItemStopsCountingTheRemovedBlock(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedItem(t, s, p.ID, "main", "en.json", "greeting", "retired")

	before, err := s.CountBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", ItemName: "en.json", TargetLocale: "nb",
	})
	require.NoError(t, err)
	require.Equal(t, 2, before.Translatable)

	_, err = s.PruneItemBlocks(ctx, p.ID, "main", "en.json", []string{"greeting"})
	require.NoError(t, err)

	after, err := s.CountBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", ItemName: "en.json", TargetLocale: "nb",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, after.Translatable)
}
