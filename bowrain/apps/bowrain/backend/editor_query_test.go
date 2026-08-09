package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desktop is offline in these tests, so every call takes the local-store
// path — the same filters, counts and review transitions the server answers
// when connected.

func TestQueryItemBlocksFiltersLocally(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	all, err := app.QueryItemBlocks(info.ID, itemName, EditorBlockFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3)

	page, err := app.QueryItemBlocks(info.ID, itemName, EditorBlockFilter{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, page, 2)

	matched, err := app.QueryItemBlocks(info.ID, itemName, EditorBlockFilter{Text: "welcome"})
	require.NoError(t, err)
	require.Len(t, matched, 1, "the substring is matched case-insensitively over the source")
	require.Len(t, matched[0].SourceRuns, 1)
	require.NotNil(t, matched[0].SourceRuns[0].Text)
	assert.Equal(t, "Welcome to neokapi", matched[0].SourceRuns[0].Text.Text)
}

func TestQueryItemBlocksFiltersByStatus(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	blocks, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	require.NoError(t, app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID: info.ID, ItemName: itemName, BlockID: blocks[0].ID,
		TargetLocale: "fr", Text: "Bonjour le monde",
	}))

	translated, err := app.QueryItemBlocks(info.ID, itemName, EditorBlockFilter{
		Locale: "fr", Status: "translated",
	})
	require.NoError(t, err)
	require.Len(t, translated, 1)
	assert.Equal(t, blocks[0].ID, translated[0].ID)

	notStarted, err := app.QueryItemBlocks(info.ID, itemName, EditorBlockFilter{
		Locale: "fr", Status: "not-started",
	})
	require.NoError(t, err)
	assert.Len(t, notStarted, 2)
}

func TestGetBlockCountsPartitionsByStatus(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	blocks, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)
	require.NoError(t, app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID: info.ID, ItemName: itemName, BlockID: blocks[0].ID,
		TargetLocale: "fr", Text: "Bonjour le monde",
	}))

	counts, err := app.GetBlockCounts(info.ID, itemName, EditorBlockFilter{Locale: "fr"})
	require.NoError(t, err)
	assert.Equal(t, 3, counts.Total)
	assert.Equal(t, "fr", counts.Locale)
	assert.Equal(t, 1, counts.Status.Translated)
	assert.Equal(t, counts.Translatable,
		counts.Status.NotStarted+counts.Status.Draft+counts.Status.Translated+counts.Status.Reviewed,
		"the buckets partition the translatable blocks")
}

func TestGetItemReportsTallies(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	item, err := app.GetItem(info.ID, itemName)
	require.NoError(t, err)
	assert.Equal(t, itemName, item.Name)
	assert.Equal(t, 3, item.BlockCount)
	assert.Equal(t, 3, item.Translatable)
}

func TestBulkReviewBlocksReportsEachBlock(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	blocks, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)
	require.Len(t, blocks, 3)
	for _, b := range blocks[:2] {
		require.NoError(t, app.UpdateBlockTarget(UpdateBlockRequest{
			ProjectID: info.ID, ItemName: itemName, BlockID: b.ID,
			TargetLocale: "fr", Text: "Bonjour",
		}))
	}

	// The third block has no fr target, so approving it refuses — in its own
	// result, without failing the two that can be approved.
	out, err := app.BulkReviewBlocks(info.ID, BulkReviewArgs{
		BlockIDs:     []string{blocks[0].ID, blocks[1].ID, blocks[2].ID},
		TargetLocale: "fr",
		Approve:      true,
		ItemName:     itemName,
	})
	require.NoError(t, err)
	require.Len(t, out.Results, 3)
	assert.Equal(t, 2, out.Succeeded)
	assert.Equal(t, 1, out.Failed)
	assert.NotEmpty(t, out.Results[2].Error)

	counts, err := app.GetBlockCounts(info.ID, itemName, EditorBlockFilter{Locale: "fr"})
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Status.Reviewed)
}

func TestBulkApplyMemorySkipsEveryBlockOffline(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	blocks, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)

	out, err := app.BulkApplyMemory(info.ID, BulkApplyMemoryArgs{
		BlockIDs:     []string{blocks[0].ID},
		TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Empty(t, out.Applied)
	require.Len(t, out.Skipped, 1)
	assert.Equal(t, "offline", out.Skipped[0].Reason)
}
