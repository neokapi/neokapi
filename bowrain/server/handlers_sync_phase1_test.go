package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncGetBlocks_Pagination(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push 5 blocks.
	var items []pushBlockItem
	for i := 0; i < 5; i++ {
		items = append(items, pushBlockItem{
			ID:       fmt.Sprintf("b%d", i),
			Text:     fmt.Sprintf("text-%d", i),
			ItemName: "en.json",
		})
	}
	pushBlocks(t, srv, e, authHeader, pid, items)

	// Get all blocks (default pagination).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/blocks?item_name=en.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var allBlocks []apiclient.SyncBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &allBlocks))
	assert.Len(t, allBlocks, 5)

	// Get with limit=2.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/blocks?item_name=en.json&limit=2", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var page1 []apiclient.SyncBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))
	assert.Len(t, page1, 2)

	// Get with limit=2, offset=2.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/blocks?item_name=en.json&limit=2&offset=2", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var page2 []apiclient.SyncBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page2))
	assert.Len(t, page2, 2)

	// Pages should not overlap.
	assert.NotEqual(t, page1[0].ID, page2[0].ID)
}

func TestSyncPush_RateLimiting(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Rate limiting is applied to the commit endpoint (burst of 3, 10/min).
	// Send 5 rapid commit requests — some should succeed, some should be rate-limited.
	body := `{"upload_id":"test","project_id":"` + pid + `","stream":"main","chunks":[]}`

	successes := 0
	rateLimited := 0
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/main/push/commit", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusAccepted {
			successes++
		} else if rec.Code == http.StatusTooManyRequests {
			rateLimited++
		}
	}

	// At least some should succeed (burst) and at least one should be rate-limited.
	assert.Greater(t, successes, 0, "some commits should succeed (burst)")
	assert.Greater(t, rateLimited, 0, "some commits should be rate-limited")
}

func TestSyncPush_BatchHashLookup(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push initial blocks.
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "original", ItemName: "en.json"},
	})

	// Push updated blocks (same IDs, different text).
	pushBlocks(t, srv, e, authHeader, pid, []pushBlockItem{
		{ID: "b1", Text: "modified", ItemName: "en.json"},
	})

	// Verify the block was updated (not duplicated).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/main/blocks?item_name=en.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var blocks []apiclient.SyncBlock
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &blocks))
	assert.Len(t, blocks, 1, "should have 1 block, not 2 (upsert)")
	assert.Equal(t, "modified", blocks[0].SourceText)
}
