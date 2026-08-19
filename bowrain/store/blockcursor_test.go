package store_test

import (
	"fmt"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cursor the batch walk rides on: blocks come back in id order, and
// AfterID resumes past one. Tested against a real PostgreSQL because the
// ordering and the comparison are the database's, not Go's.
func TestBlockQuery_AfterIDWalksInIDOrder(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p1", Name: "Cursor", DefaultSourceLanguage: model.LocaleID("en"),
	}))

	const total = 250
	blocks := make([]*model.Block, 0, total)
	for i := range total {
		b := &model.Block{ID: fmt.Sprintf("blk%04d", i), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Line %d", i))
		blocks = append(blocks, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "en.json", blocks))

	// Walked with the helper, which is the way callers reach it.
	var seen []string
	require.NoError(t, platstore.EachBlockBatch(ctx, cs,
		platstore.BlockQuery{ProjectID: "p1", Stream: "main"}, 40,
		func(page []*venue.StoredBlock) error {
			assert.LessOrEqual(t, len(page), 40)
			for _, sb := range page {
				seen = append(seen, sb.Block.ID)
			}
			return nil
		}))

	require.Len(t, seen, total, "the walk visits every block exactly once")
	assert.True(t, sortedAscending(seen), "and visits them in id order")
	assert.Len(t, uniq(seen), total, "with no block visited twice")
}

// A cursor past the last id is the end of the walk, not an error and not a
// wrap-around.
func TestBlockQuery_AfterTheLastIDIsEmpty(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: "p1", Name: "Cursor", DefaultSourceLanguage: model.LocaleID("en"),
	}))
	b := &model.Block{ID: "only", Translatable: true}
	b.SetSourceText("Hello")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "en.json", []*model.Block{b}))

	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p1", Stream: "main"})
	require.NoError(t, err)
	require.Len(t, stored, 1)

	after, err := cs.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: "p1", Stream: "main", AfterID: stored[0].Block.ID,
	})
	require.NoError(t, err)
	assert.Empty(t, after)
}

func sortedAscending(ids []string) bool {
	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			return false
		}
	}
	return true
}

func uniq(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
