package sqlitestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
)

// idOf reads the id the store minted for the block an item declares under name.
func idOf(t *testing.T, s *SQLiteStore, projectID, item, name string) string {
	t.Helper()
	stored, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, ItemName: item, Order: platstore.BlockOrderDocument,
	})
	require.NoError(t, err)
	for _, sb := range stored {
		if sb.Block.Name == name {
			return sb.Block.ID
		}
	}
	t.Fatalf("item %q holds no block named %q", item, name)
	return ""
}

// aroundIn names the two blocks either side of one in a sequence, empty at
// either end.
func aroundIn(seq []string, name string) (before, after string) {
	for i, n := range seq {
		if n != name {
			continue
		}
		if i > 0 {
			before = seq[i-1]
		}
		if i+1 < len(seq) {
			after = seq[i+1]
		}
		return before, after
	}
	return "", ""
}

// TestDocumentCursor_ReadsTheDocument is the defect this exists for: the review
// neighbourhood asked for the blocks either side of a unit and was answered
// with the unit's id neighbours, which on a file stored in any order but its
// reading order are two other paragraphs.
func TestDocumentCursor_ReadsTheDocument(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "handbook.md"

	// The store mints the ids, so the order it stored these in is read back
	// rather than assumed. Reverse that order so both neighbours of the middle
	// block must differ, even when the random ids happen to match a fixed order.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item,
		docBlocks("a", "b", "c", "d", "e")))
	stored := byID(t, s, p.ID, item)
	require.Len(t, stored, 5)
	document := []string{stored[4], stored[3], stored[2], stored[1], stored[0]}
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item, document))
	require.Equal(t, document, listed(t, s, p.ID, item))
	require.NotEqual(t, document, stored, "the case needs the two orders to differ")

	anchor := idOf(t, s, p.ID, item, document[2])

	// One block each way is the document's neighbour, and the id order names
	// other blocks there.
	idBefore, idAfter := aroundIn(stored, document[2])
	require.NotEqual(t, []string{document[1], document[3]}, []string{idBefore, idAfter},
		"the case needs the id neighbours to differ from the document's")

	before := namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentBefore: anchor, Limit: 1,
	})
	assert.Equal(t, document[1:2], before, "the nearest predecessor in the document")

	after := namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentAfter: anchor, Limit: 1,
	})
	assert.Equal(t, document[3:4], after, "the nearest successor in the document")

	// A wider window arrives in document order on both sides, so a surface
	// reading before, unit, after top to bottom reads the file.
	assert.Equal(t, document[:2], namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentBefore: anchor, Limit: 2,
	}), "the window's worth, nearest last")
	assert.Equal(t, document[3:], namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentAfter: anchor, Limit: 2,
	}), "the window's worth, nearest first")

	// The ends of the document are an empty answer rather than an error: a
	// surface draws "start of the item" from it.
	assert.Empty(t, namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item,
		DocumentBefore: idOf(t, s, p.ID, item, document[0]), Limit: 2,
	}), "the first block of the document has no predecessor")
	assert.Empty(t, namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item,
		DocumentAfter: idOf(t, s, p.ID, item, document[4]), Limit: 2,
	}), "the last block of the document has no successor")

	// An anchor the item does not hold has no neighbourhood, rather than the
	// item's first blocks read as one.
	assert.Empty(t, namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentAfter: "no-such-block", Limit: 2,
	}))
}

// TestDocumentCursor_UnplacedRowsKeepTheirOrder: an item nothing has placed
// carries position 0 throughout, so the cursor falls back to the id tiebreak
// the listing already sorts on and reads the same sequence a listing shows.
func TestDocumentCursor_UnplacedRowsKeepTheirOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "unplaced.md"

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item, docBlocks("x", "y", "z")))
	stored := byID(t, s, p.ID, item)
	require.Equal(t, stored, listed(t, s, p.ID, item))

	anchor := idOf(t, s, p.ID, item, stored[1])
	assert.Equal(t, stored[:1], namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentBefore: anchor, Limit: 2,
	}))
	assert.Equal(t, stored[2:], namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentAfter: anchor, Limit: 2,
	}))
}

// TestDocumentCursor_PlacedAndUnplacedTogether: an order that names only some
// of an item's rows leaves the rest at position 0, ahead of everything placed.
// The cursor reads that combined sequence rather than treating an unplaced row
// as absent.
func TestDocumentCursor_PlacedAndUnplacedTogether(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "partial.md"

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item, docBlocks("k", "l", "m")))
	// "l" is never named, so it keeps position 0 and opens the item.
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item, []string{"m", "k"}))
	require.Equal(t, []string{"l", "m", "k"}, listed(t, s, p.ID, item))

	anchor := idOf(t, s, p.ID, item, "m")
	assert.Equal(t, []string{"l"}, namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentBefore: anchor, Limit: 2,
	}), "an unplaced row is a neighbour like any other")
	assert.Equal(t, []string{"k"}, namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentAfter: anchor, Limit: 2,
	}))
}

// TestDocumentCursor_YieldsToTheWalk guards the walk: an id cursor set
// alongside a positional one is the one that applies, because a page ordered by
// anything but the walking cursor's own key would visit a block twice or not at
// all.
func TestDocumentCursor_YieldsToTheWalk(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	const item = "walked.md"

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", item, docBlocks("a", "b", "c", "d")))
	require.NoError(t, s.SetBlockOrder(ctx, p.ID, "", item, []string{"d", "c", "b", "a"}))

	stored := byID(t, s, p.ID, item)
	first := idOf(t, s, p.ID, item, stored[0])
	assert.Equal(t, stored[1:], namesOf(t, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, AfterID: first, DocumentBefore: first,
	}), "the id cursor applies and the positional one is dropped")

	// EachBlockBatch drops it for the same reason, so a scope carrying one
	// still walks the whole item.
	var walked []string
	require.NoError(t, platstore.EachBlockBatch(ctx, s, platstore.BlockQuery{
		ProjectID: p.ID, ItemName: item, DocumentAfter: first,
	}, 2, func(page []*venue.StoredBlock) error {
		for _, sb := range page {
			walked = append(walked, sb.Block.Name)
		}
		return nil
	}))
	assert.Equal(t, stored, walked)
}
