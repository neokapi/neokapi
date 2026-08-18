package sync

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// itemHashOf folds the blocks a client holds the way a push does — over record
// hashes, so an item is reported changed when anything the far side stores
// about one of its blocks has moved.
func itemHashOf(blocks ...*model.Block) string {
	hashes := make(map[string]string, len(blocks))
	for _, b := range blocks {
		hashes[b.ID] = model.ComputeIdentity(b).RecordHash()
	}
	return venue.ComputeItemHash(hashes)
}

// The whole point, end to end on the server's half: a kapi that starts
// recording a property about a block it has already pushed finds the item
// reported changed on its next ordinary push, and the block asked for. Nothing
// is declared, no version is bumped, and nobody forces anything.
func TestDiffEngine_ANewBlockPropertyMakesAnUnchangedItemChange(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()
	seedProject(t, cs, "proj-rec")

	before := &model.Block{ID: "b1", Translatable: true}
	before.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-rec", "src/App.jsx", []*model.Block{before})

	// The same text, from a reader that now records the runtime catalog key.
	after := &model.Block{ID: "b1", Translatable: true,
		Properties: map[string]string{"hash": "k1_abc"}}
	after.SetSourceText("Hello")

	itemDiff, err := engine.CompareItems(ctx, "proj-rec", "main", map[string]string{
		"src/App.jsx": itemHashOf(after),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"src/App.jsx"}, itemDiff.ChangedItems)
	assert.Zero(t, itemDiff.UnchangedCount)

	blockDiff, err := engine.CompareBlocks(ctx, "proj-rec", "main", "src/App.jsx", map[string]string{
		"b1": model.ComputeIdentity(after).RecordHash(),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"b1"}, blockDiff.Needed)
}

// And once it has arrived, the push after it is quiet: the re-upload happens
// once per enrichment, not on every push forever.
func TestDiffEngine_TheSameShapeIsUnchangedAgain(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()
	seedProject(t, cs, "proj-rec")

	stored := &model.Block{ID: "b1", Translatable: true,
		Properties: map[string]string{"hash": "k1_abc"}}
	stored.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-rec", "src/App.jsx", []*model.Block{stored})

	// The client scans its own file rather than reusing the seeded block:
	// storing one rewrites its ID to the server's internal one, and a push
	// keys its hashes by the reader's durable id.
	scanned := &model.Block{ID: "b1", Translatable: true,
		Properties: map[string]string{"hash": "k1_abc"}}
	scanned.SetSourceText("Hello")

	itemDiff, err := engine.CompareItems(ctx, "proj-rec", "main", map[string]string{
		"src/App.jsx": itemHashOf(scanned),
	})
	require.NoError(t, err)
	assert.Empty(t, itemDiff.ChangedItems)
	assert.Equal(t, 1, itemDiff.UnchangedCount)
}

// A locator is carried, never identifying: a block that moved down the file
// because something above it grew is not a block to re-upload.
func TestDiffEngine_AMovedLocatorIsNotAChange(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()
	seedProject(t, cs, "proj-rec")

	before := &model.Block{ID: "b1", Translatable: true,
		Properties: map[string]string{model.AdvisoryPropertyPrefix + "line": "4"}}
	before.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-rec", "notes.txt", []*model.Block{before})

	after := &model.Block{ID: "b1", Translatable: true,
		Properties: map[string]string{model.AdvisoryPropertyPrefix + "line": "91"}}
	after.SetSourceText("Hello")

	itemDiff, err := engine.CompareItems(ctx, "proj-rec", "main", map[string]string{
		"notes.txt": itemHashOf(after),
	})
	require.NoError(t, err)
	assert.Empty(t, itemDiff.ChangedItems)
	assert.Equal(t, 1, itemDiff.UnchangedCount)
}
