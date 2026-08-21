package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	corestorage "github.com/neokapi/neokapi/core/storage"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// uploadPush stages one push's chunk and manifest and returns the job that will
// apply it, so a test can say what a push carries rather than how it is packed.
func uploadPush(t *testing.T, deps *WorkerDeps, jobID, projectID string, blocks []*pb.SyncBlock, extra map[string]any) *TranslationJob {
	t.Helper()
	ctx := t.Context()

	chunk := &pb.SyncChunk{ContentType: "blocks", RecordCount: int32(len(blocks)), Blocks: blocks}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)
	rawHash := sha256.Sum256(chunkData)
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	items := []map[string]string{{"name": "en.json", "format": "json"}}
	itemsJSON, _ := json.Marshal(items)
	manifest := map[string]any{
		"upload_id":  jobID,
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         hex.EncodeToString(rawHash[:]),
			"record_count": len(blocks),
			"byte_size":    len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	}
	maps.Copy(manifest, extra)
	manifestData, _ := json.Marshal(manifest)
	manifestRef, err := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})
	require.NoError(t, err)

	job := &TranslationJob{
		ID:        jobID,
		ProjectID: projectID,
		ItemName:  "__sync_push__",
		Model:     manifestRef.Key,
		PushID:    jobID,
		Status:    StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))
	return job
}

// A push that fails partway leaves the project exactly as it found it.
//
// The failure is a real one from the protocol rather than an injected panic:
// the push carries content AND a decisions payload whose expected ledger ref
// does not match. The blocks are written first and the assertion fails after
// them, which is precisely the shape that used to commit half a transition —
// the content landed under its own transaction, the ledger refused, and the
// job reported failure over a project that had already changed.
func TestAFailedPushChangesNothing(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "atomic-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Atomic"}))

	// A first push establishes the state the second one must not disturb.
	first := uploadPush(t, deps, "job-atomic-1", projectID, []*pb.SyncBlock{
		{Id: "b1", ItemName: "en.json", SourceText: "Original", Translatable: true},
	}, nil)
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))

	before, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100})
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Equal(t, "Original", before[0].SourceText())

	// The second push edits that block, adds another, declares that the item
	// now holds only the new one (so the first would be pruned) — and asserts a
	// decisions ref the ledger does not match, so the whole thing must refuse.
	decisions, err := json.Marshal([]map[string]any{{
		"item_name": "en.json", "unit": "b1", "variant": "nb",
		"status": "approved", "decided_by": "someone", "updated": "2026-08-20T00:00:00Z",
	}})
	require.NoError(t, err)

	second := uploadPush(t, deps, "job-atomic-2", projectID, []*pb.SyncBlock{
		{Id: before[0].ID, ItemName: "en.json", SourceText: "Edited", Translatable: true},
		{Id: "b2", ItemName: "en.json", SourceText: "Added", Translatable: true},
	}, map[string]any{
		"decisions":    json.RawMessage(decisions),
		"expected_ref": map[string]string{"decisions": "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		"item_blocks":  map[string][]string{"en.json": {"b2"}},
	})
	require.Error(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	// Nothing the failed push did survives: not the edit, not the addition,
	// not the prune, not the ledger row.
	after, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100})
	require.NoError(t, err)
	require.Len(t, after, 1, "a refused push must not add, and must not prune")
	assert.Equal(t, before[0].ID, after[0].ID)
	assert.Equal(t, "Original", after[0].SourceText(), "a refused push must not edit")

	ledger, err := deps.ContentStore.(store.DecisionStore).ListUnitDecisions(ctx, projectID, "main")
	require.NoError(t, err)
	assert.Empty(t, ledger, "a refused push must not record its decisions")

	j, err := deps.JobStore.GetJob(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, j.Status)
}
