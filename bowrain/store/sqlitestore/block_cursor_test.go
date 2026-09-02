package sqlitestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// TestGetBlocks_BeforeID walks the keyset cursor backwards: "the block that
// precedes this one" must cost one row wherever the block sits, and must name
// the NEAREST predecessor rather than the item's first block. Selecting
// ascending with a LIMIT would return the latter, silently, for every block
// past the second.
func TestGetBlocks_BeforeID(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	blocks := make([]*model.Block, 0, 4)
	for i, text := range []string{"one", "two", "three", "four"} {
		b := &model.Block{ID: []string{"b1", "b2", "b3", "b4"}[i], Translatable: true}
		b.SetSourceText(text)
		blocks = append(blocks, b)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "messages.json", blocks))

	// The store mints its own ids, and the cursor orders by them — so the
	// sequence under test is the stored order, read back.
	all, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, ItemName: "messages.json"})
	require.NoError(t, err)
	require.Len(t, all, 4)
	ids := make([]string, len(all))
	for i, sb := range all {
		ids[i] = sb.Block.ID
	}

	before, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: "messages.json", BeforeID: ids[2], Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, before, 1)
	assert.Equal(t, ids[1], before[0].Block.ID, "the nearest predecessor, not the item's first block")

	after, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: "messages.json", AfterID: ids[2], Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, ids[3], after[0].Block.ID)

	// The first block has no predecessor, which is an empty answer rather than
	// an error: a surface draws "start of the item" from it.
	none, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: "messages.json", BeforeID: ids[0], Limit: 1,
	})
	require.NoError(t, err)
	assert.Empty(t, none)

	// A wider window still arrives in ascending id order, so a caller hands it
	// to the same projection an AfterID page goes through.
	window, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: "messages.json", BeforeID: ids[3], Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, window, 2)
	assert.Equal(t, []string{ids[1], ids[2]}, []string{window[0].Block.ID, window[1].Block.ID})
}
