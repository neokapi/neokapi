package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	bowrainstorage "github.com/neokapi/neokapi/bowrain/storage"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/require"
)

// initTestStores wires up in-memory SQLite stores on the server for testing.
// The server is PostgreSQL-only in production, but tests use SQLite for speed
// and isolation. It also installs factory functions on wsStores so that
// getTM/getTB create in-memory stores instead of requiring PostgreSQL.
func initTestStores(t *testing.T, srv *Server) {
	t.Helper()

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	srv.ContentStore = cs
	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)

	if srv.Config.JWTSecret != "" {
		as, err := auth.NewSQLiteAuthStore(":memory:")
		require.NoError(t, err)
		srv.AuthStore = as
		srv.Services.Auth = service.NewAuthService(as, srv.Config.JWTSecret)
	}

	// Wire up blob store and job queue for async sync push (AD-037).
	if bs, err := bloblocal.New(t.TempDir()); err == nil {
		srv.BlobStore = bs
	}
	jobDB, _ := bowrainstorage.Open(":memory:")
	if jobDB != nil {
		t.Cleanup(func() { jobDB.Close() })
		js, _ := jobs.NewSQLiteJobStore(jobDB)
		if js != nil {
			srv.JobStore = js
			srv.JobQueue = jobs.NewChannelQueue(64)
		}
	}

	// Register v1 push handler for test compatibility (removed from public routes in AD-038).
	// Tests use pushAndDrain which hits the v1 endpoint to seed data.
	testRL := RateLimitSyncPush(10, 3)
	e := srv.GetEcho()
	v1 := e.Group("/api/v1")
	v1.POST("/projects/:id/sync/push", srv.HandleSyncPush, testRL)
	v1.POST("/projects/:id/streams/:stream/sync/push", srv.HandleSyncPush, testRL)

	// Install factory functions for in-memory TM/TB stores.
	// Note: initTestStores also exposes DB() for SQLiteJobStore via cs.
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
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
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
		if err := jobs.ProcessSyncPushJobForTest(context.Background(), deps, jobID); err != nil {
			t.Logf("drainPushQueue: job %s failed: %v", jobID, err)
		}
		ack()
	}
}

// pushAndDrain sends a sync push and processes the queued job immediately.
// Returns the push response. All sync tests should use this instead of
// sending the push directly (since push is now always async).
func pushAndDrain(t *testing.T, srv *Server, e *echo.Echo, authHeader, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, "push should return 202 Accepted")
	drainPushQueue(t, srv)
	return rec
}

// testTMStore wraps InMemoryTM to satisfy the TMStore interface for tests.
type testTMStore struct {
	*sievepen.InMemoryTM
}

func (t *testTMStore) AddWithStream(entry sievepen.TMEntry, _ string) error {
	return t.Add(entry)
}

func (t *testTMStore) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]sievepen.TMEntry, int) {
	all := t.Entries()
	return filterTMEntries(all, query, sourceLocale, targetLocale, offset, limit)
}

func (t *testTMStore) SearchEntriesForStream(query, sourceLocale, targetLocale, _ string, _ []string, offset, limit int) ([]sievepen.TMEntry, int) {
	return t.SearchEntries(query, sourceLocale, targetLocale, offset, limit)
}

func (t *testTMStore) GetEntry(id string) (sievepen.TMEntry, bool) {
	for _, e := range t.Entries() {
		if e.ID == id {
			return e, true
		}
	}
	return sievepen.TMEntry{}, false
}

func filterTMEntries(entries []sievepen.TMEntry, query, sourceLocale, targetLocale string, offset, limit int) ([]sievepen.TMEntry, int) {
	var filtered []sievepen.TMEntry
	for _, e := range entries {
		if sourceLocale != "" && string(e.SourceLocale) != sourceLocale {
			continue
		}
		if targetLocale != "" && string(e.TargetLocale) != targetLocale {
			continue
		}
		filtered = append(filtered, e)
	}
	total := len(filtered)
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total
}

// testTermStore wraps InMemoryTermBase to satisfy the TBStore interface for tests.
type testTermStore struct {
	*termbase.InMemoryTermBase
}

func (t *testTermStore) AddConceptWithStream(concept termbase.Concept, _ string) error {
	return t.AddConcept(concept)
}

func (t *testTermStore) SearchForStream(query, sourceLocale, targetLocale, _ string, _ []string, offset, limit int) ([]termbase.Concept, int) {
	return t.Search(query, sourceLocale, targetLocale, offset, limit)
}
