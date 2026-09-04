package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/core/id"
)

// errInvalidAutomationRule marks a rule the caller can fix: a missing name or
// trigger, an action type the executor does not run, or a run_flow action
// naming a flow the project cannot see. The handlers answer it with 400.
var errInvalidAutomationRule = errors.New("invalid automation rule")

// automationRuleInput is the body of a create or update request.
type automationRuleInput struct {
	Name       string                      `json:"name"`
	Trigger    string                      `json:"trigger"`
	Conditions []event.AutomationCondition `json:"conditions"`
	Actions    []event.AutomationAction    `json:"actions"`
	Enabled    bool                        `json:"enabled"`
}

// validateAutomationRule rejects a rule the engine could not run as written,
// so a rule that saves is a rule that fires. A run_flow action must name a
// flow that resolves under this project: a built-in one, or one authored on
// the project. Store failures during resolution are returned unwrapped so the
// handler answers them as server errors.
func (s *Server) validateAutomationRule(ctx context.Context, projectID string, in automationRuleInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name is required", errInvalidAutomationRule)
	}
	if strings.TrimSpace(in.Trigger) == "" {
		return fmt.Errorf("%w: trigger is required", errInvalidAutomationRule)
	}
	if len(in.Actions) == 0 {
		return fmt.Errorf("%w: at least one action is required", errInvalidAutomationRule)
	}
	for i, a := range in.Actions {
		if !knownAutomationActions[a.Type] {
			return fmt.Errorf("%w: action %d: unsupported action type %q", errInvalidAutomationRule, i+1, a.Type)
		}
		if a.Type != "run_flow" {
			continue
		}
		flowID := strings.TrimSpace(a.Config["flow"])
		if flowID == "" {
			return fmt.Errorf("%w: action %d: run_flow names no flow", errInvalidAutomationRule, i+1)
		}
		if _, err := s.flowCatalog().Get(ctx, projectID, flowID); err != nil {
			if errors.Is(err, service.ErrFlowNotFound) {
				return fmt.Errorf("%w: action %d: %w", errInvalidAutomationRule, i+1, err)
			}
			return err
		}
	}
	return nil
}

// ruleValidationResponse answers a validation failure: 400 for a rule the
// caller can fix, 500 for a store failure met while checking it.
func ruleValidationResponse(c echo.Context, err error) error {
	if errors.Is(err, errInvalidAutomationRule) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	return serverErr(c, err)
}

// HandleListAutomationRules returns all automation rules for a project.
func (s *Server) HandleListAutomationRules(c echo.Context) error {
	projectID := c.Param("id")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusOK, []event.StoredRule{})
	}

	rules, err := s.AutomationRuleStore.ListRules(c.Request().Context(), projectID)
	if err != nil {
		return serverErr(c, err)
	}
	if rules == nil {
		rules = []event.StoredRule{}
	}
	return c.JSON(http.StatusOK, rules)
}

// HandleCreateAutomationRule creates a new automation rule.
func (s *Server) HandleCreateAutomationRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}

	projectID := c.Param("id")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "automation store not configured"})
	}

	var req automationRuleInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := s.validateAutomationRule(c.Request().Context(), projectID, req); err != nil {
		return ruleValidationResponse(c, err)
	}

	rule := &event.StoredRule{
		ID:         id.New(),
		ProjectID:  projectID,
		Name:       req.Name,
		Trigger:    platev.EventType(req.Trigger),
		Conditions: req.Conditions,
		Actions:    req.Actions,
		Enabled:    req.Enabled,
	}

	if err := s.AutomationRuleStore.CreateRule(c.Request().Context(), rule); err != nil {
		return serverErr(c, err)
	}
	s.reloadAutomationRules()

	return c.JSON(http.StatusCreated, rule)
}

// HandleUpdateAutomationRule updates an existing rule.
func (s *Server) HandleUpdateAutomationRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}

	projectID := c.Param("id")
	ruleID := c.Param("ruleId")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "automation store not configured"})
	}

	var req automationRuleInput
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := s.validateAutomationRule(c.Request().Context(), projectID, req); err != nil {
		return ruleValidationResponse(c, err)
	}

	rule := &event.StoredRule{
		ID:         ruleID,
		ProjectID:  projectID,
		Name:       req.Name,
		Trigger:    platev.EventType(req.Trigger),
		Conditions: req.Conditions,
		Actions:    req.Actions,
		Enabled:    req.Enabled,
	}

	if err := s.AutomationRuleStore.UpdateRule(c.Request().Context(), rule); err != nil {
		return serverErr(c, err)
	}
	s.reloadAutomationRules()

	return c.JSON(http.StatusOK, rule)
}

// HandleDeleteAutomationRule deletes a custom rule.
func (s *Server) HandleDeleteAutomationRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}

	ruleID := c.Param("ruleId")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "automation store not configured"})
	}

	if err := s.AutomationRuleStore.DeleteRule(c.Request().Context(), ruleID); err != nil {
		return serverErr(c, err)
	}
	s.reloadAutomationRules()

	return c.NoContent(http.StatusNoContent)
}

// HandleToggleAutomationRule enables or disables a rule.
func (s *Server) HandleToggleAutomationRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}

	ruleID := c.Param("ruleId")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "automation store not configured"})
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if err := s.AutomationRuleStore.ToggleRule(c.Request().Context(), ruleID, req.Enabled); err != nil {
		return serverErr(c, err)
	}
	s.reloadAutomationRules()

	return c.NoContent(http.StatusOK)
}

// HandleListAutomationEvents returns available event types for automation triggers.
//
// Only event types that are actually emitted somewhere are offered:
// EventFlowCompleted / EventFlowFailed are defined on the bus but not yet
// emitted by any flow-execution path, so they are intentionally absent here
// until an emitter exists (see AD-013, "Trigger events").
func (s *Server) HandleListAutomationEvents(c echo.Context) error {
	events := []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}{
		{string(platev.EventPushCompleted), "When content is pushed"},
		{string(platev.EventPullCompleted), "When content is pulled"},
		{string(platev.EventProjectUpdated), "When project settings change"},
		{string(platev.EventQualityGateFail), "When a quality gate fails"},
		{string(platev.EventPushAutomationsCompleted), "When all automations for a push complete"},
		{string(platev.EventSourceReviewCompleted), "When a source review task is completed"},
	}
	return c.JSON(http.StatusOK, events)
}

// HandleListAutomationHistory returns one page of execution history, newest
// first: {entries, next_cursor}. ?limit bounds the page, ?cursor continues from
// a previous one.
func (s *Server) HandleListAutomationHistory(c echo.Context) error {
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusOK, event.HistoryPage{Entries: []event.HistoryEntry{}})
	}

	limit, _ := pageParams(c, 50, maxListPageSize)
	page, err := s.AutomationRuleStore.ListHistory(c.Request().Context(), event.HistoryQuery{
		ProjectID: c.Param("id"),
		Limit:     limit,
		Cursor:    c.QueryParam("cursor"),
	})
	if err != nil {
		return serverErr(c, err)
	}
	if page.Entries == nil {
		page.Entries = []event.HistoryEntry{}
	}
	return c.JSON(http.StatusOK, page)
}
