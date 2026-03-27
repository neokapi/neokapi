package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiclient "github.com/neokapi/neokapi/platform/client"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createProject creates a test project via the API and returns its ID.
func createProject(t *testing.T, srv *Server, token string) string {
	t.Helper()
	e := srv.GetEcho()
	body := `{"name":"SyncTest","default_source_language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var project store.Project
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))
	return project.ID
}

func TestSyncPush(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks (async — returns 202, then worker processes).
	body := `{"blocks":[{"id":"b1","text":"Hello"},{"id":"b2","text":"World"}]}`
	rec := pushAndDrain(t, srv, e, authHeader, "/api/v1/projects/"+pid+"/sync/push", body)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["push_id"])
	assert.Equal(t, "queued", resp["status"])
}

func TestSyncPush_ExceedsBatchLimit(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Build a request with more blocks than allowed.
	var blocks []string
	for i := 0; i < store.MaxBlocksPerRequest+1; i++ {
		blocks = append(blocks, fmt.Sprintf(`{"id":"b%d","text":"text"}`, i))
	}
	body := `{"blocks":[` + strings.Join(blocks, ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestSyncPull(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks first.
	body := `{"blocks":[{"id":"b1","text":"Hello"},{"id":"b2","text":"World"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Pull all changes from cursor 0.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/pull?cursor=0", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp store.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Changes, 2)
	assert.Greater(t, resp.NewCursor, int64(0))
	assert.False(t, resp.HasMore)
}

func TestSyncPull_Pagination(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push 5 blocks in a single request.
	var blocks []string
	for i := 0; i < 5; i++ {
		blocks = append(blocks, fmt.Sprintf(`{"id":"b%d","text":"text %d"}`, i, i))
	}
	body := `{"blocks":[` + strings.Join(blocks, ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Pull with limit=3.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/pull?cursor=0&limit=3", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp store.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Changes, 3)
	assert.True(t, resp.HasMore)

	// Pull remaining from cursor.
	url := fmt.Sprintf("/api/v1/projects/%s/sync/pull?cursor=%d&limit=3", pid, resp.NewCursor)
	req = httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp2 store.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp2))
	assert.Len(t, resp2.Changes, 2)
	assert.False(t, resp2.HasMore)
}

func TestSyncPush_WithItemMeta(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks with item metadata (BlockIndex, PreviewHTML, Format override).
	body := `{
		"blocks":[{"id":"tu1","text":"Hello","item_name":"messages.json"}],
		"items":[{"name":"messages.json","format":"json","block_index":"{\"blocks\":[]}","preview_html":"<html>preview</html>"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp apiclient.SyncPushResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Stored)

	// Verify the item has the metadata from the push request.
	ctx := req.Context()
	item, err := srv.ContentStore.GetItem(ctx, pid, "main", "messages.json")
	require.NoError(t, err)
	assert.Equal(t, "{\"blocks\":[]}", item.BlockIndex, "BlockIndex should be populated from ItemMeta")
	assert.Equal(t, "<html>preview</html>", item.PreviewHTML, "PreviewHTML should be populated from ItemMeta")
	assert.Equal(t, "json", item.Format, "Format should be overridden from ItemMeta")
}

func TestSyncGetBlocks(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks with item_name.
	body := `{"blocks":[{"id":"b1","text":"Hello","item_name":"en.json"},{"id":"b2","text":"World","item_name":"en.json"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get blocks for item.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/blocks?item_name=en.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var blocks []apiclient.BlockContent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &blocks))
	assert.Len(t, blocks, 2)

	// Verify block content (IDs are internal random IDs, not the pushed source IDs).
	sourceMap := map[string]apiclient.BlockContent{}
	for _, b := range blocks {
		sourceMap[b.Source] = b
	}
	assert.Contains(t, sourceMap, "Hello")
	assert.Contains(t, sourceMap, "World")
	assert.Equal(t, "en.json", sourceMap["Hello"].ItemName)
	assert.Equal(t, "en.json", sourceMap["World"].ItemName)
	// Internal IDs should be 8-char random strings, not the original "b1"/"b2".
	assert.Len(t, sourceMap["Hello"].ID, 8)
	assert.NotEqual(t, "b1", sourceMap["Hello"].ID)
	assert.Len(t, sourceMap["World"].ID, 8)
	assert.NotEqual(t, "b2", sourceMap["World"].ID)
}

func TestSyncGetBlocks_Empty(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Get blocks for an item that doesn't exist.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/blocks?item_name=nonexistent.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var blocks []apiclient.BlockContent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &blocks))
	assert.Empty(t, blocks)
}

func TestGetChanges(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push, modify, then pull changes.
	body := `{"blocks":[{"id":"b1","text":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body = `{"blocks":[{"id":"b1","text":"Hello World"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get changes via the changes endpoint.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/changes?cursor=0", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var cs store.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cs))
	assert.Len(t, cs.Changes, 2)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.Equal(t, "source_modified", cs.Changes[1].ChangeType)
}

func TestSyncPush_AutoSetsDefaultStream(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	ctx := t.Context()

	// Verify project starts with empty default stream.
	proj, err := srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	assert.Empty(t, proj.DefaultStream)

	// Push blocks to a non-main stream.
	body := `{"blocks":[{"id":"b1","text":"Hello","item_name":"en.json"}],"items":[{"name":"en.json"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/streams/bowrain-main/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Default stream should now be "bowrain-main".
	proj, err = srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	assert.Equal(t, "bowrain-main", proj.DefaultStream)

	// A subsequent push to a different stream should NOT change the default.
	body = `{"blocks":[{"id":"b2","text":"World","item_name":"fr.json"}],"items":[{"name":"fr.json"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/streams/other-stream/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	proj, err = srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	assert.Equal(t, "bowrain-main", proj.DefaultStream, "default stream should not change after first push")
}

func TestStreamParamWithProject(t *testing.T) {
	e := NewServer(DefaultServerConfig()).GetEcho()

	t.Run("URL param takes precedence", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("stream")
		c.SetParamValues("from-url")

		proj := &store.Project{DefaultStream: "proj-default"}
		assert.Equal(t, "from-url", streamParamWithProject(c, proj))
	})

	t.Run("query param used when no URL param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test?stream=from-query", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		proj := &store.Project{DefaultStream: "proj-default"}
		assert.Equal(t, "from-query", streamParamWithProject(c, proj))
	})

	t.Run("project default used when no URL or query param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		proj := &store.Project{DefaultStream: "bowrain-main"}
		assert.Equal(t, "bowrain-main", streamParamWithProject(c, proj))
	})

	t.Run("falls back to main when project has no default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		proj := &store.Project{}
		assert.Equal(t, "main", streamParamWithProject(c, proj))
	})

	t.Run("falls back to main when project is nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		assert.Equal(t, "main", streamParamWithProject(c, nil))
	})
}
