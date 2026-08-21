package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole point of keying governance on an item's identity rather than its
// address: `git mv` a file and the record of who approved what survives.
//
// Every other row a project holds can be re-derived from a push. The decision
// ledger cannot — it is the one thing a reset genuinely loses — so it is the
// one this has to be proved on, at the level a person would check it: read the
// approval back at the file's NEW path, after a push that carried no blocks at
// all because the content did not change.
func TestRenamingAFileKeepsItsApprovals(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "approvals-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Approvals"}))

	blocks := []*pb.SyncBlock{
		{Id: "greeting", Name: "greeting", ItemName: "old/name.json", SourceText: "Hello", Translatable: true},
		{Id: "farewell", Name: "farewell", ItemName: "old/name.json", SourceText: "Goodbye", Translatable: true},
	}
	first := uploadPush(t, deps, "job-approval-1", projectID, blocks, declaring(
		[]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"old/name.json": blocks})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))

	ledger, ok := deps.ContentStore.(store.DecisionStore)
	require.True(t, ok)

	// Somebody approves a translation of one of its strings.
	approval := venue.UnitDecision{
		ItemName:    "old/name.json",
		Unit:        "greeting",
		Variant:     "nb",
		Status:      "approved",
		ReviewState: "approved",
		DecidedBy:   "reviewer@example.com",
		DecidedAt:   "2026-08-20T10:00:00Z",
		Note:        "checked against the style guide",
		Updated:     "2026-08-20T10:00:00Z",
	}
	applied, err := ledger.UpsertUnitDecisions(ctx, projectID, "main", []venue.UnitDecision{approval})
	require.NoError(t, err)
	require.Equal(t, 1, applied)

	// The file is renamed. The content did not change, so the push carries no
	// blocks — only the declaration says anything happened at all.
	moved := []*pb.SyncBlock{
		{Id: "greeting", Name: "greeting", ItemName: "new/name.json", SourceText: "Hello", Translatable: true},
		{Id: "farewell", Name: "farewell", ItemName: "new/name.json", SourceText: "Goodbye", Translatable: true},
	}
	second := uploadPush(t, deps, "job-approval-2", projectID, nil, declaring(
		[]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"new/name.json": moved})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	require.Equal(t, []string{"new/name.json"}, itemNames(t, deps, projectID),
		"a rename leaves one item, not two")

	after, err := ledger.ListUnitDecisions(ctx, projectID, "main")
	require.NoError(t, err)
	require.Len(t, after, 1, "the approval survives the move")

	got := after[0]
	assert.Equal(t, "new/name.json", got.ItemName,
		"and is readable at the path the file now sits at")
	assert.Equal(t, "greeting", got.Unit)
	assert.Equal(t, "nb", got.Variant)
	assert.Equal(t, "approved", got.Status)
	assert.Equal(t, "reviewer@example.com", got.DecidedBy,
		"who approved it is the fact the ledger exists to keep")
	assert.Equal(t, "checked against the style guide", got.Note)
}
