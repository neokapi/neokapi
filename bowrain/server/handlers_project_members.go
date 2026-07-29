package server

import (
	"context"
	"log/slog"
	"net/http"

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

	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      req.UserID,
		RoleID:      req.RoleID,
		WorkspaceID: workspaceID,
		Languages:   req.Languages,
	}

	if err := s.AuthStore.AddProjectMember(c.Request().Context(), pm); err != nil {
		return serverErr(c, err)
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberAdded,
		ProjectID:    projectID,
		ResourceType: "project_member",
		ResourceID:   req.UserID,
		Data:         map[string]string{"role_id": req.RoleID, "scope": "project"},
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

	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      userID,
		RoleID:      req.RoleID,
		WorkspaceID: workspaceID,
		Languages:   req.Languages,
	}

	if err := s.AuthStore.UpdateProjectMember(c.Request().Context(), pm); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventMemberRoleChanged,
		ProjectID:    projectID,
		ResourceType: "project_member",
		ResourceID:   userID,
		After:        map[string]string{"role_id": req.RoleID, "scope": "project"},
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
