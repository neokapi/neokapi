package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/mailer"
)

// InviteRequest is the request body for creating an invitation.
type InviteRequest struct {
	Role    string `json:"role"`
	Email   string `json:"email,omitempty"`
	MaxUses int    `json:"max_uses"`
	TTLDays int    `json:"ttl_days"`
}

// HandleCreateInvite creates an invitation for the workspace.
// Admin/owner only.
func (s *Server) HandleCreateInvite(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	// Verify the calling user has admin or owner role.
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	var req InviteRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)

	role := platauth.Role(req.Role)
	if !platauth.ValidRoles[role] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid role"})
	}

	maxUses := req.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	ttl := time.Duration(req.TTLDays) * 24 * time.Hour
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour // default 7 days
	}

	ctx := c.Request().Context()
	inv, err := s.Services.Auth.CreateInvite(ctx, workspaceID, userID, role, req.Email, maxUses, ttl)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventInviteCreated,
		WorkspaceID:  workspaceID,
		ResourceType: "invite",
		ResourceID:   inv.ID,
		Data:         map[string]string{"role": string(role), "email": req.Email},
	})

	// Send invite email asynchronously if email is provided and mailer is configured.
	if inv.Email != "" && s.Mailer != nil {
		baseURL := requestBaseURL(c)
		wsSlug := c.Param("ws")
		go s.sendInviteEmail(context.WithoutCancel(ctx), inv, baseURL, wsSlug)
	}

	return c.JSON(http.StatusCreated, inv)
}

// sendInviteEmail renders and sends a branded invite email.
func (s *Server) sendInviteEmail(ctx context.Context, inv *platauth.Invite, baseURL, workspaceName string) {
	joinURL := baseURL + "/join/" + inv.Code

	data := mailer.InviteData{
		WorkspaceName: workspaceName,
		Role:          string(inv.Role),
		JoinURL:       joinURL,
	}

	if err := s.Mailer.SendInvite(ctx, inv.Email, data); err != nil {
		slog.Info("failed to send invite email to", "id", inv.Email, "error", err)
	}
}

// HandleListInvites lists all invitations for the workspace.
func (s *Server) HandleListInvites(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	invites, err := s.AuthStore.ListInvites(ctx, workspaceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, invites)
}

// HandleDeleteInvite revokes an invitation. Admin/owner only.
func (s *Server) HandleDeleteInvite(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	inviteID := c.Param("id")
	ctx := c.Request().Context()

	if err := s.AuthStore.DeleteInvite(ctx, inviteID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventInviteRevoked,
		ResourceType: "invite",
		ResourceID:   inviteID,
	})

	return c.NoContent(http.StatusNoContent)
}

// HandleAcceptInvite accepts an invitation and joins the workspace.
// Any authenticated user can accept.
func (s *Server) HandleAcceptInvite(c echo.Context) error {
	if s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	code := c.Param("code")
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	ctx := c.Request().Context()
	if err := s.Services.Auth.AcceptInvite(ctx, code, userID); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventInviteAccepted,
		ResourceType: "invite",
		ResourceID:   code,
		Data:         map[string]string{"user_id": userID},
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "joined"})
}
