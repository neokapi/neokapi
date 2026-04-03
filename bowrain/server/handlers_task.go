package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// HandleListTasks returns tasks for a workspace, optionally filtered.
func (s *Server) HandleListTasks(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"tasks": []any{}, "next_cursor": ""})
	}

	ws := c.Param("ws")
	ctx := c.Request().Context()

	q := bstore.TaskQuery{
		WorkspaceID: ws,
		ProjectID:   c.QueryParam("project_id"),
		AssigneeID:  c.QueryParam("assignee_id"),
		Status:      c.QueryParam("status"),
		Type:        c.QueryParam("type"),
		Priority:    c.QueryParam("priority"),
		Cursor:      c.QueryParam("cursor"),
	}

	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			q.Limit = parsed
		}
	}

	result, err := s.TaskStore.List(ctx, q)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if result.Tasks == nil {
		result.Tasks = []bstore.Task{}
	}

	return c.JSON(http.StatusOK, result)
}

// HandleCreateTask creates a new task.
func (s *Server) HandleCreateTask(c echo.Context) error {
	if s.TaskStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "tasks not configured"})
	}

	ws := c.Param("ws")
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// AD-034: Emit source.review.completed when a source review task is completed,
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
