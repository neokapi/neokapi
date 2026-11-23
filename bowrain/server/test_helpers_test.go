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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	sqlmemory "github.com/neokapi/neokapi/bowrain/memory"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/storage"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// initTestStores wires up test stores on the server for testing.
// It also installs factory functions on wsStores so that
// getMemory/getTerms create in-memory stores instead of requiring PostgreSQL.
func initTestStores(t *testing.T, srv *Server) {
	t.Helper()
	initTestStoresOnDB(t, srv, pgtest.NewTestDB(t))
}

// initTestStoresOnDB is initTestStores over a caller-supplied database, for
// tests that need a particular connection ceiling.
func initTestStoresOnDB(t *testing.T, srv *Server, db *storage.PgDB) {
	t.Helper()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	srv.ContentStore = cs
	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)
	// Convergence runs live on the same shared DB, as in production wiring.
	srv.ConvergenceRunStore = bstore.NewConvergenceRunStore(db.DB)
	// GitHub App installation ownership, as in production wiring — the record
	// the post-install setup endpoints gate on.
	srv.ForgeInstallationStore = bstore.NewForgeInstallationStore(db.DB)

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

	// The content memory is the store production opens, one per workspace on
	// the test database; the terms store is in memory.
	srv.wsStores.memoryFactory = func(slug string) memory.Store {
		tm, err := sqlmemory.NewPostgresStoreFromDB(db, slug)
		require.NoError(t, err)
		return tm
	}
	srv.wsStores.termsFactory = func() terms.Store {
		return &testTermStore{terms.NewInMemoryStore()}
	}
}

// drainPushQueue processes all queued sync-push jobs immediately.
// Call after each push to simulate the worker.
func drainPushQueue(t *testing.T, srv *Server) {
	t.Helper()
	drainPushJobs(t, srv, false)
}

// drainPushQueueOnBus is drainPushQueue with the worker publishing on the
// server's event bus, as a deployed worker does, so a test sees the events a
// landed push publishes.
func drainPushQueueOnBus(t *testing.T, srv *Server) {
	t.Helper()
	drainPushJobs(t, srv, true)
}

// drainPushJobs processes every queued sync-push job, with the worker
// publishing on the server's event bus when onBus is set.
func drainPushJobs(t *testing.T, srv *Server, onBus bool) {
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
		if onBus {
			deps.EventBus = srv.EventBus
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
	return pushBlocksDrainedBy(t, srv, e, authHeader, projectID, items, drainPushQueue)
}

// pushBlocksDrainedBy is pushBlocks with the job queue drained by drain.
func pushBlocksDrainedBy(t *testing.T, srv *Server, e *echo.Echo, authHeader, projectID string, items []pushBlockItem, drain func(*testing.T, *Server)) *httptest.ResponseRecorder {
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
			// The transfer hash, keyed on the durable identity, as a real
			// client sends it.
			blockHashes[convergence.BlockKey(b)] = venue.RecordHash(b, "en")
		}
		blockHashesByItem[itemName] = blockHashes
		itemHashes[itemName] = venue.ComputeItemHash(blockHashes)
	}

	basePath := "/api/v1/projects/" + projectID

	// 1. Init — send item hashes.
	initBody, _ := json.Marshal(map[string]any{
		"project_id":          projectID,
		"item_hashes":         itemHashes,
		"content_model_epoch": venue.ContentModelEpoch,
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

	// 2. Fetch the venue's tree and work out what is missing, here, rather
	// than asking per item. This is what the real producer does — the helper
	// mirrors it so a route change that broke the flow shows up in these tests.
	req = httptest.NewRequest(http.MethodGet, basePath+"/sync/main/tree", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "tree should return 200")

	var treeResp struct {
		Items []struct {
			Path   string   `json:"path"`
			Record []string `json:"record"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &treeResp))
	held := map[string]bool{}
	for _, item := range treeResp.Items {
		for _, r := range item.Record {
			held[r] = true
		}
	}

	allNeeded := map[string]map[string]bool{}
	for itemName, hashes := range blockHashesByItem {
		needed := map[string]bool{}
		for key, recordHash := range hashes {
			if !held[recordHash] {
				needed[key] = true
			}
		}
		if len(needed) > 0 {
			allNeeded[itemName] = needed
		}
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
			if !neededIDs[convergence.BlockKey(b)] {
				continue
			}
			syncBlocks = append(syncBlocks, venue.BlockToProto(b, itemName))
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
		"upload_id":           uploadID,
		"project_id":          projectID,
		"stream":              "main",
		"chunks":              chunkRefs,
		"items":               json.RawMessage(itemsJSON),
		"content_model_epoch": venue.ContentModelEpoch,
	})
	req = httptest.NewRequest(http.MethodPost, basePath+"/sync/main/push/commit", bytes.NewReader(commitBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, "push commit should return 202")

	// 5. Drain the job queue.
	drain(t, srv)

	return rec
}

// testMemoryStore wraps InMemoryStore to satisfy the Store interface for tests.
// All new multilingual methods delegate directly to the in-memory
// implementation, since it already implements the full Store interface.
type testMemoryStore struct {
	*memory.InMemoryStore
}

// testTermStore is an in-memory terms store with the write rules of the
// PostgreSQL one (sqlterms.WithRevisions): AddConcept creates and never
// replaces, and a stored concept changes only at the revision the writer
// read. A test that rewrites a concept does so through rewriteConcept.
type testTermStore struct {
	*terms.InMemoryStore
}

var _ sqlterms.Store = (*testTermStore)(nil)

// testTermRevisions holds the revision wrapper of each in-memory store, so
// every testTermStore over one store shares its revisions.
var testTermRevisions sync.Map // *terms.InMemoryStore -> sqlterms.Store

func (t *testTermStore) revisioned() sqlterms.Store {
	if v, ok := testTermRevisions.Load(t.InMemoryStore); ok {
		return v.(sqlterms.Store)
	}
	v, _ := testTermRevisions.LoadOrStore(t.InMemoryStore, sqlterms.WithRevisions(memoryStreams{t.InMemoryStore}))
	return v.(sqlterms.Store)
}

func (t *testTermStore) AddConcept(ctx context.Context, concept terms.Concept) error {
	return t.revisioned().AddConcept(ctx, concept)
}

func (t *testTermStore) AddConceptWithStream(ctx context.Context, concept terms.Concept, stream string) error {
	return t.revisioned().AddConceptWithStream(ctx, concept, stream)
}

func (t *testTermStore) DeleteConcept(ctx context.Context, id string) error {
	return t.revisioned().DeleteConcept(ctx, id)
}

func (t *testTermStore) CreateImport(ctx context.Context, r sqlterms.ImportRecord) error {
	return t.revisioned().CreateImport(ctx, r)
}

func (t *testTermStore) TouchImport(ctx context.Context, id, name, by string, at time.Time) (sqlterms.ImportRecord, error) {
	return t.revisioned().TouchImport(ctx, id, name, by, at)
}

func (t *testTermStore) GetImport(ctx context.Context, id string) (sqlterms.ImportRecord, bool, error) {
	return t.revisioned().GetImport(ctx, id)
}

func (t *testTermStore) Imports(ctx context.Context, limit int) ([]sqlterms.ImportRecord, error) {
	return t.revisioned().Imports(ctx, limit)
}

func (t *testTermStore) DeleteConceptAt(ctx context.Context, id, base string) error {
	return t.revisioned().DeleteConceptAt(ctx, id, base)
}

func (t *testTermStore) DeleteConceptsAt(ctx context.Context, revs map[string]string) error {
	return t.revisioned().DeleteConceptsAt(ctx, revs)
}

func (t *testTermStore) GetConceptAt(ctx context.Context, id string) (terms.Concept, string, bool, error) {
	return t.revisioned().GetConceptAt(ctx, id)
}

func (t *testTermStore) CreateConcept(ctx context.Context, concept terms.Concept, stream string) (string, error) {
	return t.revisioned().CreateConcept(ctx, concept, stream)
}

func (t *testTermStore) UpdateConcept(ctx context.Context, concept terms.Concept, stream, base string) (string, error) {
	return t.revisioned().UpdateConcept(ctx, concept, stream, base)
}

func (t *testTermStore) ConceptRevisions(ctx context.Context, ids []string) (map[string]string, error) {
	return t.revisioned().ConceptRevisions(ctx, ids)
}

func (t *testTermStore) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, _ string, _ []string, offset, limit int) ([]terms.Concept, int, error) {
	return t.Search(ctx, query, sourceLocale, targetLocale, offset, limit)
}

func (t *testTermStore) AddRelationWithStream(ctx context.Context, rel terms.ConceptRelation, _ string) error {
	return t.AddRelation(ctx, rel)
}

func (t *testTermStore) RelationsForStream(ctx context.Context, conceptID, _ string, _ []string, scope *graph.Scope) ([]terms.ConceptRelation, error) {
	return t.RelationsOf(ctx, conceptID, scope)
}

// memoryStreams gives an in-memory store the stream methods of terms.Store,
// for the revision wrapper testTermStore writes through.
type memoryStreams struct{ *terms.InMemoryStore }

func (m memoryStreams) AddConceptWithStream(ctx context.Context, concept terms.Concept, _ string) error {
	return m.AddConcept(ctx, concept)
}

func (m memoryStreams) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, _ string, _ []string, offset, limit int) ([]terms.Concept, int, error) {
	return m.Search(ctx, query, sourceLocale, targetLocale, offset, limit)
}

func (m memoryStreams) AddRelationWithStream(ctx context.Context, rel terms.ConceptRelation, _ string) error {
	return m.AddRelation(ctx, rel)
}

func (m memoryStreams) RelationsForStream(ctx context.Context, conceptID, _ string, _ []string, scope *graph.Scope) ([]terms.ConceptRelation, error) {
	return m.RelationsOf(ctx, conceptID, scope)
}

// conceptRevision is the stored concept's current revision, as an editor
// that just read it holds it.
func conceptRevision(t *testing.T, store terms.Terminology, id string) string {
	t.Helper()
	tb, ok := store.(sqlterms.ConceptWriter)
	require.True(t, ok, "the terms store keeps revisions")
	_, rev, found, err := tb.GetConceptAt(t.Context(), id)
	require.NoError(t, err)
	require.True(t, found, "concept %s is stored", id)
	return rev
}

// withBase puts base_revision into a JSON object body.
func withBase(body, rev string) string {
	return strings.Replace(body, "{", `{"base_revision":"`+rev+`",`, 1)
}

// rewriteConcept replaces a stored concept at its current revision, as a
// person who has just read it saves it. A framework store, which keeps no
// revisions, is written over.
func rewriteConcept(t *testing.T, store terms.Terminology, concept terms.Concept) {
	t.Helper()
	tb, ok := store.(sqlterms.ConceptWriter)
	if !ok {
		require.NoError(t, store.AddConcept(t.Context(), concept))
		return
	}
	_, rev, ok, err := tb.GetConceptAt(t.Context(), concept.ID)
	require.NoError(t, err)
	require.True(t, ok, "concept %s to rewrite is stored", concept.ID)
	_, err = tb.UpdateConcept(t.Context(), concept, "", rev)
	require.NoError(t, err)
}
