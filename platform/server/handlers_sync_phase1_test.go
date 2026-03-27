package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiclient "github.com/neokapi/neokapi/platform/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncGetBlocks_Pagination(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push 5 blocks.
	var blocks []string
	for i := 0; i < 5; i++ {
		blocks = append(blocks, fmt.Sprintf(`{"id":"b%d","text":"text-%d","item_name":"en.json"}`, i, i))
	}
	body := `{"blocks":[` + strings.Join(blocks, ",") + `]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get all blocks (default pagination).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/blocks?item_name=en.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var allBlocks []apiclient.BlockContent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &allBlocks))
	assert.Len(t, allBlocks, 5)

	// Get with limit=2.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/blocks?item_name=en.json&limit=2", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var page1 []apiclient.BlockContent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))
	assert.Len(t, page1, 2)

	// Get with limit=2, offset=2.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/blocks?item_name=en.json&limit=2&offset=2", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var page2 []apiclient.BlockContent
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

	body := `{"blocks":[{"id":"b1","text":"hello"}]}`

	// The rate limiter allows burst of 3, then 10/min.
	// Send 4 rapid requests — 3 should succeed, 4th should be rate-limited.
	successes := 0
	rateLimited := 0
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			successes++
		} else if rec.Code == http.StatusTooManyRequests {
			rateLimited++
		}
	}

	// At least some should succeed (burst) and at least one should be rate-limited.
	assert.Greater(t, successes, 0, "some pushes should succeed (burst)")
	assert.Greater(t, rateLimited, 0, "some pushes should be rate-limited")
}

func TestSyncPush_AsyncAccepted(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Without blob store configured, async falls back to sync.
	body := `{"blocks":[{"id":"b1","text":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push?async=true", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Falls back to sync when BlobStore is nil.
	assert.Equal(t, http.StatusOK, rec.Code, "without blob store, async falls back to sync")
}

func TestSyncPush_BatchHashLookup(t *testing.T) {
	srv, token := newTestServer(t)
	e := srv.GetEcho()
	authHeader := "Bearer " + token
	pid := createProject(t, srv, token)

	// Push initial blocks.
	body := `{"blocks":[{"id":"b1","text":"original","item_name":"en.json"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Push updated blocks (same IDs, different text).
	body = `{"blocks":[{"id":"b1","text":"modified","item_name":"en.json"}]}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+pid+"/sync/push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify the block was updated (not duplicated).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/sync/blocks?item_name=en.json", nil)
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var blocks []apiclient.BlockContent
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &blocks))
	assert.Len(t, blocks, 1, "should have 1 block, not 2 (upsert)")
	assert.Equal(t, "modified", blocks[0].Source)
}
