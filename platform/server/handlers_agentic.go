package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/server/agenticmcp"
)

// HandleListAgenticExecutions returns recent agent executions.
// GET /api/v1/agentic/executions?workspace=&agent=&since=&limit=
func (s *Server) HandleListAgenticExecutions(c echo.Context) error {
	store := s.agenticExecStore()
	if store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "agentic execution store not configured")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 50
	}

	execs, err := store.ListExecutions(c.Request().Context(), agenticmcp.ExecutionFilter{
		WorkspaceSlug: c.QueryParam("workspace"),
		Agent:         c.QueryParam("agent"),
		Since:         c.QueryParam("since"),
		Limit:         limit,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"executions": execs,
	})
}

// HandleListAgenticEvents returns recent agent events.
// GET /api/v1/agentic/events?workspace=&execution_id=&event_type=&limit=
func (s *Server) HandleListAgenticEvents(c echo.Context) error {
	store := s.agenticExecStore()
	if store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "agentic execution store not configured")
	}

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 100
	}

	events, err := store.ListEvents(c.Request().Context(), agenticmcp.EventFilter{
		ExecutionID:   c.QueryParam("execution_id"),
		WorkspaceSlug: c.QueryParam("workspace"),
		EventType:     c.QueryParam("event_type"),
		Limit:         limit,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"events": events,
	})
}

// HandleGetAgenticExecution returns a single execution with its events.
// GET /api/v1/agentic/executions/:id/events
func (s *Server) HandleGetAgenticExecutionEvents(c echo.Context) error {
	store := s.agenticExecStore()
	if store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "agentic execution store not configured")
	}

	execID := c.Param("id")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 200
	}

	events, err := store.ListEvents(c.Request().Context(), agenticmcp.EventFilter{
		ExecutionID: execID,
		Limit:       limit,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"execution_id": execID,
		"events":       events,
	})
}

// HandleAgenticEventsWebSocket streams real-time agentic events via WebSocket.
// GET /api/v1/agentic/events/ws?workspace=
func (s *Server) HandleAgenticEventsWebSocket(c echo.Context) error {
	hub := s.agenticEventHub()
	if hub == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "agentic event hub not configured")
	}

	conn, err := websocket.Accept(c.Response().Writer, c.Request(), &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return err
	}
	defer func() { _ = conn.CloseNow() }()

	client := &agenticmcp.EventClient{
		C:             make(chan agenticmcp.AgenticEvent, 64),
		WorkspaceSlug: c.QueryParam("workspace"),
	}
	hub.Subscribe(client)
	defer hub.Unsubscribe(client)

	ctx := c.Request().Context()

	// Read loop in background to detect disconnection.
	go func() {
		for {
			_, _, err := conn.Read(ctx)
			if err != nil {
				return
			}
		}
	}()

	// Write loop: send events to the client.
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-client.C:
			if !ok {
				return nil
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return nil
			}
		}
	}
}

// HandleListAgenticAgents returns agent profiles derived from execution history.
// GET /api/v1/agentic/agents
func (s *Server) HandleListAgenticAgents(c echo.Context) error {
	store := s.agenticExecStore()
	if store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "agentic execution store not configured")
	}

	agents, err := store.ListAgents(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"agents": agents,
	})
}

// HandleListAgenticIssues returns GitHub issues from the agent feedback repo.
// GET /api/v1/agentic/issues?state=open&limit=20
func (s *Server) HandleListAgenticIssues(c echo.Context) error {
	tracker := s.agenticIssueTracker()
	if tracker == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "issue tracker not configured")
	}

	state := c.QueryParam("state")
	if state == "" {
		state = "all"
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 20
	}

	issues, err := tracker.ListIssues(c.Request().Context(), state, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{
		"issues": issues,
	})
}

// agenticIssueTracker returns the issue tracker from the agentic MCP server, or nil.
func (s *Server) agenticIssueTracker() *agenticmcp.GitHubIssueTracker {
	if s.agenticMCP == nil {
		return nil
	}
	return s.agenticMCP.IssueTracker()
}

// agenticExecStore returns the execution store from the agentic MCP server, or nil.
func (s *Server) agenticExecStore() *agenticmcp.PostgresExecutionStore {
	if s.agenticMCP == nil {
		return nil
	}
	return s.agenticMCP.ExecStore()
}

// agenticEventHub returns the event hub from the agentic MCP server, or nil.
func (s *Server) agenticEventHub() *agenticmcp.EventHub {
	if s.agenticMCP == nil {
		return nil
	}
	return s.agenticMCP.EventHub()
}
