package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
)

// The positional cursor against a real PostgreSQL, because the comparison it
// rests on is the database's row-value ordering rather than anything Go does
// with the rows afterwards.

const positionCursorItem = "handbook.md"

// positionCursorStore seeds one item under the names given and hands back the
// store and the project it sits in.
func positionCursorStore(t *testing.T, names ...string) (*bstore.PostgresStore, string) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p1", Name: "Neighbourhood", DefaultSourceLanguage: model.LocaleID("en"),
	}))
	blocks := make([]*model.Block, 0, len(names))
	for _, name := range names {
		b := &model.Block{Name: name, Translatable: true}
		b.SetSourceText(name)
		blocks = append(blocks, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", positionCursorItem, blocks))
	return cs, "p1"
}

// pgNames reads a query and names the blocks it answered with.
func pgNames(t *testing.T, cs *bstore.PostgresStore, q platstore.BlockQuery) []string {
	t.Helper()
	q.ProjectID, q.Stream, q.ItemName = "p1", "main", positionCursorItem
	stored, err := cs.GetBlocks(t.Context(), q)
	require.NoError(t, err)
	names := make([]string, len(stored))
	for i, sb := range stored {
		names[i] = sb.Block.Name
	}
	return names
}

// pgIDOf reads the id the store minted for one of the item's blocks.
func pgIDOf(t *testing.T, cs *bstore.PostgresStore, name string) string {
	t.Helper()
	stored, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: "p1", Stream: "main", ItemName: positionCursorItem,
	})
	require.NoError(t, err)
	for _, sb := range stored {
		if sb.Block.Name == name {
			return sb.Block.ID
		}
	}
	t.Fatalf("item %q holds no block named %q", positionCursorItem, name)
	return ""
}

// TestDocumentCursor_ReadsTheDocument is the defect this exists for: the review
// neighbourhood asked for the blocks either side of a unit and was answered
// with the unit's id neighbours, which on a file stored in any order but its
// reading order are two other paragraphs.
func TestDocumentCursor_ReadsTheDocument(t *testing.T) {
	cs, pid := positionCursorStore(t, "a", "b", "c", "d", "e")
	ctx := t.Context()

	document := []string{"c", "e", "a", "d", "b"}
	require.NoError(t, cs.SetBlockOrder(ctx, pid, "main", positionCursorItem, document))
	require.Equal(t, document, pgNames(t, cs, platstore.BlockQuery{Order: platstore.BlockOrderDocument}))

	stored := pgNames(t, cs, platstore.BlockQuery{})
	require.NotEqual(t, document, stored, "the case needs the two orders to differ")

	anchor := pgIDOf(t, cs, "a")

	assert.Equal(t, []string{"e"}, pgNames(t, cs, platstore.BlockQuery{
		DocumentBefore: anchor, Limit: 1,
	}), "the nearest predecessor in the document")
	assert.Equal(t, []string{"d"}, pgNames(t, cs, platstore.BlockQuery{
		DocumentAfter: anchor, Limit: 1,
	}), "the nearest successor in the document")

	// A wider window arrives in document order on both sides, so a surface
	// reading before, unit, after top to bottom reads the file.
	assert.Equal(t, []string{"c", "e"}, pgNames(t, cs, platstore.BlockQuery{
		DocumentBefore: anchor, Limit: 2,
	}), "the window's worth, nearest last")
	assert.Equal(t, []string{"d", "b"}, pgNames(t, cs, platstore.BlockQuery{
		DocumentAfter: anchor, Limit: 2,
	}), "the window's worth, nearest first")

	// The ends of the document are an empty answer rather than an error.
	assert.Empty(t, pgNames(t, cs, platstore.BlockQuery{
		DocumentBefore: pgIDOf(t, cs, "c"), Limit: 2,
	}), "the first block of the document has no predecessor")
	assert.Empty(t, pgNames(t, cs, platstore.BlockQuery{
		DocumentAfter: pgIDOf(t, cs, "b"), Limit: 2,
	}), "the last block of the document has no successor")

	// An anchor the item does not hold has no neighbourhood, rather than the
	// item's first blocks read as one.
	assert.Empty(t, pgNames(t, cs, platstore.BlockQuery{
		DocumentAfter: "no-such-block", Limit: 2,
	}))
}

// TestDocumentCursor_UnplacedRowsKeepTheirOrder: an item nothing has placed
// carries position 0 throughout, so the cursor falls back to the id tiebreak
// the listing already sorts on and reads the same sequence a listing shows.
func TestDocumentCursor_UnplacedRowsKeepTheirOrder(t *testing.T) {
	cs, _ := positionCursorStore(t, "x", "y", "z")

	stored := pgNames(t, cs, platstore.BlockQuery{})
	require.Equal(t, stored, pgNames(t, cs, platstore.BlockQuery{Order: platstore.BlockOrderDocument}))

	anchor := pgIDOf(t, cs, stored[1])
	assert.Equal(t, stored[:1], pgNames(t, cs, platstore.BlockQuery{
		DocumentBefore: anchor, Limit: 2,
	}))
	assert.Equal(t, stored[2:], pgNames(t, cs, platstore.BlockQuery{
		DocumentAfter: anchor, Limit: 2,
	}))
}

// TestDocumentCursor_YieldsToTheWalk guards the walk: an id cursor set
// alongside a positional one is the one that applies, because a page ordered by
// anything but the walking cursor's own key would visit a block twice or not at
// all.
func TestDocumentCursor_YieldsToTheWalk(t *testing.T) {
	cs, pid := positionCursorStore(t, "a", "b", "c", "d")
	ctx := t.Context()
	require.NoError(t, cs.SetBlockOrder(ctx, pid, "main", positionCursorItem,
		[]string{"d", "c", "b", "a"}))

	stored := pgNames(t, cs, platstore.BlockQuery{})
	first := pgIDOf(t, cs, stored[0])
	assert.Equal(t, stored[1:], pgNames(t, cs, platstore.BlockQuery{
		AfterID: first, DocumentBefore: first,
	}), "the id cursor applies and the positional one is dropped")
}
