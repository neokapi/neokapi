package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createProject creates a test project via the API and returns its ID.
func createProject(t *testing.T, srv *Server) string {
	t.Helper()
	e := srv.GetEcho()
	body := `{"name":"SyncTest","source_locale":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))
	return project.ID
}

func TestSyncPush(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv)

	// Push blocks.
	body := `{"blocks":[{"id":"b1","text":"Hello"},{"id":"b2","text":"World"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp SyncPushResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Stored)
	assert.Greater(t, resp.NewCursor, int64(0))
}

func TestSyncPush_ExceedsBatchLimit(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv)

	// Build a request with more blocks than allowed.
	var blocks []string
	for i := 0; i < store.MaxBlocksPerRequest+1; i++ {
		blocks = append(blocks, fmt.Sprintf(`{"id":"b%d","text":"text"}`, i))
	}
	body := `{"blocks":[` + strings.Join(blocks, ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestSyncPull(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv)

	// Push blocks first.
	body := `{"blocks":[{"id":"b1","text":"Hello"},{"id":"b2","text":"World"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Pull all changes from cursor 0.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/pull?cursor=0", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp SyncPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Changes, 2)
	assert.Greater(t, resp.NewCursor, int64(0))
	assert.False(t, resp.HasMore)
}

func TestSyncPull_Pagination(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv)

	// Push 5 blocks.
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"blocks":[{"id":"b%d","text":"text %d"}]}`, i, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Pull with limit=3.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/pull?cursor=0&limit=3", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp SyncPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Changes, 3)
	assert.True(t, resp.HasMore)

	// Pull remaining from cursor.
	url := fmt.Sprintf("/api/v1/projects/%s/sync/pull?cursor=%d&limit=3", pid, resp.NewCursor)
	req = httptest.NewRequest(http.MethodGet, url, nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp2 SyncPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp2))
	assert.Len(t, resp2.Changes, 2)
	assert.False(t, resp2.HasMore)
}

func TestGetChanges(t *testing.T) {
	srv := newTestServer(t)
	e := srv.GetEcho()
	pid := createProject(t, srv)

	// Push, modify, then pull changes.
	body := `{"blocks":[{"id":"b1","text":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body = `{"blocks":[{"id":"b1","text":"Hello World"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get changes via the changes endpoint.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/changes?cursor=0", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var cs store.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cs))
	assert.Len(t, cs.Changes, 2)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.Equal(t, "source_modified", cs.Changes[1].ChangeType)
}
