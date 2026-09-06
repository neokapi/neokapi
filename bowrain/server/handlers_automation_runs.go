package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// HandleListAutomationRuns returns automation runs for a project.
func (s *Server) HandleListAutomationRuns(c echo.Context) error {
	if s.AutomationRunStore == nil {
		return c.JSON(http.StatusOK, map[string]any{"runs": []any{}})
	}

	projectID := c.Param("id")
	status := c.QueryParam("status")
	limit, offset := pageParams(c, 20, maxListPageSize)

	runs, err := s.AutomationRunStore.ListRuns(c.Request().Context(), projectID, status, limit, offset)
	if err != nil {
		return serverErr(c, err)
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
		return serverErr(c, err)
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
		return serverErr(c, err)
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
	limit, _ := pageParams(c, 100, maxListPageSize)

	logs, err := s.AutomationRunStore.ListLogs(c.Request().Context(), stepID, limit)
	if err != nil {
		return serverErr(c, err)
	}
	if logs == nil {
		logs = []bstore.AutomationLog{}
	}
	return c.JSON(http.StatusOK, map[string]any{"logs": logs})
}

// HandleCancelAutomationRun stops a running automation run. The run manager
// owns the transition, so the run's stream subscribers see it, and it is the
// manager that cancels the work rather than only relabelling the record.
//
// A run that has already finished answers 409: there is nothing left to stop,
// and its recorded outcome stands.
func (s *Server) HandleCancelAutomationRun(c echo.Context) error {
	if s.AutomationRunStore == nil || s.runManager == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "automation runs not configured"})
	}

	runID := c.Param("runId")
	switch err := s.runManager.CancelRun(c.Request().Context(), runID, "cancelled by user"); {
	case err == nil:
	case errors.Is(err, event.ErrRunNotCancellable):
		return apiErr(c, http.StatusConflict, "the run has already finished")
	default:
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
