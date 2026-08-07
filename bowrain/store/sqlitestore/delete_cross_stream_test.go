package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeItemBlock registers an item and stores one source block under it,
// returning the block's stored (internal) id. StoreBlocksForItem maps the
// caller's id to source_id and mints an internal id, so the id is read back
// rather than assumed.
func storeItemBlock(t *testing.T, s *SQLiteStore, projectID, stream, itemName, sourceID, source string, targets map[model.LocaleID]string) string {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.StoreItem(ctx, projectID, stream, &platstore.Item{Name: itemName, Format: "json"}))
	b := model.NewBlock(sourceID, source)
	for locale, text := range targets {
		b.SetTargetText(locale, text)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, projectID, stream, itemName, []*model.Block{b}))
	return blockIDForSource(t, s, projectID, stream, itemName, sourceID)
}

func blockIDForSource(t *testing.T, s *SQLiteStore, projectID, stream, itemName, sourceID string) string {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: stream, ItemName: itemName, Limit: 100,
	})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == sourceID {
			return sb.Block.ID
		}
	}
	return ""
}

// TestDeleteItem_BranchDeleteKeepsMainBlocks pins the cross-stream data-loss
// fix: streams share block rows (CreateStream copies items, not blocks), so
// removing a file on a branch must not destroy main's source blocks or targets.
func TestDeleteItem_BranchDeleteKeepsMainBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// main holds one item with a source block and an approved French target.
	id := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Hello",
		map[model.LocaleID]string{model.LocaleFrench: "Bonjour"})
	require.NotEmpty(t, id)

	// Branch off main; the branch gets its own item row over the shared blocks.
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{ProjectID: p.ID, Name: "feature", Parent: "main"}))

	// Removing the file on the branch must leave main untouched.
	require.NoError(t, s.DeleteItem(ctx, p.ID, "feature", "file.json"))

	got, err := s.GetBlock(ctx, p.ID, "main", id)
	require.NoError(t, err, "main's block must survive a branch-side delete")
	assert.Equal(t, "Hello", got.Block.SourceText())
	assert.Equal(t, "Bonjour", got.Block.TargetText(model.LocaleFrench),
		"main's approved target must survive a branch-side delete")
}

// TestDeleteItem_LastStreamReclaimsBlocks: once no stream references the item,
// the shared blocks are reclaimed.
func TestDeleteItem_LastStreamReclaimsBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	id := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Hello", nil)
	require.NotEmpty(t, id)

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "file.json"))

	_, err := s.GetBlock(ctx, p.ID, "main", id)
	require.Error(t, err, "the last stream's delete reclaims the shared block")
}

// TestDeleteItem_NoTargetResurrection is the finding-2 guard: deleting an item
// clears its per-stream overlays, so re-pushing the same block id under new
// source text must not resurrect the old target.
func TestDeleteItem_NoTargetResurrection(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Hello",
		map[model.LocaleID]string{model.LocaleFrench: "Bonjour"})

	require.NoError(t, s.DeleteItem(ctx, p.ID, "main", "file.json"))

	// Re-push the same source id with different source text and no target.
	id := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Goodbye", nil)
	require.NotEmpty(t, id)

	got, err := s.GetBlock(ctx, p.ID, "main", id)
	require.NoError(t, err)
	assert.Equal(t, "Goodbye", got.Block.SourceText())
	assert.Empty(t, got.Block.TargetText(model.LocaleFrench),
		"a stale target must not resurrect onto new source text")
}

// TestDeleteBlock_ClearsTargets is the finding-2 guard for the single-block
// path: deleting a block removes its targets so a re-push starts clean.
func TestDeleteBlock_ClearsTargets(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	id := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Hello",
		map[model.LocaleID]string{model.LocaleFrench: "Bonjour"})
	require.NotEmpty(t, id)

	require.NoError(t, s.DeleteBlock(ctx, p.ID, "main", id))

	newID := storeItemBlock(t, s, p.ID, "main", "file.json", "greeting", "Goodbye", nil)
	require.NotEmpty(t, newID)

	got, err := s.GetBlock(ctx, p.ID, "main", newID)
	require.NoError(t, err)
	assert.Empty(t, got.Block.TargetText(model.LocaleFrench),
		"a deleted block's target must not resurrect on re-push")
}
