package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// batchStatusCtx builds an echo context for GET /connectors/status with the
// given query string.
func batchStatusCtx(t *testing.T, wsID, query string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/connectors/status"+query, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.Set("workspace_id", wsID)
	c.Set("workspace_role", platauth.RoleOwner)
	c.Set("project_permissions", platauth.PermManageConnectors)
	return c, rec
}

// addTestConnector adds a wordpress connector and returns its id.
func addTestConnector(t *testing.T, s *Server, wsID, name string) string {
	t.Helper()
	c, rec := connCtx(t, http.MethodPost,
		`{"type":"wordpress","config":{"url":"https://`+name+`.example.com","username":"editor","password":"pw","name":"`+name+`"}}`,
		wsID, "")
	require.NoError(t, s.HandleAddConnector(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var added map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &added))
	require.NotEmpty(t, added["id"])
	return added["id"]
}

// One request answers a whole panel of connector rows; the two surfaces that
// render a status per row used to fire one live probe each.
func TestHandleConnectorStatusBatch(t *testing.T) {
	s, _ := newConnectorPersistenceServer(t, connectorTestCipher(t))
	const wsID = "ws-1"
	ctx := context.Background()

	first := addTestConnector(t, s, wsID, "first")
	second := addTestConnector(t, s, wsID, "second")

	synced := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, s.ConnectorConfigStore.TouchLastSync(ctx, wsID, first, synced))
	require.NoError(t, s.ConnectorConfigStore.SetLastError(ctx, wsID, second, "fetch failed: boom"))

	post := func(t *testing.T, query string) ConnectorStatusBatchResponse {
		t.Helper()
		c, rec := batchStatusCtx(t, wsID, query)
		require.NoError(t, s.HandleConnectorStatusBatch(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var resp ConnectorStatusBatchResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	// The keys, not just the values: SyncStatus carried no json tags, so every
	// element of this map serialized PascalCase — the one bowrain response that
	// did, and the reason the TypeScript adapter kept a second DTO for the same
	// reading the desktop binding already returned in snake_case.
	t.Run("elements serialize snake_case", func(t *testing.T) {
		c, rec := batchStatusCtx(t, wsID, "?ids="+first)
		require.NoError(t, s.HandleConnectorStatusBatch(c))
		var raw struct {
			Statuses map[string]map[string]json.RawMessage `json:"statuses"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
		keys := make([]string, 0, len(raw.Statuses[first]))
		for k := range raw.Statuses[first] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.Equal(t, []string{
			"connector_id", "errors", "file_count", "item_count",
			"last_sync", "pending_pull", "pending_push", "word_count",
		}, keys)
	})

	t.Run("one call answers every id", func(t *testing.T) {
		resp := post(t, "?ids="+first+","+second)
		require.Len(t, resp.Statuses, 2)
		assert.Empty(t, resp.Unknown)
		assert.Equal(t, synced, resp.Statuses[first].LastSync.UTC())
		assert.Empty(t, resp.Statuses[first].Errors)
		assert.Equal(t, []string{"fetch failed: boom"}, resp.Statuses[second].Errors)
	})

	t.Run("matches the per-connector route", func(t *testing.T) {
		single := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(single, rec)
		c.Set("workspace_id", wsID)
		c.SetParamNames("id")
		c.SetParamValues(second)
		require.NoError(t, s.HandleConnectorStatus(c))
		require.Equal(t, http.StatusOK, rec.Code)

		var one venueconn.SyncStatus
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &one))
		assert.Equal(t, &one, post(t, "?ids="+second).Statuses[second])
	})

	t.Run("probe applies to the whole batch", func(t *testing.T) {
		resp := post(t, "?ids="+first+","+second+"&probe=1")
		require.Len(t, resp.Statuses, 2)
		// The live probe fabricates no last-sync: the stored timestamp wins.
		assert.Equal(t, synced, resp.Statuses[first].LastSync.UTC())
	})

	t.Run("an unknown id is reported, not fatal", func(t *testing.T) {
		resp := post(t, "?ids="+first+",nope")
		require.Len(t, resp.Statuses, 1)
		assert.Equal(t, []string{"nope"}, resp.Unknown)
	})

	t.Run("a repeated id is resolved once", func(t *testing.T) {
		resp := post(t, "?ids="+first+","+first)
		assert.Len(t, resp.Statuses, 1)
	})

	t.Run("ids is required", func(t *testing.T) {
		c, rec := batchStatusCtx(t, wsID, "")
		require.NoError(t, s.HandleConnectorStatusBatch(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("an oversized batch is refused", func(t *testing.T) {
		ids := make([]string, maxConnectorStatusBatch+1)
		for i := range ids {
			ids[i] = "c"
		}
		c, rec := batchStatusCtx(t, wsID, "?ids="+strings.Join(ids, ","))
		require.NoError(t, s.HandleConnectorStatusBatch(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
