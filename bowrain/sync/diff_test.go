package sync

import (
	"maps"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDiffEngine(t *testing.T) (*DiffEngine, *bstore.PostgresStore) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	ss, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewDiffEngine(ss, nil), ss
}

func seedProject(t *testing.T, cs platstore.ContentStore, projectID string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID:                    projectID,
		Name:                  "Test",
		DefaultSourceLanguage: model.LocaleID("en"),
	}))
}

func seedBlocks(t *testing.T, cs platstore.ContentStore, projectID, itemName string, blocks []*model.Block) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", itemName, blocks))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{
		Name:     itemName,
		Format:   "json",
		ItemType: "file",
	}))
}

func TestDiffEngine_CompareItems_AllNew(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()

	seedProject(t, cs, "proj-1")

	// Client has items, server has none.
	clientHashes := map[string]string{
		"en.json":     "abc123",
		"messages.po": "def456",
	}

	result, err := engine.CompareItems(ctx, "proj-1", "main", clientHashes)
	require.NoError(t, err)
	assert.Len(t, result.NewItems, 2)
	assert.Empty(t, result.ChangedItems)
	assert.Empty(t, result.DeletedItems)
	assert.Equal(t, 0, result.UnchangedCount)
}

func TestDiffEngine_CompareItems_Unchanged(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()

	seedProject(t, cs, "proj-1")

	// Seed server with blocks.
	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-1", "en.json", []*model.Block{b1})

	// Compute what the server has.
	serverBlockHashes, err := engine.loadBlockHashes(ctx, "proj-1", "main", "en.json")
	require.NoError(t, err)
	serverItemHash := ComputeItemHash(serverBlockHashes)

	// Client sends matching hash.
	clientHashes := map[string]string{
		"en.json": serverItemHash,
	}

	result, err := engine.CompareItems(ctx, "proj-1", "main", clientHashes)
	require.NoError(t, err)
	assert.Empty(t, result.NewItems)
	assert.Empty(t, result.ChangedItems)
	assert.Empty(t, result.DeletedItems)
	assert.Equal(t, 1, result.UnchangedCount)
}

func TestDiffEngine_CompareItems_Changed(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()

	seedProject(t, cs, "proj-1")

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-1", "en.json", []*model.Block{b1})

	// Client sends a different hash (content changed locally).
	clientHashes := map[string]string{
		"en.json": "different-hash",
	}

	result, err := engine.CompareItems(ctx, "proj-1", "main", clientHashes)
	require.NoError(t, err)
	assert.Len(t, result.ChangedItems, 1)
	assert.Equal(t, "en.json", result.ChangedItems[0])
}

func TestDiffEngine_CompareItems_ServerHasExtra(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()

	seedProject(t, cs, "proj-1")

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-1", "en.json", []*model.Block{b1})

	// Client sends empty — server has en.json that client doesn't know about.
	result, err := engine.CompareItems(ctx, "proj-1", "main", map[string]string{})
	require.NoError(t, err)
	assert.Len(t, result.DeletedItems, 1)
	assert.Equal(t, "en.json", result.DeletedItems[0])
}

func TestDiffEngine_CompareBlocks(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()

	seedProject(t, cs, "proj-1")

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("World")
	seedBlocks(t, cs, "proj-1", "en.json", []*model.Block{b1, b2})

	// Load server hashes.
	serverHashes, err := engine.loadBlockHashes(ctx, "proj-1", "main", "en.json")
	require.NoError(t, err)
	require.Len(t, serverHashes, 2)

	// Client has b1 unchanged, b2 changed, b3 is new.
	clientHashes := map[string]string{}
	// copy server hashes
	maps.Copy(clientHashes, serverHashes)
	// Modify b2's hash to simulate a change.
	for id := range clientHashes {
		if id != "" { // just change the first one we find for testing
			clientHashes[id] = "modified-hash"
			break
		}
	}
	clientHashes["b3"] = "new-block-hash"

	result, err := engine.CompareBlocks(ctx, "proj-1", "main", "en.json", clientHashes)
	require.NoError(t, err)

	// b3 is new, one existing block was modified.
	assert.GreaterOrEqual(t, len(result.Needed), 1, "should have at least the modified block")
	assert.Contains(t, result.Needed, "b3", "new block should be needed")
}

func TestDiffEngine_RootHashFastPath(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	ctx := t.Context()

	seedProject(t, cs, "proj-1")

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	seedBlocks(t, cs, "proj-1", "en.json", []*model.Block{b1})

	// Compute server root hash.
	serverItemHashes, err := engine.loadItemHashes(ctx, "proj-1", "main")
	require.NoError(t, err)
	serverRoot := ComputeRootHash(serverItemHashes)

	// Matching root hash → unchanged.
	unchanged, err := engine.CheckRootHash(ctx, "proj-1", "main", serverRoot)
	require.NoError(t, err)
	assert.True(t, unchanged)

	// Different root hash → changed.
	unchanged, err = engine.CheckRootHash(ctx, "proj-1", "main", "different-root")
	require.NoError(t, err)
	assert.False(t, unchanged)
}
