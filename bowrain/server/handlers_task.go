package server

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// taskWorkspaceID is the value the tasks table is keyed by: the workspace's
// **id**, resolved by WorkspaceAccessMiddleware from the :ws slug.
//
// Every server-side task writer keys on the id — createSourceProposalTask and
// the automation fan-out both pass proj.WorkspaceID — so a queue that read
// c.Param("ws") (the slug) would match none of them and answer /:ws/tasks with
// an empty list. The slug is kept only as the fallback for the handful of
// routes that run without the workspace middleware.
func taskWorkspaceID(c echo.Context) string {
	if wsID, _ := c.Get("workspace_id").(string); wsID != "" {
		return wsID
	}
	return c.Param("ws")
}

// taskAssigneeParam reads ?assignee_id, resolving the literal "me" to the
// authenticated user. ok is false when "me" has no user to resolve against —
// "my tasks" is then empty, never the whole workspace list.
//
// "me" mirrors the review queue's assigned_to=me: /:ws/tasks?assignee_id=me is
// the "my tasks" surface (the dedicated /my/tasks route was folded into this
// filter, Bowrain AD-011), so the literal must never reach the store, where it
// would match nothing.
func taskAssigneeParam(c echo.Context) (assignee string, ok bool) {
	assignee = c.QueryParam("assignee_id")
	if assignee != "me" {
		return assignee, true
	}
	assignee, _ = c.Get("user_id").(string)
	return assignee, assignee != ""
}

// taskQueryFromRequest builds the filter half of a task query — everything the
// list and the status counts share. Paging is the caller's to add.
//
// ?types=a,b,c is the multi-type filter the review inbox and the dashboard need
// (review, review_terms, source_review are one set); ?type stays the single
// exact match and applies when ?types is absent.
func taskQueryFromRequest(c echo.Context, ws, assignee string) bstore.TaskQuery {
	return bstore.TaskQuery{
		WorkspaceID: ws,
		ProjectID:   c.QueryParam("project_id"),
		AssigneeID:  assignee,
		Status:      c.QueryParam("status"),
		Statuses:    csvParam(c, "statuses"),
		Type:        c.QueryParam("type"),
		Types:       csvParam(c, "types"),
		Priority:    c.QueryParam("priority"),
	}
}

// HandleListTasks returns tasks for a workspace, optionally filtered.
func (s *Server) HandleListTasks(c echo.Context) error {
	// The same bstore.TaskResult the live path returns, so an unconfigured
	// deployment answers the shape a client already parses — next_cursor
	// omitted, not an empty string that reads as a cursor.
	if s.TaskStore == nil {
		return c.JSON(http.StatusOK, bstore.TaskResult{Tasks: []bstore.Task{}})
	}

	ws := taskWorkspaceID(c)
	ctx := c.Request().Context()

	assignee, ok := taskAssigneeParam(c)
	if !ok {
		return c.JSON(http.StatusOK, bstore.TaskResult{Tasks: []bstore.Task{}})
	}

	q := taskQueryFromRequest(c, ws, assignee)
	q.Cursor = c.QueryParam("cursor")
	q.Limit, _ = pageParams(c, 0, maxListPageSize)

	result, err := s.TaskStore.List(ctx, q)
	if err != nil {
		return serverErr(c, err)
	}
	if result.Tasks == nil {
		result.Tasks = []bstore.Task{}
	}

	return c.JSON(http.StatusOK, result)
}

// HandleGetTaskCounts returns how many tasks match the same filters
// HandleListTasks accepts, grouped by status.
//
// A board's column headers count the whole column, not the page it has loaded,
// so they read this rather than tallying the rows in hand.
// GET /api/v1/:ws/tasks/counts
func (s *Server) HandleGetTaskCounts(c echo.Context) error {
	empty := &bstore.TaskCounts{ByStatus: map[string]int{}}
	if s.TaskStore == nil {
		return c.JSON(http.StatusOK, empty)
	}

	assignee, ok := taskAssigneeParam(c)
	if !ok {
		return c.JSON(http.StatusOK, empty)
	}

	counts, err := s.TaskStore.Counts(c.Request().Context(), taskQueryFromRequest(c, taskWorkspaceID(c), assignee))
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, counts)
}

// HandleCreateTask creates a new task.
func (s *Server) HandleCreateTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	ws := taskWorkspaceID(c)
	userID, _ := c.Get("user_id").(string)

	var req struct {
		ProjectID   string            `json:"project_id"`
		Stream      string            `json:"stream"`
		Type        string            `json:"type"`
		Priority    string            `json:"priority"`
		Title       string            `json:"title"`
		Description string            `json:"description"`
		AssigneeID  string            `json:"assignee_id"`
		Data        map[string]string `json:"data"`
		DueAt       string            `json:"due_at"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.Title == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "title is required"})
	}
	if req.ProjectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project_id is required"})
	}

	task := &bstore.Task{
		WorkspaceID: ws,
		ProjectID:   req.ProjectID,
		Stream:      req.Stream,
		Type:        bstore.TaskType(req.Type),
		Priority:    bstore.TaskPriority(req.Priority),
		Title:       req.Title,
		Description: req.Description,
		AssigneeID:  req.AssigneeID,
		CreatedBy:   userID,
		Data:        req.Data,
	}

	if req.DueAt != "" {
		if t, err := time.Parse(time.RFC3339, req.DueAt); err == nil {
			task.DueAt = &t
		}
	}

	if err := s.TaskStore.Create(c.Request().Context(), task); err != nil {
		return serverErr(c, err)
	}

	// Notify assignee if set.
	if s.NotificationDispatcher != nil && task.AssigneeID != "" && task.AssigneeID != userID {
		s.NotificationDispatcher.DispatchTaskNotification(
			c.Request().Context(), task,
			bstore.NotificationTaskAssigned,
			"Task assigned: "+task.Title,
			"You have been assigned a new task",
		)
	}

	return c.JSON(http.StatusCreated, task)
}

// HandleGetTask returns a single task.
func (s *Server) HandleGetTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	taskID := c.Param("taskId")
	task, err := s.TaskStore.Get(c.Request().Context(), taskID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "task not found"})
	}

	return c.JSON(http.StatusOK, task)
}

// HandleUpdateTask updates a task.
func (s *Server) HandleUpdateTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	taskID := c.Param("taskId")
	ctx := c.Request().Context()

	task, err := s.TaskStore.Get(ctx, taskID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "task not found"})
	}

	var req struct {
		Priority    *string           `json:"priority"`
		Title       *string           `json:"title"`
		Description *string           `json:"description"`
		AssigneeID  *string           `json:"assignee_id"`
		Data        map[string]string `json:"data"`
		DueAt       *string           `json:"due_at"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	if req.Priority != nil {
		task.Priority = bstore.TaskPriority(*req.Priority)
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.AssigneeID != nil {
		task.AssigneeID = *req.AssigneeID
	}
	if req.Data != nil {
		task.Data = req.Data
	}
	if req.DueAt != nil {
		if *req.DueAt == "" {
			task.DueAt = nil
		} else if t, err := time.Parse(time.RFC3339, *req.DueAt); err == nil {
			task.DueAt = &t
		}
	}

	if err := s.TaskStore.Update(ctx, task); err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, task)
}

// HandleDeleteTask removes a task.
func (s *Server) HandleDeleteTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	taskID := c.Param("taskId")
	if err := s.TaskStore.Delete(c.Request().Context(), taskID); err != nil {
		return serverErr(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleAssignTask assigns a task to a user.
func (s *Server) HandleAssignTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	taskID := c.Param("taskId")
	var req struct {
		AssigneeID string `json:"assignee_id"`
	}
	if err := c.Bind(&req); err != nil || req.AssigneeID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "assignee_id is required"})
	}

	if err := s.TaskStore.Assign(c.Request().Context(), taskID, req.AssigneeID); err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleCompleteTask marks a task as completed.
func (s *Server) HandleCompleteTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	taskID := c.Param("taskId")
	userID, _ := c.Get("user_id").(string)

	if err := s.TaskStore.Complete(c.Request().Context(), taskID, userID); err != nil {
		return serverErr(c, err)
	}

	// Bowrain AD-014: Emit source.review.completed when a source review task is completed,
	// allowing automation rules to fan out per-locale tasks.
	if s.EventBus != nil {
		task, err := s.TaskStore.Get(c.Request().Context(), taskID)
		if err == nil && task.Type == bstore.TaskSourceReview {
			s.EventBus.Publish(platev.Event{
				Type:        platev.EventSourceReviewCompleted,
				Source:      "task",
				ProjectID:   task.ProjectID,
				Actor:       userID,
				Data:        task.Data,
				CausationID: event.NextCausationID(platev.Event{ID: task.ID}),
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}

// HandleCancelTask cancels a task.
func (s *Server) HandleCancelTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	taskID := c.Param("taskId")
	if err := s.TaskStore.Cancel(c.Request().Context(), taskID); err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
