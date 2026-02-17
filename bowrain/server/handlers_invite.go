package server

import (
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

	return c.JSON(http.StatusCreated, inv)
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
