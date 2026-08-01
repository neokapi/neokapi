package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// stageSyncPush uploads a chunk + manifest for blocks and creates the queued
// push job that ingests them, returning the job ID.
func stageSyncPush(t *testing.T, deps *WorkerDeps, projectID, jobID string, blocks []*pb.SyncBlock) string {
	t.Helper()
	ctx := t.Context()

	chunk := &pb.SyncChunk{ContentType: "blocks", RecordCount: int32(len(blocks)), Blocks: blocks}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	itemsJSON, err := json.Marshal([]map[string]string{{"name": "en.json", "format": "json"}})
	require.NoError(t, err)
	manifestData, err := json.Marshal(map[string]any{
		"upload_id":  "upload-" + jobID,
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         chunkHash,
			"record_count": len(blocks),
			"byte_size":    len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	})
	require.NoError(t, err)
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
	return job.ID
}

// crashBeforeCompleteStore drops the terminal "completed" write, modelling a
// worker that stored the content and then died before recording that it had.
// The row is left in 'processing', which is exactly what the sweeper resurrects
// — the redelivery path this test exercises.
type crashBeforeCompleteStore struct {
	JobStore
	crashed bool
}

func (s *crashBeforeCompleteStore) UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error {
	if status == StatusCompleted && !s.crashed {
		s.crashed = true
		return errors.New("simulated worker crash before completion was recorded")
	}
	return s.JobStore.UpdateJobStatus(ctx, id, status, errMsg)
}

// TestProcessSyncPush_RedeliveredJobStoresOneCopy is the unit-level reproducer
// for #1527: an ingestion job that is delivered twice must converge on the
// blocks it was given, not append a second copy of each.
func TestProcessSyncPush_RedeliveredJobStoresOneCopy(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "redelivery-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Redelivery"}))

	realJobStore := deps.JobStore
	crashing := &crashBeforeCompleteStore{JobStore: realJobStore}
	deps.JobStore = crashing

	jobID := stageSyncPush(t, deps, projectID, "job-redelivery", []*pb.SyncBlock{
		{Id: "greeting", ItemName: "en.json", SourceText: "Hello, world!", Translatable: true},
		{Id: "farewell", ItemName: "en.json", SourceText: "Goodbye!", Translatable: true},
	})

	// First delivery: content lands, completion does not.
	require.Error(t, ProcessSyncPushJobForTest(ctx, deps, jobID))

	first, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, first, 2, "first delivery should store both blocks")

	// The sweeper resurrects the stranded row and the broker redelivers it.
	deps.JobStore = realJobStore
	requeued, failed, err := realJobStore.SweepStaleProcessing(ctx, 0, 3)
	require.NoError(t, err)
	require.Equal(t, 0, failed)
	require.Contains(t, requeued, jobID, "the stranded push job should be requeued")

	// Second delivery of the same job.
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, jobID))

	second, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100,
	})
	require.NoError(t, err)
	assert.Len(t, second, 2, "a redelivered push must not duplicate its blocks")

	ids := map[string]string{}
	for _, b := range second {
		ids[b.SourceID] = b.Block.ID
	}
	assert.Len(t, ids, 2, "each pushed block should still map to one stored row")
}

// TestProcessSyncPush_AnnotationWriteBackDoesNotDuplicate is the worker-level
// reproducer for what #1527 actually was: not a redelivered push, but the
// entity-extraction worker writing its annotated blocks back for the same item
// moments after ingestion. Those blocks carry the internal ids the store minted
// during ingestion, and storing them under an item name used to mint a *second*
// row per block — two rows, same content_hash, different ids, which is the
// signature the issue reports.
func TestProcessSyncPush_AnnotationWriteBackDoesNotDuplicate(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "annotate-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Annotate"}))

	jobID := stageSyncPush(t, deps, projectID, "job-annotate", []*pb.SyncBlock{
		{Id: "greeting", ItemName: "en.json", SourceText: "Hello, world!", Translatable: true},
		{Id: "farewell", ItemName: "en.json", SourceText: "Goodbye!", Translatable: true},
	})
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, jobID))

	query := store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100}
	ingested, err := deps.ContentStore.GetBlocks(ctx, query)
	require.NoError(t, err)
	require.Len(t, ingested, 2)

	// What extraction_worker.go does with its annotated output: read the item's
	// blocks, run a tool over them, store them back under the same item.
	annotated := make([]*model.Block, 0, len(ingested))
	for _, sb := range ingested {
		annotated = append(annotated, sb.Block)
	}
	require.NoError(t, deps.ContentStore.StoreBlocksForItem(ctx, projectID, "main", "en.json", annotated))

	after, err := deps.ContentStore.GetBlocks(ctx, query)
	require.NoError(t, err)
	assert.Len(t, after, 2, "annotating an item must not double its blocks")

	sourceIDs := map[string]struct{}{}
	for _, b := range after {
		sourceIDs[b.SourceID] = struct{}{}
	}
	assert.Equal(t, map[string]struct{}{"greeting": {}, "farewell": {}}, sourceIDs,
		"the client ids must survive the write-back as the rows' source ids")
}

// TestProcessSyncPush_RepeatedContentInOnePushStaysDistinct pins the other side
// of the identity choice: two blocks that legitimately carry the same text are
// two blocks, not one. Keying ingestion on the client's block ID keeps them
// apart; keying it on the content hash would silently collapse them.
func TestProcessSyncPush_RepeatedContentInOnePushStaysDistinct(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "repeat-content-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Repeat"}))

	jobID := stageSyncPush(t, deps, projectID, "job-repeat", []*pb.SyncBlock{
		{Id: "title", ItemName: "en.json", SourceText: "Open", Translatable: true},
		{Id: "button", ItemName: "en.json", SourceText: "Open", Translatable: true},
	})
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, jobID))

	blocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100,
	})
	require.NoError(t, err)
	require.Len(t, blocks, 2, "same text under two client IDs is two blocks")

	sourceIDs := map[string]struct{}{}
	for _, b := range blocks {
		sourceIDs[b.SourceID] = struct{}{}
	}
	assert.Equal(t, map[string]struct{}{"title": {}, "button": {}}, sourceIDs)
}
