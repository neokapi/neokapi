package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockWithText builds a translatable block carrying id and text.
func blockWithText(id, text string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(text)
	return b
}

// storedIdentity is the (source_id → internal id) view of an item's rows, which
// is what these tests are really asserting about.
func storedIdentity(t *testing.T, s *PostgresStore, projectID, itemName string) map[string]string {
	t.Helper()
	blocks, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: itemName, Limit: 100,
	})
	require.NoError(t, err)
	ids := make(map[string]string, len(blocks))
	for _, b := range blocks {
		ids[b.SourceID] = b.Block.ID
	}
	require.Len(t, ids, len(blocks), "every stored row should have a distinct source id")
	return ids
}

// TestStoreBlocksForItem_RoundTripKeepsIdentity is the store-level reproducer
// for #1527. A caller that reads an item's blocks back out of the store and
// writes them again — the extraction worker annotating an item, the editor
// writing a parsed item back — hands over blocks whose ID is the internal one
// this store minted. Storing those must update the rows they name, not mint a
// second copy of each under source_id = the first row's id.
func TestStoreBlocksForItem_RoundTripKeepsIdentity(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello, world!"),
		blockWithText("farewell", "Goodbye!"),
	}))
	first := storedIdentity(t, s, p.ID, "en.json")
	require.Len(t, first, 2)
	require.Contains(t, first, "greeting")
	require.Contains(t, first, "farewell")

	// Read back and write straight through, exactly as the extraction worker does.
	readBack, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 100,
	})
	require.NoError(t, err)
	roundTripped := make([]*model.Block, 0, len(readBack))
	for _, sb := range readBack {
		roundTripped = append(roundTripped, sb.Block)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", roundTripped))

	after := storedIdentity(t, s, p.ID, "en.json")
	assert.Len(t, after, 2, "a read-modify-write round trip must not duplicate the item's blocks")
	assert.Equal(t, first, after, "identity must survive the round trip unchanged")

	// And a subsequent push under the original client ids still lands on the
	// same rows rather than a third set.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello again!"),
		blockWithText("farewell", "Goodbye!"),
	}))
	assert.Equal(t, first, storedIdentity(t, s, p.ID, "en.json"),
		"a re-push after a round trip must still resolve to the same rows")
}

// TestStoreBlocksForItem_RepeatedTextStaysDistinct pins the other side of the
// identity choice: the key is the caller's block id scoped to the item, not the
// content hash. Two blocks that legitimately carry the same text are two
// occurrences and must stay two rows.
func TestStoreBlocksForItem_RepeatedTextStaysDistinct(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("title", "Open"),
		blockWithText("button", "Open"),
	}))

	ids := storedIdentity(t, s, p.ID, "en.json")
	assert.Len(t, ids, 2, "the same text under two client ids is two blocks")
	assert.Contains(t, ids, "title")
	assert.Contains(t, ids, "button")
	assert.NotEqual(t, ids["title"], ids["button"])
}

// TestStoreBlocksForItem_RepeatedStoreIsStable guards the plain case the round
// trip builds on: storing the same payload twice changes nothing.
func TestStoreBlocksForItem_RepeatedStoreIsStable(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	payload := func() []*model.Block {
		return []*model.Block{
			blockWithText("greeting", "Hello, world!"),
			blockWithText("farewell", "Goodbye!"),
		}
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", payload()))
	before := storedIdentity(t, s, p.ID, "en.json")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", payload()))

	assert.Equal(t, before, storedIdentity(t, s, p.ID, "en.json"))
}
