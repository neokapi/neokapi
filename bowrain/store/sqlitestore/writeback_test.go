package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readItemByKey reads an item's blocks keyed by the durable key they were
// stored under.
func readItemByKey(t *testing.T, s *SQLiteStore, projectID, itemName string) map[string]*venue.StoredBlock {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: itemName, Limit: 100,
	})
	require.NoError(t, err)
	byKey := make(map[string]*venue.StoredBlock, len(rows))
	for _, sb := range rows {
		byKey[sb.SourceID] = sb
	}
	return byKey
}

// A write-back lands only on the row it was read from, and only while that row
// holds the content the caller read, as it does on the Postgres store.
func TestWriteBackBlocks_LandsOnlyOnTheRowItWasRead(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "en.json", Format: "json"}))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		model.NewBlock("same", "Unchanged"),
		model.NewBlock("gone", "Removed"),
		model.NewBlock("moved", "Before"),
	}))
	read := readItemByKey(t, s, p.ID, "en.json")
	require.Len(t, read, 3)

	require.NoError(t, s.DeleteBlock(ctx, p.ID, "main", read["gone"].Block.ID))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json",
		[]*model.Block{model.NewBlock("moved", "After")}))

	reads := make([]*venue.StoredBlock, 0, len(read))
	for _, key := range []string{"same", "gone", "moved"} {
		read[key].Block.SetTargetText("nb", "Oversatt "+key)
		reads = append(reads, read[key])
	}
	res, err := s.WriteBackBlocks(ctx, p.ID, "main", reads)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Written)
	assert.ElementsMatch(t, []string{read["gone"].Block.ID, read["moved"].Block.ID}, res.Skipped)

	after := readItemByKey(t, s, p.ID, "en.json")
	require.Len(t, after, 2, "the removed block is not stored again")
	assert.Equal(t, "Oversatt same", after["same"].Block.TargetText("nb"))
	assert.Equal(t, "After", after["moved"].Block.SourceText(), "the newer source stands")
	assert.Empty(t, after["moved"].Block.TargetText("nb"), "a target of the old source does not land on the new one")
	assert.Zero(t, countRows(t, s, "blocks", `project_id=? AND item_name=''`, p.ID))
}

// A caller that changes the source it read still writes it.
func TestWriteBackBlocks_ASourceEditOfTheReadContentLands(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "en.json", Format: "json"}))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{model.NewBlock("edit", "Before")}))
	sb := readItemByKey(t, s, p.ID, "en.json")["edit"]
	require.NotNil(t, sb)

	sb.Block.SetSourceText("After")
	res, err := s.WriteBackBlocks(ctx, p.ID, "main", []*venue.StoredBlock{sb})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Written)
	assert.Empty(t, res.Skipped)
	assert.Equal(t, "After", readItemByKey(t, s, p.ID, "en.json")["edit"].Block.SourceText())
	assert.Equal(t, 1, countRows(t, s, "change_log",
		`project_id=? AND block_id=? AND change_type='source_modified'`, p.ID, sb.Block.ID))
}

// A write-back leaves the stored context hash as the push stored it, as it does
// on the Postgres store.
func TestWriteBackBlocks_KeepsTheStoredContextHash(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "en.json", Format: "json"}))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{model.NewBlock("k", "Hello")}))
	sb := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, sb)
	pushed := sb.ContextHash

	if sb.Block.Properties == nil {
		sb.Block.Properties = map[string]string{}
	}
	sb.Block.Properties["__source_settled_hash"] = sb.ContentHash
	res, err := s.WriteBackBlocks(ctx, p.ID, "main", []*venue.StoredBlock{sb})
	require.NoError(t, err)
	require.Equal(t, 1, res.Written)

	after := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, after)
	assert.Equal(t, sb.ContentHash, after.Block.Properties["__source_settled_hash"], "the recorded property is stored")
	assert.Equal(t, pushed, after.ContextHash, "the stored context hash is the one the push stored")
}
