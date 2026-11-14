package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordWorkspaceActivity files one activity under each key the recorder
// meets: the slug an event carries in its data, the workspace id an audit
// event carries, and the workspace of the project a store event names.
func recordWorkspaceActivity(t *testing.T, srv *Server, ws *platauth.Workspace) {
	t.Helper()
	require.NoError(t, srv.ContentStore.CreateProject(t.Context(), &store.Project{
		ID: "p-1", Name: "Docs", WorkspaceID: ws.ID, DashboardVisibility: "public",
	}))

	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	recorder := event.NewActivityRecorder(srv.ActivityStore, bus,
		event.WithProjectWorkspaces(&contentStoreWorkspaceResolver{store: srv.ContentStore}))

	bus.Publish(platev.Event{
		Type: platev.EventProjectCreated, ProjectID: "p-1", Actor: "u1",
		Data: map[string]string{"name": "Docs", "workspace_slug": ws.Slug},
	})
	bus.Publish(platev.Event{
		Type: platev.EventProjectUpdated, ProjectID: "p-1", Actor: "u1", WorkspaceID: ws.ID,
		Data: map[string]string{"name": "Docs"},
	})
	bus.Publish(platev.Event{
		Type: platev.EventProjectUpdated, ProjectID: "p-1", Actor: "u1",
		Data: map[string]string{"name": "Docs renamed"},
	})
	// Close waits for the handler to finish every delivered event.
	recorder.Close()
}

// Pulse lists the activity the recorder files, whichever workspace key the
// event carried, and pages through it with the cursor it returns.
func TestHandlePulseActivity_ListsRecordedActivity(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ws := &platauth.Workspace{ID: "ws-uuid-1", Name: "Acme", Slug: "acme"}
	recordWorkspaceActivity(t, srv, ws)

	e := srv.GetEcho()
	seen := map[string]int{}
	cursor := ""
	for range 5 {
		q := url.Values{"limit": {"1"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/acme/activity?"+q.Encode(), ws)
		require.NoError(t, srv.HandlePulseActivity(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var resp struct {
			Activities []store.PulseActivity `json:"activities"`
			NextCursor string                `json:"next_cursor"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		for _, a := range resp.Activities {
			seen[a.ID]++
		}
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	require.Len(t, seen, 3, "one activity per workspace key")
	for id, n := range seen {
		assert.Equal(t, 1, n, "activity %s", id)
	}

	c, rec := pulseContext(e, http.MethodGet, "/api/v1/pulse/acme/activity/heatmap", ws)
	require.NoError(t, srv.HandlePulseActivityHeatmap(c))
	var heat struct {
		Days []store.PulseHeatmapDay `json:"days"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &heat))
	total := 0
	for _, d := range heat.Days {
		total += d.Count
	}
	assert.Equal(t, 3, total)
}

// The workspace feed lists the same three entries.
func TestHandleListActivities_ListsEveryWorkspaceKey(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ws := &platauth.Workspace{ID: "ws-uuid-1", Name: "Acme", Slug: "acme"}
	recordWorkspaceActivity(t, srv, ws)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/activities", nil)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues(ws.Slug)
	c.Set("workspace_id", ws.ID)

	require.NoError(t, srv.HandleListActivities(c))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp ActivityListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Activities, 3)
}

func TestHandleListActivities_RejectsForeignCursor(t *testing.T) {
	srv := setupTestServerWithStores(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/acme/activities?cursor=2026-11-14T09:30:00Z", nil)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("acme")

	require.NoError(t, srv.HandleListActivities(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
