package sync

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tree is the venue's content, and an item row holding no blocks is not
// content. Two things mint one: a decision naming a file the venue has no
// content for, and a removal that leaves the item row standing to anchor the
// decisions it holds.
//
// Serving either in the tree reads to a producer, and to the identity plan,
// as a file the declaration dropped. The plan would remove it, taking the
// decisions it anchors, which is the removal this whole rule exists to stop.
func TestLoadTree_OmitsAnItemWithNoBlocks(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()
	seedProject(t, cs, "proj-tree")

	b := model.NewBlock("greeting", "Hello")
	b.Translatable = true
	seedBlocks(t, cs, "proj-tree", "en.json", []*model.Block{b})
	require.NoError(t, cs.StoreItem(ctx, "proj-tree", "main", &platstore.Item{
		Name: "gone.json", ItemType: "file",
	}))

	tree, err := engine.LoadTree(ctx, "proj-tree", "main", nil)
	require.NoError(t, err)

	assert.Contains(t, tree, "en.json", "content is the tree")
	assert.NotContains(t, tree, "gone.json",
		"an item holding no blocks is not a file the producer is missing")
}
