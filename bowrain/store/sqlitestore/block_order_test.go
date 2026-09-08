package sqlitestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// docBlocks builds one item's blocks. Ids are minted by the store, so the order
// a listing used to come back in was neither the order these were written nor
// the order they are read in.
func docBlocks(names ...string) []*model.Block {
	blocks := make([]*model.Block, 0, len(names))
	for _, name := range names {
		b := &model.Block{Name: name, Translatable: true}
		b.SetSourceText(name)
		blocks = append(blocks, b)
	}
	return blocks
}

func namesOf(t *testing.T, s *SQLiteStore, q platstore.BlockQuery) []string {
	t.Helper()
	stored, err := s.GetBlocks(t.Context(), q)
	require.NoError(t, err)
	names := make([]string, len(stored))
	for i, sb := range stored {
		names[i] = sb.Block.Name
	}
	return names
}

func listed(t *testing.T, s *SQLiteStore, projectID, item string) []string {
	t.Helper()
	return namesOf(t, s, platstore.BlockQuery{
		ProjectID: projectID, ItemName: item, Order: platstore.BlockOrderDocument,
	})
}

func byID(t *testing.T, s *SQLiteStore, projectID, item string) []string {
	t.Helper()
	return namesOf(t, s, platstore.BlockQuery{ProjectID: projectID, ItemName: item})
}

// TestSetBlockOrder_ListsInDocumentOrder is the defect this exists for: a
// listing showed a file in the order its rows happened to be stored, so a
// seeded heading landed under the sections it introduces.
func TestSetBlockOrder_ListsInDocumentOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "about-us.html"

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item,
		docBlocks("p-mission", "p-history", "h1-title")))

	// Nothing has placed these, so the listing is the id order it always was.
	assert.Equal(t, byID(t, s, p.ID, item), listed(t, s, p.ID, item),
		"an unplaced item keeps the order it had")

	// The heading opens the document.
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item,
		[]string{"h1-title", "p-mission", "p-history"}))
	assert.Equal(t, []string{"h1-title", "p-mission", "p-history"}, listed(t, s, p.ID, item))

	// A later push moves a paragraph without editing it.
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item,
		[]string{"h1-title", "p-history", "p-mission"}))
	assert.Equal(t, []string{"h1-title", "p-history", "p-mission"}, listed(t, s, p.ID, item))

	// Restating the same order writes nothing and changes nothing.
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item,
		[]string{"h1-title", "p-history", "p-mission"}))
	assert.Equal(t, []string{"h1-title", "p-history", "p-mission"}, listed(t, s, p.ID, item))
}

// TestSetBlockOrder_KeysetPagingStaysIDOrdered guards the cursor: a walk is a
// keyset over ids, and ordering its pages by position would make the same block
// arrive twice or not at all.
func TestSetBlockOrder_KeysetPagingStaysIDOrdered(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "guide.md"

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item, docBlocks("a", "b", "c", "d")))
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item, []string{"d", "c", "b", "a"}))

	assert.Equal(t, []string{"d", "c", "b", "a"}, listed(t, s, p.ID, item),
		"the listing reads the document")

	want := byID(t, s, p.ID, item)
	var walked []string
	cursor := ""
	for {
		page, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: p.ID, ItemName: item, AfterID: cursor, Limit: 2,
		})
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, sb := range page {
			walked = append(walked, sb.Block.Name)
			cursor = sb.Block.ID
		}
	}
	assert.Equal(t, want, walked, "the walk visits every block once, in id order")

	// The backwards cursor still names the nearest predecessor by id.
	all, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, ItemName: item})
	require.NoError(t, err)
	require.Len(t, all, 4)
	before, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, BeforeID: all[2].Block.ID, Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, before, 1)
	assert.Equal(t, all[1].Block.ID, before[0].Block.ID)
}

// TestSetBlockOrder_LeavesUnnamedRowsAlone: an order that names only some of an
// item's blocks places those and leaves the rest where they were, and a key the
// item has no row for is skipped rather than failing the push.
func TestSetBlockOrder_LeavesUnnamedRowsAlone(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "notes.md"

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item, docBlocks("x", "y", "z")))
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item, []string{"z", "gone", "y"}))

	// z takes position 1 and y position 3; x was never named, so it keeps
	// position 0 and sorts ahead of both.
	assert.Equal(t, []string{"x", "z", "y"}, listed(t, s, p.ID, item))
}

// TestSetBlockOrder_NoItemOrKeys is a no-op rather than an error: an item-less
// write and an item that read to nothing both reach it.
func TestSetBlockOrder_NoItemOrKeys(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", "", []string{"a"}))
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", "empty.md", nil))
}
