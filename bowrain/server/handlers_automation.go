package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/id"
)

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

	var req struct {
		Name       string                      `json:"name"`
		Trigger    string                      `json:"trigger"`
		Conditions []event.AutomationCondition `json:"conditions"`
		Actions    []event.AutomationAction    `json:"actions"`
		Enabled    bool                        `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
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

	return c.JSON(http.StatusCreated, rule)
}

// HandleUpdateAutomationRule updates an existing rule.
func (s *Server) HandleUpdateAutomationRule(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAutomation); err != nil {
		return err
	}

	ruleID := c.Param("ruleId")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "automation store not configured"})
	}

	var req struct {
		Name       string                      `json:"name"`
		Trigger    string                      `json:"trigger"`
		Conditions []event.AutomationCondition `json:"conditions"`
		Actions    []event.AutomationAction    `json:"actions"`
		Enabled    bool                        `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	rule := &event.StoredRule{
		ID:         ruleID,
		Name:       req.Name,
		Trigger:    platev.EventType(req.Trigger),
		Conditions: req.Conditions,
		Actions:    req.Actions,
		Enabled:    req.Enabled,
	}

	if err := s.AutomationRuleStore.UpdateRule(c.Request().Context(), rule); err != nil {
		return serverErr(c, err)
	}

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
