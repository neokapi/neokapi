package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// ActivityListResponse is one page of a workspace's activity feed.
//
// NextCursor is omitted on the last page, matching bstore.ActivityResult and
// every other cursor-paged answer this server gives (tasks, review queue,
// automation history): "no next page" is the key's absence, never an empty
// string. NewCount is a pointer because it is only meaningful for an
// identified user. An anonymous read omits it rather than reporting zero
// unseen activities, which would be a claim the server has not made.
type ActivityListResponse struct {
	Activities []bstore.Activity `json:"activities"`
	NextCursor string            `json:"next_cursor,omitempty"`
	NewCount   *int              `json:"new_count,omitempty"`
}

// HandleListActivities returns activities for a workspace, optionally filtered.
func (s *Server) HandleListActivities(c echo.Context) error {
	if s.ActivityStore == nil {
		return c.JSON(http.StatusOK, ActivityListResponse{Activities: []bstore.Activity{}})
	}

	ws := c.Param("ws")
	ctx := c.Request().Context()
	wsID := s.activityWorkspaceID(ctx, c, ws)

	// ?type is one prefix ("block" matches block.*); ?types is a comma-separated
	// set of prefixes, for feeds that span families: a review inbox wants
	// review.* and task.*, which no single prefix expresses.
	q := bstore.ActivityQuery{
		WorkspaceID:   ws,
		WorkspaceKeys: []string{wsID},
		ProjectID:     c.QueryParam("project_id"),
		Stream:        c.QueryParam("stream"),
		ActorID:       c.QueryParam("actor_id"),
		Type:          c.QueryParam("type"),
		Types:         csvParam(c, "types"),
		Cursor:        c.QueryParam("cursor"),
	}

	q.Limit, _ = pageParams(c, 0, maxListPageSize)

	result, err := s.ActivityStore.List(ctx, q)
	if errors.Is(err, bstore.ErrInvalidActivityCursor) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cursor is not one this endpoint returned"})
	}
	if err != nil {
		return serverErr(c, err)
	}
	if result.Activities == nil {
		result.Activities = []bstore.Activity{}
	}

	resp := ActivityListResponse{
		Activities: result.Activities,
		NextCursor: result.NextCursor,
	}
	// The indicator badge's unseen count, for an identified user only.
	userID, _ := c.Get("user_id").(string)
	if userID != "" && wsID != "" {
		newCount, _ := s.ActivityStore.CountNewActivities(ctx, userID, wsID, ws)
		resp.NewCount = &newCount
	}

	return c.JSON(http.StatusOK, resp)
}

// activityWorkspaceID returns the id of the workspace whose slug is slug, or
// "" when it cannot be found. The recorder files an activity under the slug
// when the event carries one and under the workspace id otherwise, so every
// activity read matches both keys.
func (s *Server) activityWorkspaceID(ctx context.Context, c echo.Context, slug string) string {
	if wsID, _ := c.Get("workspace_id").(string); wsID != "" {
		return wsID
	}
	if s.AuthStore == nil {
		return ""
	}
	wsObj, err := s.AuthStore.GetWorkspaceBySlug(ctx, slug)
	if err != nil {
		return ""
	}
	return wsObj.ID
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
