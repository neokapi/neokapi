package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/labstack/echo/v4"
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

	var req InviteRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	userID, _ := c.Get("user_id").(string)

	role := auth.Role(req.Role)
	if !auth.ValidRoles[role] {
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

	// Send invite email asynchronously if email is provided and SMTP is configured.
	if inv.Email != "" && s.EmailSender != nil {
		baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)
		go s.sendInviteEmail(context.Background(), inv, baseURL)
	}

	return c.JSON(http.StatusCreated, inv)
}

// sendInviteEmail sends an HTML email with the invite link.
func (s *Server) sendInviteEmail(ctx context.Context, inv *auth.Invite, baseURL string) {
	joinURL := fmt.Sprintf("%s/join/%s", baseURL, inv.Code)

	subject := fmt.Sprintf("You've been invited to join a workspace on Bowrain")

	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 480px; margin: 0 auto; padding: 24px;">
<h2 style="margin-bottom: 8px;">You're Invited</h2>
<p>You've been invited to join a workspace on Bowrain as <strong>%s</strong>.</p>
<p style="margin: 24px 0;">
  <a href="%s" style="display: inline-block; padding: 12px 24px; background-color: #2563eb; color: #ffffff; text-decoration: none; border-radius: 6px; font-weight: 500;">
    Accept Invitation
  </a>
</p>
<p style="color: #6b7280; font-size: 13px;">
  Or copy this link: <a href="%s">%s</a>
</p>
</body>
</html>`, string(inv.Role), joinURL, joinURL, joinURL)

	if err := s.EmailSender.Send(ctx, inv.Email, subject, body); err != nil {
		log.Printf("failed to send invite email to %s: %v", inv.Email, err)
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

// HandleDeleteInvite revokes an invitation.
func (s *Server) HandleDeleteInvite(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	inviteID := c.Param("id")
	ctx := c.Request().Context()

	if err := s.AuthStore.DeleteInvite(ctx, inviteID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

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

	return c.JSON(http.StatusOK, map[string]string{"status": "joined"})
}
