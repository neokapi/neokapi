package server

import (
	"net/http"
	"time"

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

	// ?type is one prefix ("block" matches block.*); ?types is a comma-separated
	// set of prefixes, for feeds that span families — a review inbox wants
	// review.* and task.*, which no single prefix expresses.
	q := bstore.ActivityQuery{
		WorkspaceID: ws,
		ProjectID:   c.QueryParam("project_id"),
		Stream:      c.QueryParam("stream"),
		ActorID:     c.QueryParam("actor_id"),
		Type:        c.QueryParam("type"),
		Types:       csvParam(c, "types"),
		Cursor:      c.QueryParam("cursor"),
	}

	q.Limit, _ = pageParams(c, 0, maxListPageSize)

	result, err := s.ActivityStore.List(ctx, q)
	if err != nil {
		return serverErr(c, err)
	}
	if result.Activities == nil {
		result.Activities = []bstore.Activity{}
	}

	// Include new activity count for the indicator badge.
	resp := map[string]any{
		"activities":  result.Activities,
		"next_cursor": result.NextCursor,
	}
	userID, _ := c.Get("user_id").(string)
	if userID != "" {
		if wsObj, err := s.AuthStore.GetWorkspaceBySlug(ctx, ws); err == nil {
			newCount, _ := s.ActivityStore.CountNewActivities(ctx, userID, wsObj.ID)
			resp["new_count"] = newCount
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleMarkActivitiesSeen records that the user has viewed the activity feed.
// POST /api/v1/:ws/activities/seen
func (s *Server) HandleMarkActivitiesSeen(c echo.Context) error {
	if s.ActivityStore == nil {
		return c.NoContent(http.StatusNoContent)
	}

	ws := c.Param("ws")
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.NoContent(http.StatusNoContent)
	}

	ctx := c.Request().Context()
	wsObj, err := s.AuthStore.GetWorkspaceBySlug(ctx, ws)
	if err != nil {
		return c.NoContent(http.StatusNoContent)
	}

	_ = s.ActivityStore.SetActivitySeenAt(ctx, userID, wsObj.ID, time.Now())
	return c.NoContent(http.StatusNoContent)
}
