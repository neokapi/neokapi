package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// personName is the name to show for the user userID names
// (platauth.ShownName), or "" for an id that names no user.
func (s *Server) personName(ctx context.Context, userID string) string {
	if userID == "" || userID == "system" || s.AuthStore == nil {
		return ""
	}
	u, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	handle := ""
	if strings.TrimSpace(u.Name) == "" {
		handle = s.personHandle(ctx, u.ID)
	}
	return platauth.ShownName(u, handle)
}

// personHandle is the slug of the user's personal workspace, or "" when they
// have none yet.
func (s *Server) personHandle(ctx context.Context, userID string) string {
	workspaces, err := s.AuthStore.ListWorkspaces(ctx, userID)
	if err != nil {
		return ""
	}
	for _, w := range workspaces {
		if w.Type == platauth.WorkspaceTypePersonal {
			return w.Slug
		}
	}
	return ""
}

// meResponse is the signed-in user as GET and PATCH /auth/me return them:
// the account, plus the name to show while they have set none and their
// handle.
type meResponse struct {
	*platauth.User

	ShownName string `json:"shown_name"`
	Handle    string `json:"handle,omitempty"`
}

func (s *Server) meResponseOf(ctx context.Context, u *platauth.User) meResponse {
	handle := s.personHandle(ctx, u.ID)
	return meResponse{User: u, ShownName: platauth.ShownName(u, handle), Handle: handle}
}

// updateMeRequest is the body of PATCH /auth/me. A field left out stays as
// it is.
type updateMeRequest struct {
	Name *string `json:"name"`
}

// HandleUpdateMe changes the signed-in user's own account. Name is the only
// field: an empty name clears it, and the person then shows under their
// handle.
func (s *Server) HandleUpdateMe(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	var req updateMeRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	u, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil || u == nil {
		return apiErr(c, http.StatusNotFound, "user not found")
	}
	if req.Name != nil {
		name, err := platauth.CleanPersonName(*req.Name)
		if err != nil {
			return apiErr(c, http.StatusBadRequest, err.Error())
		}
		u.Name = name
		if err := s.AuthStore.UpdateUser(ctx, u); err != nil {
			return serverErr(c, err)
		}
	}
	return c.JSON(http.StatusOK, s.meResponseOf(ctx, u))
}
