package server

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/service"
)

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

type createConversationRequest struct {
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
}

// HandleCreateBravoConversation creates a new @bravo conversation.
func (s *Server) HandleCreateBravoConversation(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	userID := c.Get("user_id").(string)
	wsID := c.Get("workspace_id").(string)

	var req createConversationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	conv, err := s.AgentService.CreateConversation(c.Request().Context(), wsID, userID, req.ProjectID, req.Title)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, conv)
}

// HandleListBravoConversations lists conversations for the current user.
func (s *Server) HandleListBravoConversations(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusOK, map[string]any{"conversations": []any{}, "total": 0})
	}

	userID := c.Get("user_id").(string)
	wsID := c.Get("workspace_id").(string)

	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	convs, total, err := s.AgentService.ListConversations(c.Request().Context(), wsID, userID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Force empty array instead of null in JSON.
	if convs == nil {
		return c.JSON(http.StatusOK, map[string]any{
			"conversations": []any{},
			"total":         0,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"conversations": convs,
		"total":         total,
	})
}

// HandleGetBravoConversation gets a conversation with recent messages.
func (s *Server) HandleGetBravoConversation(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	convID := c.Param("id")
	conv, err := s.AgentService.GetConversation(c.Request().Context(), convID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "conversation not found"})
	}

	// Fetch recent messages.
	msgs, err := s.AgentService.ListMessages(c.Request().Context(), convID, 50, 0)
	if err != nil {
		msgs = nil
	}

	return c.JSON(http.StatusOK, map[string]any{
		"conversation": conv,
		"messages":     msgs,
	})
}

// HandleDeleteBravoConversation deletes a conversation.
func (s *Server) HandleDeleteBravoConversation(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	convID := c.Param("id")
	if err := s.AgentService.DeleteConversation(c.Request().Context(), convID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type sendMessageRequest struct {
	Content string              `json:"content"`
	Mode    string              `json:"mode,omitempty"`    // "ask", "coworker", "bravo"
	Context *sendMessageContext `json:"context,omitempty"` // current workspace context
}

type sendMessageContext struct {
	ProjectID string `json:"projectId,omitempty"`
	Stream    string `json:"stream,omitempty"`
	ItemID    string `json:"itemId,omitempty"`
}

// HandleSendBravoMessage sends a message and returns the agent response.
// When Accept: text/event-stream is requested, streams SSE events.
// Otherwise, returns JSON with user + assistant messages.
func (s *Server) HandleSendBravoMessage(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	convID := c.Param("id")
	userID := c.Get("user_id").(string)

	var req sendMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.Content == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "content is required"})
	}

	// SSE streaming mode.
	accept := c.Request().Header.Get("Accept")
	if accept == "text/event-stream" {
		wsID, _ := c.Get("workspace_id").(string)
		wsRole := ""
		if r, ok := c.Get("workspace_role").(platauth.Role); ok {
			wsRole = string(r)
		}

		c.Response().Header().Set("Content-Type", "text/event-stream")
		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Connection", "keep-alive")
		c.Response().WriteHeader(http.StatusOK)

		sse := service.NewSSEWriter(c.Response())
		// Build context map from request.
		var bravoCtx map[string]string
		if req.Context != nil {
			bravoCtx = map[string]string{}
			if req.Context.ProjectID != "" {
				bravoCtx["project_id"] = req.Context.ProjectID
			}
			if req.Context.Stream != "" {
				bravoCtx["stream"] = req.Context.Stream
			}
			if req.Context.ItemID != "" {
				bravoCtx["item_id"] = req.Context.ItemID
			}
		}

		// Create or update session grant based on the requested mode.
		if s.SessionStore != nil && req.Mode != "" {
			userPerms, _ := c.Get("project_permissions").(platauth.Permission)
			if userPerms == 0 {
				userPerms = platauth.DefaultPermissionsForRole(platauth.Role(wsRole)).Permissions
			}
			userLangs, _ := c.Get("project_languages").([]string)
			grant := CreateSessionGrantForMode(convID, userID, platauth.AgentMode(req.Mode), userPerms, userLangs)
			_ = SetSessionGrant(c.Request().Context(), s.SessionStore, grant)
			s.emitAudit(c, auditEvent{
				Type:         platev.EventSessionGrantCreated,
				ResourceType: "session_grant",
				ResourceID:   convID,
				Data:         map[string]string{"mode": req.Mode, "permissions": grant.Permissions.String()},
			})
		}

		if err := s.AgentService.SendMessageStream(
			c.Request().Context(), convID, userID, wsID, wsRole, req.Content, req.Mode, bravoCtx, sse,
		); err != nil {
			slog.Error("bravo stream failed", "error", err)
			_ = sse.WriteEvent(service.SSEError, service.ErrorData{Error: err.Error()})
		}
		return nil
	}

	// JSON mode (backward-compatible with Phase 1 clients).
	userMsg, assistantMsg, err := s.AgentService.SendMessage(c.Request().Context(), convID, userID, req.Content)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"user_message":      userMsg,
		"assistant_message": assistantMsg,
	})
}

// HandleListBravoMessages lists messages in a conversation.
func (s *Server) HandleListBravoMessages(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusOK, map[string]any{"messages": []any{}})
	}

	convID := c.Param("id")
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	msgs, err := s.AgentService.ListMessages(c.Request().Context(), convID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{"messages": msgs})
}

// ---------------------------------------------------------------------------
// Tool call approval
// ---------------------------------------------------------------------------

// HandleApproveBravoToolCall approves a gated tool call.
func (s *Server) HandleApproveBravoToolCall(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	convID := c.Param("id")
	tcID := c.Param("tcid")
	userID := c.Get("user_id").(string)

	if err := s.AgentService.ApproveToolCall(c.Request().Context(), convID, tcID, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "approved"})
}

// HandleDenyBravoToolCall denies a gated tool call.
func (s *Server) HandleDenyBravoToolCall(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	convID := c.Param("id")
	tcID := c.Param("tcid")
	userID := c.Get("user_id").(string)

	if err := s.AgentService.DenyToolCall(c.Request().Context(), convID, tcID, userID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "denied"})
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// HandleCancelBravoConversation cancels a running agent.
func (s *Server) HandleCancelBravoConversation(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	convID := c.Param("id")
	if err := s.AgentService.CancelConversation(c.Request().Context(), convID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
}

// ---------------------------------------------------------------------------
// Mode
// ---------------------------------------------------------------------------

// HandleUpdateBravoMode updates the session grant mode for a conversation.
// Used for step-up prompting: when @bravo suggests switching modes, the frontend
// calls this endpoint to update the session grant without starting a new conversation.
func (s *Server) HandleUpdateBravoMode(c echo.Context) error {
	if s.SessionStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "session store not configured"})
	}

	convID := c.Param("id")
	userID, _ := c.Get("user_id").(string)

	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	mode := platauth.AgentMode(req.Mode)
	if !platauth.ValidAgentModes[mode] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid mode: must be ask, coworker, or voice"})
	}

	// Get the user's base permissions to create the new grant.
	wsRole := ""
	if r, ok := c.Get("workspace_role").(platauth.Role); ok {
		wsRole = string(r)
	}
	userPerms, _ := c.Get("project_permissions").(platauth.Permission)
	if userPerms == 0 {
		userPerms = platauth.DefaultPermissionsForRole(platauth.Role(wsRole)).Permissions
	}
	userLangs, _ := c.Get("project_languages").([]string)

	grant := CreateSessionGrantForMode(convID, userID, mode, userPerms, userLangs)
	if err := SetSessionGrant(c.Request().Context(), s.SessionStore, grant); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to update session grant"})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventSessionGrantCreated,
		ResourceType: "session_grant",
		ResourceID:   convID,
		Data:         map[string]string{"mode": req.Mode, "permissions": grant.Permissions.String()},
	})

	return c.JSON(http.StatusOK, map[string]any{
		"mode":        req.Mode,
		"permissions": grant.Permissions.Strings(),
	})
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// HandleGetBravoConfig returns the workspace agent config.
func (s *Server) HandleGetBravoConfig(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	wsID := c.Get("workspace_id").(string)
	cfg, err := s.AgentService.GetConfig(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, cfg)
}

// HandleUpdateBravoConfig updates the workspace agent config (admin/owner only).
func (s *Server) HandleUpdateBravoConfig(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	wsID := c.Get("workspace_id").(string)

	var cfg struct {
		Enabled         *bool    `json:"enabled"`
		AllowedTools    []string `json:"allowed_tools"`
		DeniedTools     []string `json:"denied_tools"`
		RequireApproval []string `json:"require_approval"`
		CodeExecEnabled *bool    `json:"code_exec_enabled"`
		MaxConcurrent   *int     `json:"max_concurrent"`
	}
	if err := c.Bind(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	ctx := c.Request().Context()
	existing, err := s.AgentService.GetConfig(ctx, wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	if cfg.Enabled != nil {
		existing.Enabled = *cfg.Enabled
	}
	if cfg.AllowedTools != nil {
		existing.AllowedTools = cfg.AllowedTools
	}
	if cfg.DeniedTools != nil {
		existing.DeniedTools = cfg.DeniedTools
	}
	if cfg.RequireApproval != nil {
		existing.RequireApproval = cfg.RequireApproval
	}
	if cfg.CodeExecEnabled != nil {
		existing.CodeExecEnabled = *cfg.CodeExecEnabled
	}
	if cfg.MaxConcurrent != nil {
		existing.MaxConcurrent = *cfg.MaxConcurrent
	}

	if err := s.AgentService.SaveConfig(ctx, existing); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, existing)
}

// ---------------------------------------------------------------------------
// Tools listing
// ---------------------------------------------------------------------------

// HandleListBravoTools lists available agent tools (respects policy).
func (s *Server) HandleListBravoTools(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusOK, map[string]any{"tools": []any{}})
	}

	wsID := c.Get("workspace_id").(string)
	tools, err := s.AgentService.ListAvailableTools(c.Request().Context(), wsID, agentToolNames())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"tools": tools})
}

// agentToolNames returns all registered MCP agent tool names.
func agentToolNames() []string {
	return []string{
		"list_projects", "get_project", "create_project", "update_project",
		"list_blocks", "get_block", "update_block",
		"create_version", "list_streams", "diff_streams", "merge_stream",
		"list_flows", "run_flow", "get_flow_status",
		"tm_search", "tm_import",
		"term_search", "term_add",
		"connector_pull", "connector_push", "connector_status",
		"execute_script",
		"check_vocabulary", "list_profiles", "get_voice_guide",
	}
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

// HandleGetBravoUsage returns usage summary for the workspace.
// Query params: from, to (RFC3339 timestamps). Defaults to current month.
func (s *Server) HandleGetBravoUsage(c echo.Context) error {
	if s.AgentService == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "agent not configured"})
	}

	wsID := c.Get("workspace_id").(string)

	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := now

	if v := c.QueryParam("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := c.QueryParam("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	summary, err := s.AgentService.GetUsageSummary(c.Request().Context(), wsID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, summary)
}

// registerBravoRoutes registers all @bravo agent endpoints on a workspace group.
func (s *Server) registerBravoRoutes(g *echo.Group) {
	bravo := g.Group("/bravo")

	bravo.POST("/conversations", s.HandleCreateBravoConversation)
	bravo.GET("/conversations", s.HandleListBravoConversations)
	bravo.GET("/conversations/:id", s.HandleGetBravoConversation)
	bravo.DELETE("/conversations/:id", s.HandleDeleteBravoConversation)

	// Message sending consumes credits — apply QuotaGuard.
	bravo.POST("/conversations/:id/messages", s.HandleSendBravoMessage, billing.QuotaGuard(s.BillingStore))

	bravo.GET("/conversations/:id/messages", s.HandleListBravoMessages)
	bravo.PATCH("/conversations/:id/mode", s.HandleUpdateBravoMode)
	bravo.POST("/conversations/:id/tool-calls/:tcid/approve", s.HandleApproveBravoToolCall)
	bravo.POST("/conversations/:id/tool-calls/:tcid/deny", s.HandleDenyBravoToolCall)
	bravo.POST("/conversations/:id/cancel", s.HandleCancelBravoConversation)
	bravo.GET("/config", s.HandleGetBravoConfig)
	bravo.PUT("/config", s.HandleUpdateBravoConfig)
	bravo.GET("/tools", s.HandleListBravoTools)
	bravo.GET("/usage", s.HandleGetBravoUsage)
}
