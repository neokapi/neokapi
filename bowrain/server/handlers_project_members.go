package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// addProjectCreatorMembership makes the user who created a project a member of
// it, with a review-capable role. Without this, a wizard/API-created project has
// zero project members, so governed review has no one to assign to and the
// creator (typically the workspace owner) never sees pending review — the
// onboarding gap this closes. Best-effort and idempotent: a lookup/insert error
// is logged, never fatal to project creation, and an existing membership is left
// untouched.
func (s *Server) addProjectCreatorMembership(ctx context.Context, workspaceID, projectID, userID string) {
	if s.AuthStore == nil || workspaceID == "" || projectID == "" || userID == "" {
		return
	}
	if _, err := s.AuthStore.GetProjectMembership(ctx, projectID, userID); err == nil {
		return // already a member (e.g. re-entrant create)
	}
	roleID := s.reviewCapableRoleTemplate(ctx, workspaceID)
	if roleID == "" {
		slog.WarnContext(ctx, "project creator membership: no review-capable role template",
			"project", projectID, "workspace", workspaceID)
		return
	}
	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      userID,
		RoleID:      roleID,
		WorkspaceID: workspaceID,
	}
	if err := s.AuthStore.AddProjectMember(ctx, pm); err != nil {
		slog.WarnContext(ctx, "project creator membership: add member failed",
			"project", projectID, "user", userID, "error", err)
	}
}

// reviewCapableRoleTemplate resolves a workspace role template for a project
// creator: the built-in "project-admin" (full control) when present, else the
// first template carrying PermReview. Returns "" when none qualifies.
func (s *Server) reviewCapableRoleTemplate(ctx context.Context, workspaceID string) string {
	templates, err := s.AuthStore.ListRoleTemplates(ctx, workspaceID)
	if err != nil {
		return ""
	}
	reviewRole := ""
	for _, rt := range templates {
		if rt.Name == "project-admin" {
			return rt.ID
		}
		if reviewRole == "" && rt.Permissions.Has(platauth.PermReview) {
			reviewRole = rt.ID
		}
	}
	return reviewRole
}

// ProjectMemberRequest is the request body for adding or updating a project member.
type ProjectMemberRequest struct {
	UserID    string   `json:"user_id"`
	RoleID    string   `json:"role_id"`
	Languages []string `json:"languages,omitempty"` // empty = all languages
	// Coordinates binds the member to a region of the project's context space —
	// a partial point, empty meaning the whole space. Combined with a
	// coordinate-scoped role it is what makes this member a custodian of that
	// region: `{"brand": "acme", "channel": "support"}`.
	Coordinates map[string]string `json:"coordinates,omitempty"`
}

// coordinatesFrom validates a request's region and returns it as a filter. Axis
// names and values are required to be non-empty: a half-written axis would
// silently widen custody, since an axis a filter does not name is an axis it
// does not constrain.
func coordinatesFrom(raw map[string]string) (platauth.CoordinateFilter, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(platauth.CoordinateFilter, len(raw))
	for axis, value := range raw {
		axis, value = strings.TrimSpace(axis), strings.TrimSpace(value)
		if axis == "" || value == "" {
			return nil, errors.New("coordinates must name a non-empty axis and value")
		}
		out[axis] = value
	}
	return out, nil
}

// HandleListProjectMembers returns all members of a project.
func (s *Server) HandleListProjectMembers(c echo.Context) error {
	projectID := projectParam(c)
	ctx := c.Request().Context()

	members, err := s.AuthStore.ListProjectMembers(ctx, projectID)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, members)
}

// HandleAddProjectMember adds a member to a project. Requires PermManageMembers.
func (s *Server) HandleAddProjectMember(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMembers); err != nil {
		return err
	}

	var req ProjectMemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.UserID == "" || req.RoleID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user_id and role_id are required"})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	projectID := projectParam(c)

	// Verify the role template exists in this workspace.
	if _, err := s.AuthStore.GetRoleTemplate(c.Request().Context(), workspaceID, req.RoleID); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid role_id"})
	}

	coords, err := coordinatesFrom(req.Coordinates)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	// Members are free and uncapped; custody is not. This is the only place a
	// plan meters people.
	if err := s.guardCustodianSeat(c, projectID, req.RoleID, coords, req.UserID); err != nil {
		return err
	}

	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      req.UserID,
		RoleID:      req.RoleID,
		WorkspaceID: workspaceID,
		Languages:   req.Languages,
		Coordinates: coords,
	}

	if err := s.AuthStore.AddProjectMember(c.Request().Context(), pm); err != nil {
		return serverErr(c, err)
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberAdded,
		ProjectID:    projectID,
		ResourceType: "project_member",
		ResourceID:   req.UserID,
		Data: map[string]string{
			"role_id": req.RoleID,
			"scope":   "project",
			// The region is part of what was granted, so it belongs in the audit
			// line beside the role — "added as reviewer" and "added as reviewer
			// for acme support" are different grants.
			"coordinates": coords.String(),
		},
	})
	return c.JSON(http.StatusCreated, pm)
}

// HandleUpdateProjectMember updates a project member's role or language scope.
// Requires PermManageMembers.
func (s *Server) HandleUpdateProjectMember(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMembers); err != nil {
		return err
	}

	var req ProjectMemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	projectID := projectParam(c)
	userID := c.Param("uid")

	if req.RoleID != "" {
		// Verify the role template exists.
		if _, err := s.AuthStore.GetRoleTemplate(c.Request().Context(), workspaceID, req.RoleID); err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid role_id"})
		}
	}

	coords, err := coordinatesFrom(req.Coordinates)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.guardCustodianSeat(c, projectID, req.RoleID, coords, userID); err != nil {
		return err
	}

	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      userID,
		RoleID:      req.RoleID,
		WorkspaceID: workspaceID,
		Languages:   req.Languages,
		Coordinates: coords,
	}

	if err := s.AuthStore.UpdateProjectMember(c.Request().Context(), pm); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberRoleChanged,
		ProjectID:    projectID,
		ResourceType: "project_member",
		ResourceID:   userID,
		After: map[string]string{
			"role_id":     req.RoleID,
			"scope":       "project",
			"coordinates": coords.String(),
		},
	})
	return c.JSON(http.StatusOK, pm)
}

// HandleRemoveProjectMember removes a member from a project. Requires PermManageMembers.
func (s *Server) HandleRemoveProjectMember(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageMembers); err != nil {
		return err
	}

	projectID := projectParam(c)
	userID := c.Param("uid")

	if err := s.AuthStore.RemoveProjectMember(c.Request().Context(), projectID, userID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberRemoved,
		ProjectID:    projectID,
		ResourceType: "project_member",
		ResourceID:   userID,
		Data:         map[string]string{"scope": "project"},
	})
	return c.NoContent(http.StatusNoContent)
}
