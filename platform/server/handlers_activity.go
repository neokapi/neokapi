package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// HandleListActivities returns activities for a workspace, optionally filtered.
func (s *Server) HandleListActivities(c echo.Context) error {
	if s.ActivityStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"activities": []any{}, "next_cursor": ""})
	}

	ws := c.Param("ws")
	ctx := c.Request().Context()

	q := bstore.ActivityQuery{
		WorkspaceID: ws,
		ProjectID:   c.QueryParam("project_id"),
		Stream:      c.QueryParam("stream"),
		ActorID:     c.QueryParam("actor_id"),
		Type:        c.QueryParam("type"),
		Cursor:      c.QueryParam("cursor"),
	}

	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			q.Limit = parsed
		}
	}

	result, err := s.ActivityStore.List(ctx, q)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if result.Activities == nil {
		result.Activities = []bstore.Activity{}
	}

	return c.JSON(http.StatusOK, result)
}
