package jobs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
)

// What a producer decided outlives the file it decided about.
//
// A committed record holds decisions for files the checkout no longer has, and
// a producer sends that record only when its fold moves. So a push that carries
// no record is not a producer saying the decisions are gone, and a declared tree
// that does not name a file is not either: it says where the content is, and
// nothing about the record.
//
// Removing those rows lost them for good. The push that removed them carried
// no record to put them back, the next push carried none either, and the file
// coming back found no approval.

// decisionsFor is the ledger rows one item holds on the main stream.
func decisionsFor(t *testing.T, deps *WorkerDeps, projectID, itemName string) []venue.UnitDecision {
	t.Helper()
	held, err := deps.ContentStore.(store.DecisionStore).ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	var out []venue.UnitDecision
	for _, d := range held {
		if d.ItemName == itemName {
			out = append(out, d)
		}
	}
	return out
}

// approvalFor is a committed record holding one approval for a unit of itemName.
func approvalFor(t *testing.T, itemName string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{
		"item": itemName, "unit": "u1", "variant": "nb",
		"status": "established", "targetHash": "sha256:t1",
		"reviewState": "approved", "by": "ana", "at": "2026-08-06T13:45:44Z",
		"updated": "2026-08-06T13:45:44Z",
	}})
	require.NoError(t, err)
	return raw
}

// A decision naming a file the venue holds no content for mints an item row for
// it. The next push declares a tree without that file, because the checkout
// does not have it, and carries no record, because the record has not changed.
func TestAPushCarryingNoRecordKeepsADecisionForAFileTheCheckoutDoesNotHold(t *testing.T) {
	deps := newTestWorkerDeps(t)
	deps.ReviewAuthority = pushAuthority{review: map[string]bool{"nb": true}}
	ctx := t.Context()

	projectID := "decision-survives-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Decision survives"}))

	blocks := []*pb.SyncBlock{{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true}}
	declaration := with(declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"en.json": blocks})),
		map[string]any{"actor_id": "u-reviewer"})

	first := uploadPush(t, deps, "job-survives-1", projectID, blocks,
		with(declaration, map[string]any{"decisions": approvalFor(t, "gone.json")}))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))
	require.Len(t, decisionsFor(t, deps, projectID, "gone.json"), 1, "the approval is held")

	second := uploadPush(t, deps, "job-survives-2", projectID, nil, declaration)
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	assert.Len(t, decisionsFor(t, deps, projectID, "gone.json"), 1,
		"a push that carries no record does not say the decision is gone")
}

// The same rule where the venue does hold the content: the file is deleted in
// the checkout after its record was sent, so the declaration removes it and the
// push carries no record. The content goes, the ledger keeps the approval, and
// content pushed to that path again lands on the item still holding it.
//
// What the returning target shows is the projection's business, and this
// asserts nothing about it.
func TestARemovedFileKeepsItsDecisionsWhenItsContentReturns(t *testing.T) {
	deps := newTestWorkerDeps(t)
	deps.ReviewAuthority = pushAuthority{review: map[string]bool{"nb": true}}
	ctx := t.Context()

	projectID := "decision-returns-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Decision returns"}))

	kept := []*pb.SyncBlock{{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true}}
	removed := []*pb.SyncBlock{{Id: "u1", ItemName: "doomed.json", SourceText: "Goodbye", Translatable: true}}
	both := treeOf(map[string][]*pb.SyncBlock{"en.json": kept, "doomed.json": removed})

	first := uploadPush(t, deps, "job-returns-1", projectID, append(append([]*pb.SyncBlock{}, kept...), removed...),
		with(with(declaring([]string{"**"}, both), map[string]any{"actor_id": "u-reviewer"}),
			map[string]any{"decisions": approvalFor(t, "doomed.json")}))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))
	require.Len(t, decisionsFor(t, deps, projectID, "doomed.json"), 1, "the approval is held")

	withoutIt := with(declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"en.json": kept})),
		map[string]any{"actor_id": "u-reviewer"})
	second := uploadPush(t, deps, "job-returns-2", projectID, nil, withoutIt)
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	gone, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "doomed.json", Limit: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, gone, "the content the declaration dropped is removed")
	assert.Len(t, decisionsFor(t, deps, projectID, "doomed.json"), 1,
		"and the approval the record still holds stays")

	third := uploadPush(t, deps, "job-returns-3", projectID, removed,
		with(declaring([]string{"**"}, both), map[string]any{"actor_id": "u-reviewer"}))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, third.ID))

	back, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "doomed.json", Limit: 10,
	})
	require.NoError(t, err)
	assert.Len(t, back, 1, "the file comes back")
	assert.Len(t, decisionsFor(t, deps, projectID, "doomed.json"), 1,
		"and the ledger still holds its approval")
}
