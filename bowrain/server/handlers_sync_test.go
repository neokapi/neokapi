package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/storage/compression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createProject creates a test project via the workspace-scoped API and returns its ID.
// The test workspace slug is "test" (created by newTestServer).
func createProject(t *testing.T, srv *Server, token string) string {
	t.Helper()
	e := srv.GetEcho()
	body := `{"name":"SyncTest","default_source_language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
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

	// Push blocks via full push flow (init → diff → chunk → commit → drain).
	rec := pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
		{ID: "b2", Text: "World", ItemName: "en.json"},
	})

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["push_id"])
	assert.Equal(t, "queued", resp["status"])
}

func TestSyncPull(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks first.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
		{ID: "b2", Text: "World", ItemName: "en.json"},
	})

	// Pull all blocks from cursor 0.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/pull?cursor=0", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp apiclient.RichPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Blocks, 2)
	assert.Greater(t, resp.Cursor, int64(0))
	assert.False(t, resp.HasMore)

	// Verify rich block content: source segments with text.
	sourceTexts := map[string]bool{}
	for _, b := range resp.Blocks {
		assert.NotEmpty(t, b.SourceText)
		assert.NotEmpty(t, b.Source, "source segments should be populated")
		assert.Equal(t, "en.json", b.ItemName)
		sourceTexts[b.SourceText] = true
	}
	assert.True(t, sourceTexts["Hello"])
	assert.True(t, sourceTexts["World"])
}

func TestSyncPull_Pagination(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push 5 blocks.
	var items []pushBlockItem
	for i := range 5 {
		items = append(items, pushBlockItem{
			ID:       fmt.Sprintf("b%d", i),
			Text:     fmt.Sprintf("text %d", i),
			ItemName: "en.json",
		})
	}
	pushBlocks(t, srv, e, authHeader, pid, items)

	// Pull with limit=3.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/pull?cursor=0&limit=3", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp apiclient.RichPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Blocks, 3)
	assert.True(t, resp.HasMore)

	// Pull remaining from cursor.
	url := fmt.Sprintf("/api/v1/projects/%s/sync/main/pull?cursor=%d&limit=3", pid, resp.Cursor)
	req = httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp2 apiclient.RichPullResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp2))
	assert.Len(t, resp2.Blocks, 2)
	assert.False(t, resp2.HasMore)
}

func TestSyncGetBlocks(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks with item_name.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
		{ID: "b2", Text: "World", ItemName: "en.json"},
	})

	// Get blocks for item.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/blocks?item_name=en.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var blocks []apiclient.SyncBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &blocks))
	assert.Len(t, blocks, 2)

	// Verify rich block content.
	sourceMap := map[string]apiclient.SyncBlock{}
	for _, b := range blocks {
		sourceMap[b.SourceText] = b
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
	// Verify source segments are populated.
	require.NotEmpty(t, sourceMap["Hello"].Source, "source segments should be populated")
	require.Len(t, sourceMap["Hello"].Source[0].Runs, 1, "source segment should have one run")
	require.NotNil(t, sourceMap["Hello"].Source[0].Runs[0].Text, "source run should be a text run")
	assert.Equal(t, "Hello", sourceMap["Hello"].Source[0].Runs[0].Text.Text)
}

func TestSyncGetBlocks_Empty(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Get blocks for an item that doesn't exist.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/blocks?item_name=nonexistent.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var blocks []apiclient.SyncBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &blocks))
	assert.Empty(t, blocks)
}

func TestSyncPull_ZstdCompression(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push blocks.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	// Pull with Accept-Encoding: zstd.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/pull?cursor=0", nil)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept-Encoding", "zstd")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "zstd", rec.Header().Get("Content-Encoding"))

	// Decompress and verify.
	pool := compression.NewPool(nil)
	decompressed, err := pool.Decompress(rec.Body.Bytes())
	require.NoError(t, err)

	var resp apiclient.RichPullResponse
	require.NoError(t, json.Unmarshal(decompressed, &resp))
	assert.Len(t, resp.Blocks, 1)
	assert.Equal(t, "Hello", resp.Blocks[0].SourceText)
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

	// Push blocks — the push commit specifies stream="main" by default.
	// To test non-main stream, we would need to modify pushBlocks to support stream param.
	// For now, push to main and verify default is set.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
	})

	// Default stream should now be "main" (set by the worker on first push).
	proj, err = srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	assert.Equal(t, "main", proj.DefaultStream)
}

func TestStreamParamWithProject(t *testing.T) {
	e := NewServer(DefaultConfig()).GetEcho()

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
