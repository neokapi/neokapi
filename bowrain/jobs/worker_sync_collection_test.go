package jobs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	"github.com/neokapi/neokapi/bowrain/core/store"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// logSink collects the default logger's output for a test. The worker writes
// from one goroutine, but slog handlers are documented as concurrency-safe and
// the buffer is read after the push returns, so the lock is what makes that
// read safe rather than racy.
type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// captureLogs points the default logger at a buffer for the duration of a test.
func captureLogs(t *testing.T) *logSink {
	t.Helper()
	sink := &logSink{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(sink, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return sink
}

// stageSyncPushWithCollection stages a one-block push whose item meta names a
// collection, and returns the job ID. It is stageSyncPush with the item meta's
// collection filled in, which is the field the binding under test reads.
func stageSyncPushWithCollection(t *testing.T, deps *WorkerDeps, projectID, jobID, itemName, collection string) string {
	t.Helper()
	ctx := t.Context()

	blocks := []*pb.SyncBlock{{Id: "b1", ItemName: itemName, SourceText: "Hello", Translatable: true}}
	chunk := &pb.SyncChunk{ContentType: "blocks", RecordCount: 1, Blocks: blocks}
	chunkData, err := proto.Marshal(chunk)
	require.NoError(t, err)
	rawHash := sha256.Sum256(chunkData)
	chunkHash := hex.EncodeToString(rawHash[:])
	_, err = deps.BlobStore.Upload(ctx, chunkData, corestorage.UploadOptions{})
	require.NoError(t, err)

	itemsJSON, err := json.Marshal([]map[string]string{
		{"name": itemName, "format": "json", "collection": collection},
	})
	require.NoError(t, err)
	manifestData, err := json.Marshal(map[string]any{
		"upload_id":  "upload-" + jobID,
		"project_id": projectID,
		"stream":     "main",
		"chunks": []map[string]any{{
			"index":        0,
			"content_type": "blocks",
			"hash":         chunkHash,
			"record_count": 1,
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

// TestProcessSyncPush_ItemCollectionBinding pins where a pushed item lands when
// the collection its meta names cannot be resolved, and what the worker says
// about it.
//
// The two have to be pinned together. Storing an item with no collection id
// does not leave it outside every collection: the store resolves the empty id
// to the project's default collection, which every project created through the
// API has (#1786). So a bind failure hands the item to the default collection's
// bindings — the opposite of "ungrouped, and no governance applies" — and only a
// project without a default collection produces a genuinely ungrouped item.
func TestProcessSyncPush_ItemCollectionBinding(t *testing.T) {
	tests := []struct {
		name string
		// seed prepares the project's collections and returns the collection id
		// the pushed item must end up carrying ("" for genuinely ungrouped).
		seed func(t *testing.T, deps *WorkerDeps, projectID string) string
		// meta is the collection name the pushed item's meta carries.
		meta string
		// wantLog is the substring the warning must contain; empty means the
		// push must not warn about the binding at all.
		wantLog string
		// notInLog must not appear anywhere in the captured log.
		notInLog string
	}{
		{
			name: "named collection resolves",
			seed: func(t *testing.T, deps *WorkerDeps, projectID string) string {
				coll := &store.Collection{ProjectID: projectID, Name: "docs", ItemLabel: "item"}
				require.NoError(t, deps.ContentStore.CreateCollection(t.Context(), coll))
				resolved, err := deps.ContentStore.GetCollectionByName(t.Context(), projectID, "docs", "main")
				require.NoError(t, err)
				return resolved.ID
			},
			meta:     "docs",
			notInLog: "could not be bound",
		},
		{
			name: "named collection missing, project has a default",
			seed: func(t *testing.T, deps *WorkerDeps, projectID string) string {
				def := &store.Collection{ProjectID: projectID, Name: "default", ItemLabel: "item", IsDefault: true}
				require.NoError(t, deps.ContentStore.CreateCollection(t.Context(), def))
				resolved, err := deps.ContentStore.GetDefaultCollection(t.Context(), projectID)
				require.NoError(t, err)
				return resolved.ID
			},
			meta:     "docs",
			wantLog:  "it falls to the project's default collection",
			notInLog: "it is stored ungrouped",
		},
		{
			name: "named collection missing and no default collection",
			seed: func(t *testing.T, deps *WorkerDeps, projectID string) string {
				return ""
			},
			meta:    "docs",
			wantLog: "the project has no default collection; it is stored ungrouped",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := newTestWorkerDeps(t)
			ctx := t.Context()

			projectID := "collection-binding"
			require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Collection Binding"}))
			wantCollectionID := tc.seed(t, deps, projectID)

			logs := captureLogs(t)
			jobID := stageSyncPushWithCollection(t, deps, projectID, "job-collection-binding", "en.json", tc.meta)
			require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, jobID))

			item, err := deps.ContentStore.GetItem(ctx, projectID, "main", "en.json")
			require.NoError(t, err)
			assert.Equal(t, wantCollectionID, item.CollectionID,
				"the stored item's collection is what governs it, whatever the log says")

			if tc.wantLog != "" {
				assert.Contains(t, logs.String(), tc.wantLog)
			}
			if tc.notInLog != "" {
				assert.NotContains(t, logs.String(), tc.notInLog)
			}
		})
	}
}
