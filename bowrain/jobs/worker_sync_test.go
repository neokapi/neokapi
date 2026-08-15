package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// newTestWorkerDeps creates WorkerDeps with PostgreSQL stores and local blob storage.
func newTestWorkerDeps(t *testing.T) *WorkerDeps {
	t.Helper()
	db := pgtest.NewTestDB(t)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	bs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	return &WorkerDeps{
		JobStore:     js,
		ContentStore: cs,
		BlobStore:    bs,
	}
}

func TestProcessSyncPush_BasicBlocks(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "test-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Test Project"})

	// Build a chunk with two blocks.
	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 2,
		Blocks: []*pb.SyncBlock{
			{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true},
			{Id: "b2", ItemName: "en.json", SourceText: "World", Translatable: true},
		},
	}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)

	// Upload chunk to blob store.
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	// Build and upload manifest.
	items := []map[string]string{{"name": "en.json", "format": "json"}}
	itemsJSON, _ := json.Marshal(items)
	manifest := map[string]any{
		"upload_id":  "test-upload",
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         chunkHash,
			"record_count": 2,
			"byte_size":    len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	}
	manifestData, _ := json.Marshal(manifest)
	manifestRef, err := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})
	require.NoError(t, err)

	// Create and process the job.
	job := &TranslationJob{
		ID:        "job-1",
		ProjectID: projectID,
		ItemName:  "__sync_push__",
		Model:     manifestRef.Key,
		PushID:    "push-1",
		Status:    StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err = ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.NoError(t, err)

	// Verify blocks were stored.
	blocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 100})
	require.NoError(t, err)
	assert.Len(t, blocks, 2)

	// Verify job is completed.
	j, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, j.Status)
}

func TestProcessSyncPush_ConflictDetection(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "conflict-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Conflict Test"})

	// First push: store a block via the sync push worker.
	initBlock := &model.Block{ID: "b1", Translatable: true}
	initBlock.SetSourceText("Original")
	require.NoError(t, deps.ContentStore.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{initBlock}))

	// Get the stored block's hash by querying the item's blocks.
	storedBlocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, storedBlocks)
	internalID := storedBlocks[0].ID
	correctHash := storedBlocks[0].ContentHash

	// Second push with wrong expected_hash → should fail.
	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{
				Id:           internalID,
				ItemName:     "en.json",
				SourceText:   "Updated",
				Translatable: true,
				ExpectedHash: "wrong-hash",
			},
		},
	}
	chunkData, _ := proto.Marshal(chunk)
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	itemsJSON, _ := json.Marshal([]map[string]string{{"name": "en.json", "format": "json"}})
	manifest := map[string]any{
		"upload_id":  "upload-2",
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index": 0, "content_type": "blocks",
			"hash": chunkHash, "record_count": 1, "byte_size": len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	}
	manifestData, _ := json.Marshal(manifest)
	manifestRef, _ := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})

	job := &TranslationJob{
		ID: "job-conflict", ProjectID: projectID, ItemName: "__sync_push__",
		Model: manifestRef.Key, PushID: "push-2", Status: StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err = ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.Error(t, err, "should fail with conflict")
	assert.Contains(t, err.Error(), "conflict on block")

	// Now push with correct expected_hash → should succeed.
	chunk.Blocks[0].ExpectedHash = correctHash
	chunkData, _ = proto.Marshal(chunk)
	rawHash = sha256.Sum256(chunkData)
	chunkHash = hex.EncodeToString(rawHash[:])
	_, _ = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})

	manifest["chunks"] = []map[string]any{{
		"index": 0, "content_type": "blocks",
		"hash": chunkHash, "record_count": 1, "byte_size": len(chunkData),
	}}
	manifestData, _ = json.Marshal(manifest)
	manifestRef, _ = deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})

	job2 := &TranslationJob{
		ID: "job-conflict-ok", ProjectID: projectID, ItemName: "__sync_push__",
		Model: manifestRef.Key, PushID: "push-3", Status: StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job2))

	err = ProcessSyncPushJobForTest(ctx, deps, job2.ID)
	assert.NoError(t, err, "should succeed with correct expected_hash")
}

func TestProcessSyncPush_AutoCreateStream(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "stream-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Stream Test"})

	// Push to non-main stream should auto-create it.
	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{Id: "b1", ItemName: "fr.json", SourceText: "Bonjour", Translatable: true},
		},
	}
	chunkData, _ := proto.Marshal(chunk)
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	_, _ = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})

	itemsJSON, _ := json.Marshal([]map[string]string{{"name": "fr.json", "format": "json"}})
	manifest := map[string]any{
		"upload_id":  "upload-stream",
		"project_id": projectID,
		"stream":     "feature-1",
		"chunks": []map[string]any{{
			"index": 0, "content_type": "blocks",
			"hash": chunkHash, "record_count": 1, "byte_size": len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	}
	manifestData, _ := json.Marshal(manifest)
	manifestRef, _ := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})

	job := &TranslationJob{
		ID: "job-stream", ProjectID: projectID, ItemName: "__sync_push__",
		Model: manifestRef.Key, PushID: "push-stream", Status: StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err := ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.NoError(t, err)

	// Verify stream was created.
	stream, err := deps.ContentStore.GetStream(ctx, projectID, "feature-1")
	require.NoError(t, err)
	assert.Equal(t, "main", stream.Parent)
}

func TestProcessSyncPush_ItemMetadata(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "meta-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Meta Test"})

	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{Id: "b1", ItemName: "page.md", SourceText: "# Title", Translatable: true},
		},
	}
	chunkData, _ := proto.Marshal(chunk)
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	_, _ = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})

	items := []map[string]string{
		{"name": "page.md", "format": "markdown"},
	}
	itemsJSON, _ := json.Marshal(items)
	manifest := map[string]any{
		"upload_id":  "upload-meta",
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index": 0, "content_type": "blocks",
			"hash": chunkHash, "record_count": 1, "byte_size": len(chunkData),
		}},
		"items": json.RawMessage(itemsJSON),
	}
	manifestData, _ := json.Marshal(manifest)
	manifestRef, _ := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})

	job := &TranslationJob{
		ID: "job-meta", ProjectID: projectID, ItemName: "__sync_push__",
		Model: manifestRef.Key, PushID: "push-meta", Status: StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err := ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.NoError(t, err)

	// Verify item metadata.
	item, err := deps.ContentStore.GetItem(ctx, projectID, "main", "page.md")
	require.NoError(t, err)
	assert.Equal(t, "markdown", item.Format)
}

// swappedBlobStore serves one substituted payload for a chosen key and
// otherwise delegates. It stands in for any store that could return content
// other than what a key names — a bug, a shared cache, a backend whose keys are
// not derived from content.
type swappedBlobStore struct {
	corestorage.BlobStore
	key     string
	payload []byte
}

func (s *swappedBlobStore) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == s.key {
		return io.NopCloser(bytes.NewReader(s.payload)), nil
	}
	return s.BlobStore.Download(ctx, key)
}

// failingItemStore fails every StoreItem and otherwise delegates. Item metadata
// is a separate write from the blocks it describes, so it is the write that can
// fail on its own.
type failingItemStore struct {
	store.ContentStore
	err error
}

func (s *failingItemStore) StoreItem(context.Context, string, string, *store.Item) error {
	return s.err
}

// Item metadata — format, block index, preview, collection — is written
// separately from the blocks it describes. If that write is allowed to fail
// unheard, the push reports success while the items it just stored are missing
// everything that says how to render them.
func TestProcessSyncPush_ItemMetadataWriteFailureFailsTheJob(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "item-write-failure-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Item Write Failure"})

	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true},
		},
	}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)
	sum := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(sum[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	manifestRef := uploadSyncManifest(t, ctx, deps, projectID, chunkHash, len(chunkData),
		json.RawMessage(`[{"name":"en.json","format":"json"}]`))

	deps.ContentStore = &failingItemStore{
		ContentStore: deps.ContentStore,
		err:          errors.New("item table is unavailable"),
	}

	job := &TranslationJob{
		ID:        "job-item-write-failure",
		ProjectID: projectID,
		ItemName:  "__sync_push__",
		Model:     manifestRef.Key,
		PushID:    "push-item-write-failure",
		Status:    StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err = ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.Error(t, err, "an item metadata write that failed must not be reported as a completed push")
	assert.Contains(t, err.Error(), "item table is unavailable")

	j, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, j.Status)
}

// The manifest's item metadata is what tells the worker each item's format,
// block index, preview and collection. Parsing it as far as the first bad byte
// and carrying on would store the content with default metadata and nothing to
// say the rest was ever there.
func TestProcessSyncPush_UnparseableItemMetadataFailsTheJob(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "corrupt-items-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Corrupt Items"})

	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true},
		},
	}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)
	sum := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(sum[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	// Well-formed JSON, but an object where the array of item metadata belongs.
	manifestRef := uploadSyncManifest(t, ctx, deps, projectID, chunkHash, len(chunkData),
		json.RawMessage(`{"en.json":{"format":"json"}}`))

	job := &TranslationJob{
		ID:        "job-corrupt-items",
		ProjectID: projectID,
		ItemName:  "__sync_push__",
		Model:     manifestRef.Key,
		PushID:    "push-corrupt-items",
		Status:    StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err = ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.Error(t, err, "item metadata that cannot be parsed must fail the push rather than be dropped")

	blocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, blocks, "no content may land from a push whose item metadata was lost")

	j, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, j.Status)
}

// uploadSyncManifest writes a single-chunk push manifest carrying the given raw
// item metadata, and returns the blob ref the job points at.
func uploadSyncManifest(t *testing.T, ctx context.Context, deps *WorkerDeps, projectID, chunkHash string, byteSize int, items json.RawMessage) *corestorage.BlobRef {
	t.Helper()
	manifest := map[string]any{
		"upload_id":  "upload-" + projectID,
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         chunkHash,
			"record_count": 1,
			"byte_size":    byteSize,
		}},
		"items": items,
	}
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)
	ref, err := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})
	require.NoError(t, err)
	return ref
}

// TestProcessSyncPush_ChunkContentMustMatchItsHash pins that the worker treats a
// manifest hash as a promise about the bytes rather than only as a place to look
// them up. Content that does not re-derive to the hash it was fetched under is
// refused, so no store that hands back the wrong payload can get that payload
// parsed and written into a project.
func TestProcessSyncPush_ChunkContentMustMatchItsHash(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "hash-mismatch-project"
	_ = deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Hash Mismatch"})

	// The chunk the manifest describes, and whose hash it carries.
	declared := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{Id: "declared", ItemName: "en.json", SourceText: "Declared", Translatable: true},
		},
	}
	declaredData, err := proto.Marshal(declared)
	require.NoError(t, err)
	sum := sha256.Sum256(declaredData)
	chunkHash := hex.EncodeToString(sum[:])
	_, err = deps.BlobStore.Upload(ctx, declaredData, corestorage.UploadOptions{})
	require.NoError(t, err)

	// Different content, perfectly valid on its own terms, that the store will
	// return for that hash instead.
	substituted := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 1,
		Blocks: []*pb.SyncBlock{
			{Id: "substituted", ItemName: "en.json", SourceText: "Substituted", Translatable: true},
		},
	}
	substitutedData, err := proto.Marshal(substituted)
	require.NoError(t, err)

	items := []map[string]string{{"name": "en.json", "format": "json"}}
	itemsJSON, _ := json.Marshal(items)
	manifest := map[string]any{
		"upload_id":  "mismatch-upload",
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         chunkHash,
			"record_count": 1,
			"byte_size":    len(declaredData),
		}},
		"items": json.RawMessage(itemsJSON),
	}
	manifestData, _ := json.Marshal(manifest)
	manifestRef, err := deps.BlobStore.Upload(ctx, manifestData, corestorage.UploadOptions{})
	require.NoError(t, err)

	// Only now swap what that hash resolves to, so the manifest itself still loads.
	deps.BlobStore = &swappedBlobStore{
		BlobStore: deps.BlobStore,
		key:       chunkHash,
		payload:   substitutedData,
	}

	job := &TranslationJob{
		ID:        "job-hash-mismatch",
		ProjectID: projectID,
		ItemName:  "__sync_push__",
		Model:     manifestRef.Key,
		PushID:    "push-hash-mismatch",
		Status:    StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	err = ProcessSyncPushJobForTest(ctx, deps, job.ID)
	require.Error(t, err, "a chunk whose content does not match its hash must not be processed")

	blocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, blocks, "nothing from an unverified chunk may reach the project")

	j, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, j.Status)
}

// TestComputeHashes verifies that the Merkle hash computation in bowsync is deterministic.
func TestComputeHashes(t *testing.T) {
	blockHashes := map[string]string{
		"b1": "hash1",
		"b2": "hash2",
	}
	h1 := venue.ComputeItemHash(blockHashes)
	h2 := venue.ComputeItemHash(blockHashes)
	assert.Equal(t, h1, h2, "item hash should be deterministic")

	itemHashes := map[string]string{
		"en.json": h1,
		"fr.json": "other",
	}
	r1 := venue.ComputeRootHash(itemHashes)
	r2 := venue.ComputeRootHash(itemHashes)
	assert.Equal(t, r1, r2, "root hash should be deterministic")
}
