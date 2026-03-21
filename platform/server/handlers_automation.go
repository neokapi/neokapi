package server

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/core/id"
	platauth "github.com/neokapi/neokapi/platform/auth"
	platev "github.com/neokapi/neokapi/platform/event"
)

// HandleListAutomationRules returns all automation rules for a project.
func (s *Server) HandleListAutomationRules(c echo.Context) error {
	projectID := c.Param("id")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusOK, []event.StoredRule{})
	}

	rules, err := s.AutomationRuleStore.ListRules(c.Request().Context(), projectID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusOK)
}

// HandleListAutomationEvents returns available event types for automation triggers.
func (s *Server) HandleListAutomationEvents(c echo.Context) error {
	events := []struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}{
		{string(platev.EventPushCompleted), "When content is pushed"},
		{string(platev.EventPullCompleted), "When content is pulled"},
		{string(platev.EventProjectUpdated), "When project settings change"},
		{string(platev.EventFlowCompleted), "When a flow finishes"},
		{string(platev.EventFlowFailed), "When a flow fails"},
		{string(platev.EventQualityGateFail), "When a quality gate fails"},
	}
	return c.JSON(http.StatusOK, events)
}

// HandleListAutomationHistory returns recent execution history.
func (s *Server) HandleListAutomationHistory(c echo.Context) error {
	projectID := c.Param("id")
	if s.AutomationRuleStore == nil {
		return c.JSON(http.StatusOK, []event.HistoryEntry{})
	}

	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	entries, err := s.AutomationRuleStore.ListHistory(c.Request().Context(), projectID, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if entries == nil {
		entries = []event.HistoryEntry{}
	}
	return c.JSON(http.StatusOK, entries)
}
