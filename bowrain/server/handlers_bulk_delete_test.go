package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bulkDeleteCtx builds an echo context for a workspace owner posting a
// bulk-delete body.
func bulkDeleteCtx(t *testing.T, srv *Server, ws, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/bulk-delete", bytes.NewReader([]byte(body)))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(r, rec)
	c.SetParamNames("ws")
	c.SetParamValues(ws)
	c.Set("workspace_id", ws)
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageMemory|platauth.PermManageTerms)
	return c, rec
}

// seedMemoryEntries adds n content-memory entries and returns their ids.
func seedMemoryEntries(t *testing.T, srv *Server, ws string, ids ...string) {
	t.Helper()
	tm, err := srv.wsStores.getMemory(ws)
	require.NoError(t, err)
	for _, entryID := range ids {
		require.NoError(t, tm.Add(t.Context(), memory.Entry{
			ID: entryID,
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: "hello " + entryID}}},
			},
			HintSrcLang: "en",
		}))
	}
}

// The resource browser's multi-select deletes in one request instead of one
// DELETE per selected row.
func TestHandleBulkDeleteMemoryEntries(t *testing.T) {
	srv := setupTestServerWithStores(t)
	const ws = "demo"
	seedMemoryEntries(t, srv, ws, "e1", "e2", "e3")

	post := func(t *testing.T, body string) (BulkDeleteResponse, int) {
		t.Helper()
		c, rec := bulkDeleteCtx(t, srv, ws, body)
		require.NoError(t, srv.HandleBulkDeleteMemoryEntries(c))
		var resp BulkDeleteResponse
		if rec.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		}
		return resp, rec.Code
	}

	t.Run("deletes the batch and reports each id", func(t *testing.T) {
		resp, code := post(t, `{"ids":["e1","e2"]}`)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, 2, resp.Deleted)
		assert.Equal(t, 0, resp.Failed)
		require.Len(t, resp.Results, 2)
		for _, r := range resp.Results {
			assert.True(t, r.Deleted)
			assert.Empty(t, r.Error)
		}

		tm, err := srv.wsStores.getMemory(ws)
		require.NoError(t, err)
		_, ok, err := tm.GetEntry(t.Context(), "e1")
		require.NoError(t, err)
		assert.False(t, ok, "a deleted entry is gone")
		_, ok, err = tm.GetEntry(t.Context(), "e3")
		require.NoError(t, err)
		assert.True(t, ok, "an unselected entry survives")
	})

	t.Run("one bad id does not strand the rest", func(t *testing.T) {
		resp, code := post(t, `{"ids":["e3","never-existed"]}`)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, 1, resp.Deleted)
		assert.Equal(t, 1, resp.Failed)
		byID := map[string]BulkDeleteResult{}
		for _, r := range resp.Results {
			byID[r.ID] = r
		}
		assert.True(t, byID["e3"].Deleted)
		assert.False(t, byID["never-existed"].Deleted)
		assert.NotEmpty(t, byID["never-existed"].Error)
	})

	t.Run("a repeated id is reported once", func(t *testing.T) {
		seedMemoryEntries(t, srv, ws, "dup")
		resp, code := post(t, `{"ids":["dup","dup"]}`)
		require.Equal(t, http.StatusOK, code)
		assert.Len(t, resp.Results, 1)
		assert.Equal(t, 1, resp.Deleted)
	})

	t.Run("an empty batch is refused", func(t *testing.T) {
		_, code := post(t, `{"ids":[]}`)
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("an oversized batch is refused", func(t *testing.T) {
		ids := make([]string, maxBulkDeleteIDs+1)
		for i := range ids {
			ids[i] = "x"
		}
		body, err := json.Marshal(BulkDeleteRequest{IDs: ids})
		require.NoError(t, err)
		_, code := post(t, string(body))
		assert.Equal(t, http.StatusBadRequest, code)
	})
}

// Deleting a concept is governed, so the batch is refused as a whole — one
// round trip carrying the change-set hint instead of one refusal per row.
func TestHandleBulkDeleteConcepts(t *testing.T) {
	srv := setupTestServerWithStores(t)

	c, rec := bulkDeleteCtx(t, srv, "demo", `{"ids":["c1","c2"]}`)
	require.NoError(t, srv.HandleBulkDeleteConcepts(c))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "governed change requires a change-set", resp["error"])
	assert.Contains(t, resp["hint"], "/changesets")

	// The body is still validated, so a caller learns about a malformed batch
	// rather than reading the governance refusal as the answer to it.
	c, rec = bulkDeleteCtx(t, srv, "demo", `{"ids":[]}`)
	require.NoError(t, srv.HandleBulkDeleteConcepts(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
