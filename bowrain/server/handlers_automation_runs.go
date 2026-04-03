package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// HandleListAutomationRuns returns automation runs for a project.
func (s *Server) HandleListAutomationRuns(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"runs": []any{}})
	}

	projectID := c.Param("id")
	status := c.QueryParam("status")
	limit := 20
	offset := 0
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.QueryParam("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	runs, err := s.AutomationRunStore.ListRuns(c.Request().Context(), projectID, status, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if runs == nil {
		runs = []*bstore.AutomationRun{}
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": runs})
}

// HandleGetAutomationRun returns a single automation run with its steps.
func (s *Server) HandleGetAutomationRun(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "automation runs not configured"})
	}

	runID := c.Param("runId")

	run, err := s.AutomationRunStore.GetRun(c.Request().Context(), runID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	steps, err := s.AutomationRunStore.ListSteps(c.Request().Context(), runID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if steps == nil {
		steps = []*bstore.AutomationStep{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"run":   run,
		"steps": steps,
	})
}

// HandleListAutomationRunSteps returns steps for a run.
func (s *Server) HandleListAutomationRunSteps(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"steps": []any{}})
	}

	runID := c.Param("runId")
	steps, err := s.AutomationRunStore.ListSteps(c.Request().Context(), runID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if steps == nil {
		steps = []*bstore.AutomationStep{}
	}
	return c.JSON(http.StatusOK, map[string]any{"steps": steps})
}

// HandleListStepLogs returns logs for a step.
func (s *Server) HandleListStepLogs(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"logs": []any{}})
	}

	stepID := c.Param("stepId")
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	logs, err := s.AutomationRunStore.ListLogs(c.Request().Context(), stepID, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if logs == nil {
		logs = []bstore.AutomationLog{}
	}
	return c.JSON(http.StatusOK, map[string]any{"logs": logs})
}

// HandleCancelAutomationRun cancels a running automation run.
func (s *Server) HandleCancelAutomationRun(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "automation runs not configured"})
	}

	runID := c.Param("runId")
	if err := s.AutomationRunStore.UpdateRunStatus(c.Request().Context(), runID, bstore.RunStatusFailed, "cancelled by user"); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
