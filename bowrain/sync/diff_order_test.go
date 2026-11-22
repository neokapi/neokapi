package sync

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The push diff names items and blocks in sorted order, the same on every
// call: its lists go to the client in the push/init response.
func TestDiffEngineListsAreSorted(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()
	seedProject(t, cs, "proj-1")
	for _, item := range []string{"s4.json", "s1.json", "s3.json", "s2.json"} {
		seedBlocks(t, cs, "proj-1", item, []*model.Block{model.NewBlock("k", "server "+item)})
	}
	seedBlocks(t, cs, "proj-1", "blocks.json", []*model.Block{
		model.NewBlock("x4", "a"), model.NewBlock("x1", "b"), model.NewBlock("x3", "c"), model.NewBlock("x2", "d"),
	})

	client := map[string]string{
		"n3.json": "h", "n1.json": "h", "n2.json": "h", "n4.json": "h",
		"s3.json": "changed", "s1.json": "changed", "s4.json": "changed", "s2.json": "changed",
	}
	clientBlocks := map[string]string{"y3": "h", "y1": "h", "y4": "h", "y2": "h"}

	for range 20 {
		items, err := engine.CompareItems(ctx, "proj-1", "main", client)
		require.NoError(t, err)
		assert.Equal(t, []string{"n1.json", "n2.json", "n3.json", "n4.json"}, items.NewItems)
		assert.Equal(t, []string{"s1.json", "s2.json", "s3.json", "s4.json"}, items.ChangedItems)
		assert.Equal(t, []string{"blocks.json"}, items.DeletedItems)

		blocks, err := engine.CompareBlocks(ctx, "proj-1", "main", "blocks.json", clientBlocks)
		require.NoError(t, err)
		assert.Equal(t, []string{"y1", "y2", "y3", "y4"}, blocks.Needed)
		assert.Equal(t, []string{"x1", "x2", "x3", "x4"}, blocks.Deleted)
	}
}
