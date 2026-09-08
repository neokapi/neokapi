package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockTexts lists an item the way a surface does: in the order the file is
// read, which is what a person opening it expects to see.
func blockTexts(t *testing.T, deps *WorkerDeps, projectID, item string) []string {
	t.Helper()
	stored, err := deps.ContentStore.GetBlocks(t.Context(), store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  item,
		Order:     store.BlockOrderDocument,
	})
	require.NoError(t, err)
	texts := make([]string, len(stored))
	for i, sb := range stored {
		texts[i] = sb.Block.SourceText()
	}
	return texts
}

// A push declares the order it read a file in, and the venue lists the file
// that way. Ids are minted on push, so a listing left to them showed the
// heading of a document below the sections it introduces.
func TestAPushDeclaresItsDocumentOrder(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "order-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Order"}))

	const item = "about-us.html"
	// Sent in one order, read in another: the heading is the last block the
	// payload carries and the first the file holds.
	sent := []*pb.SyncBlock{
		{Id: "p-mission", ItemName: item, SourceText: "Our Mission", Translatable: true},
		{Id: "p-history", ItemName: item, SourceText: "Our History", Translatable: true},
		{Id: "h1-title", ItemName: item, SourceText: "About Acme Inc.", Translatable: true},
	}
	read := venue.Tree{item: venue.TreeItem{
		Path: item,
		Keys: []string{"h1-title", "p-mission", "p-history"},
	}}
	job := uploadPush(t, deps, "job-order-1", projectID, sent,
		map[string]any{"scope": []string{"**"}, "tree": read})
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, job.ID))

	assert.Equal(t, []string{"About Acme Inc.", "Our Mission", "Our History"},
		blockTexts(t, deps, projectID, item))

	// A second push that moves a paragraph and edits nothing carries no blocks
	// at all, so the declaration is the only thing that can say so.
	moved := venue.Tree{item: venue.TreeItem{
		Path: item,
		Keys: []string{"h1-title", "p-history", "p-mission"},
	}}
	second := uploadPush(t, deps, "job-order-2", projectID, nil,
		map[string]any{"scope": []string{"**"}, "tree": moved})
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	assert.Equal(t, []string{"About Acme Inc.", "Our History", "Our Mission"},
		blockTexts(t, deps, projectID, item))
}
