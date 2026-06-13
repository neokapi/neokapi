package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	pb "github.com/neokapi/neokapi/bowrain/core/proto/sync/v1"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	bowsync "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// initTestStores wires up test stores on the server for testing.
// It also installs factory functions on wsStores so that
// getTM/getTB create in-memory stores instead of requiring PostgreSQL.
func initTestStores(t *testing.T, srv *Server) {
	t.Helper()

	db := pgtest.NewTestDB(t)

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	srv.ContentStore = cs
	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)

	if srv.Config.JWTSecret != "" {
		as, err := auth.NewAuthStoreFromDB(db)
		require.NoError(t, err)
		srv.AuthStore = as
		srv.Services.Auth = service.NewAuthService(as, srv.Config.JWTSecret)
	}

	// Wire up blob store and job queue for async sync push.
	if bs, err := bloblocal.New(t.TempDir()); err == nil {
		srv.BlobStore = bs
	}
	js, err := jobs.NewJobStore(db)
	if err == nil {
		srv.JobStore = js
		srv.JobQueue = jobs.NewChannelQueue(64)
	}

	// Install factory functions for in-memory TM/TB stores.
	srv.wsStores.tmFactory = func() sievepen.TMStore {
		return &testTMStore{sievepen.NewInMemoryTM()}
	}
	srv.wsStores.tbFactory = func() termbase.TBStore {
		return &testTermStore{termbase.NewInMemoryTermBase()}
	}
}

// drainPushQueue processes all queued sync-push jobs immediately.
// Call after each push to simulate the worker.
func drainPushQueue(t *testing.T, srv *Server) {
	t.Helper()
	for {
		// Non-blocking dequeue with immediate timeout.
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		jobID, ack, _, err := srv.JobQueue.Dequeue(ctx)
		cancel()
		if err != nil || jobID == "" {
			break
		}
		deps := &jobs.WorkerDeps{
			JobStore:     srv.JobStore,
			ContentStore: srv.ContentStore,
			BlobStore:    srv.BlobStore,
			Queue:        srv.JobQueue,
		}
		if err := jobs.ProcessSyncPushJobForTest(t.Context(), deps, jobID); err != nil {
			t.Logf("drainPushQueue: job %s failed: %v", jobID, err)
		}
		ack()
	}
}

// pushBlockItem represents a block to push in a test, associated with an item.
type pushBlockItem struct {
	ID       string
	Text     string
	ItemName string
}

// pushBlocks performs a full push flow (init → diff → chunk upload → commit → drain)
// and returns the commit response recorder. This tests the real end-to-end push flow.
func pushBlocks(t *testing.T, srv *Server, e *echo.Echo, authHeader, projectID string, items []pushBlockItem) *httptest.ResponseRecorder {
	t.Helper()

	// Build blocks grouped by item.
	blocksByItem := map[string][]*model.Block{}
	for _, item := range items {
		b := &model.Block{ID: item.ID, Translatable: true}
		b.SetSourceText(item.Text)
		blocksByItem[item.ItemName] = append(blocksByItem[item.ItemName], b)
	}

	// Compute item hashes for init.
	itemHashes := map[string]string{}
	blockHashesByItem := map[string]map[string]string{}
	for itemName, blocks := range blocksByItem {
		blockHashes := map[string]string{}
		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			blockHashes[b.ID] = identity.ContentHash
		}
		blockHashesByItem[itemName] = blockHashes
		itemHashes[itemName] = bowsync.ComputeItemHash(blockHashes)
	}

	basePath := "/api/v1/projects/" + projectID

	// 1. Init — send item hashes.
	initBody, _ := json.Marshal(map[string]any{
		"project_id":  projectID,
		"item_hashes": itemHashes,
	})
	req := httptest.NewRequest(http.MethodPost, basePath+"/sync/main/push/init", bytes.NewReader(initBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "push init should return 200")

	var initResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))

	// If unchanged, return early.
	if initResp["status"] == "unchanged" {
		return rec
	}

	uploadID := initResp["upload_id"].(string)

	// Collect items that need uploading (changed + new).
	var diffItems []string
	if changed, ok := initResp["changed_items"].([]any); ok {
		for _, v := range changed {
			diffItems = append(diffItems, v.(string))
		}
	}
	if newItems, ok := initResp["new_items"].([]any); ok {
		for _, v := range newItems {
			diffItems = append(diffItems, v.(string))
		}
	}

	// 2. Diff each changed/new item.
	allNeeded := map[string]map[string]bool{}
	for _, itemName := range diffItems {
		hashes := blockHashesByItem[itemName]
		if hashes == nil {
			continue
		}
		diffBody, _ := json.Marshal(map[string]any{
			"upload_id":    uploadID,
			"item_name":    itemName,
			"block_hashes": hashes,
		})
		req = httptest.NewRequest(http.MethodPost, basePath+"/sync/main/push/diff", bytes.NewReader(diffBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "push diff should return 200")

		var diffResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &diffResp))
		needed := map[string]bool{}
		if neededList, ok := diffResp["needed"].([]any); ok {
			for _, v := range neededList {
				needed[v.(string)] = true
			}
		}
		allNeeded[itemName] = needed
	}

	// 3. Upload protobuf chunks via proxy.
	type chunkRefJSON struct {
		Index       int    `json:"index"`
		ContentType string `json:"content_type"`
		Hash        string `json:"hash"`
		RecordCount int    `json:"record_count"`
		ByteSize    int    `json:"byte_size"`
	}
	var chunkRefs []chunkRefJSON
	chunkIndex := 0

	for itemName, neededIDs := range allNeeded {
		blocks := blocksByItem[itemName]
		var syncBlocks []*pb.SyncBlock
		for _, b := range blocks {
			if !neededIDs[b.ID] {
				continue
			}
			syncBlocks = append(syncBlocks, bowsync.BlockToProto(b, itemName))
		}
		if len(syncBlocks) == 0 {
			continue
		}

		chunk := &pb.SyncChunk{
			ContentType: "blocks",
			RecordCount: int32(len(syncBlocks)),
			Blocks:      syncBlocks,
		}
		chunkData, err := proto.Marshal(chunk)
		require.NoError(t, err)

		req = httptest.NewRequest(http.MethodPut,
			fmt.Sprintf("%s/sync/main/push/chunks/%s/%d", basePath, uploadID, chunkIndex),
			bytes.NewReader(chunkData))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Authorization", authHeader)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "chunk upload should return 200")

		// Use raw SHA-256 hash (matching local blob store's content-addressing).
		rawHash := sha256.Sum256(chunkData)
		chunkRefs = append(chunkRefs, chunkRefJSON{
			Index:       chunkIndex,
			ContentType: "blocks",
			Hash:        hex.EncodeToString(rawHash[:]),
			RecordCount: len(syncBlocks),
			ByteSize:    len(chunkData),
		})
		chunkIndex++
	}

	// 4. Commit.
	// Build item metadata for the commit.
	var itemMetaJSON []map[string]string
	seenItems := map[string]bool{}
	for _, item := range items {
		if item.ItemName != "" && !seenItems[item.ItemName] {
			seenItems[item.ItemName] = true
			itemMetaJSON = append(itemMetaJSON, map[string]string{
				"name":   item.ItemName,
				"format": "json",
			})
		}
	}
	itemsJSON, _ := json.Marshal(itemMetaJSON)

	commitBody, _ := json.Marshal(map[string]any{
		"upload_id":  uploadID,
		"project_id": projectID,
		"stream":     "main",
		"chunks":     chunkRefs,
		"items":      json.RawMessage(itemsJSON),
	})
	req = httptest.NewRequest(http.MethodPost, basePath+"/sync/main/push/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, "push commit should return 202")

	// 5. Drain the job queue.
	drainPushQueue(t, srv)

	return rec
}

// testTMStore wraps InMemoryTM to satisfy the TMStore interface for tests.
// All new multilingual methods delegate directly to the in-memory
// implementation, since it already implements the full TMStore interface.
type testTMStore struct {
	*sievepen.InMemoryTM
}

// testTermStore wraps InMemoryTermBase to satisfy the TBStore interface for tests.
type testTermStore struct {
	*termbase.InMemoryTermBase
}

func (t *testTermStore) AddConceptWithStream(ctx context.Context, concept termbase.Concept, _ string) error {
	return t.AddConcept(ctx, concept)
}

func (t *testTermStore) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, _ string, _ []string, offset, limit int) ([]termbase.Concept, int, error) {
	return t.Search(ctx, query, sourceLocale, targetLocale, offset, limit)
}

func (t *testTermStore) AddRelationWithStream(ctx context.Context, rel termbase.ConceptRelation, _ string) error {
	return t.AddRelation(ctx, rel)
}

func (t *testTermStore) RelationsForStream(ctx context.Context, conceptID, _ string, _ []string, scope *graph.Scope) ([]termbase.ConceptRelation, error) {
	return t.RelationsOf(ctx, conceptID, scope)
}
