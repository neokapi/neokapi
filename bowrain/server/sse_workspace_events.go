package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// contentStoreWorkspaceResolver adapts the ContentStore to the relay's
// ProjectWorkspaceResolver so workspace-scoped clients can be matched.
type contentStoreWorkspaceResolver struct {
	store store.ContentStore
}

func (r *contentStoreWorkspaceResolver) WorkspaceForProject(ctx context.Context, projectID string) (string, error) {
	if r.store == nil {
		return "", nil
	}
	p, err := r.store.GetProject(ctx, projectID)
	if err != nil || p == nil {
		return "", err
	}
	return p.WorkspaceID, nil
}

// HandleWorkspaceEventsSSE streams unified change events for a workspace (and
// optionally a single project) to a web client over Server-Sent Events.
//
//	GET /api/v1/:ws/events            → all change events in the workspace
//	GET /api/v1/:ws/events?project=ID → change events for one project only
//
// Auth and workspace membership are enforced by the route group middleware
// (AuthMiddleware + WorkspaceAccessMiddleware), which sets workspace_id on the
// context. The stream sends a JSON ChangeEvent per `data:` frame, emits a
// periodic heartbeat comment to keep proxies from idling the connection, and
// tears down cleanly when the client disconnects.
func (s *Server) HandleWorkspaceEventsSSE(c echo.Context) error {
	if s.changeRelay == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "event relay not configured"})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	if workspaceID == "" {
		// Should be set by WorkspaceAccessMiddleware; refuse rather than leak
		// cross-workspace events.
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "workspace context unavailable"})
	}
	projectID := c.QueryParam("project")

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	c.Response().WriteHeader(http.StatusOK)

	clientID, events := s.changeRelay.Subscribe(workspaceID, projectID)
	defer s.changeRelay.Unsubscribe(clientID)

	ctx := c.Request().Context()

	// Initial comment so the client's onopen fires immediately and any proxy
	// flushes the response headers.
	fmt.Fprint(c.Response(), ": connected\n\n")
	c.Response().Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ce, ok := <-events:
			if !ok {
				return nil // relay closed (server shutdown)
			}
			payload, err := json.Marshal(ce)
			if err != nil {
				continue
			}
			fmt.Fprintf(c.Response(), "event: change\ndata: %s\n\n", payload)
			c.Response().Flush()
		case <-heartbeat.C:
			fmt.Fprint(c.Response(), ": heartbeat\n\n")
			c.Response().Flush()
		}
	}
}
