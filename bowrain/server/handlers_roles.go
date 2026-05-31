package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// RoleTemplateRequest is the request body for creating or updating a role template.
type RoleTemplateRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"` // permission names, e.g. ["translate", "review"]
	Position    int      `json:"position"`
}

// HandleListRoleTemplates returns all role templates for the workspace.
func (s *Server) HandleListRoleTemplates(c echo.Context) error {
	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	templates, err := s.AuthStore.ListRoleTemplates(ctx, workspaceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Convert to response with permission names instead of bitmask.
	type roleTemplateResponse struct {
		*platauth.RoleTemplate
		PermissionNames []string `json:"permission_names"`
	}
	result := make([]roleTemplateResponse, len(templates))
	for i, rt := range templates {
		result[i] = roleTemplateResponse{
			RoleTemplate:    rt,
			PermissionNames: rt.Permissions.Strings(),
		}
	}
	return c.JSON(http.StatusOK, result)
}

// HandleCreateRoleTemplate creates a custom role template. Admin/owner only.
func (s *Server) HandleCreateRoleTemplate(c echo.Context) error {
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	var req RoleTemplateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	perms := platauth.ParsePermissions(req.Permissions)

	// Privilege-escalation guard: an admin cannot mint a role granting
	// permissions beyond their own effective workspace permissions.
	wsRole, _ := c.Get("workspace_role").(platauth.Role)
	creatorPerms := platauth.DefaultPermissionsForRole(wsRole).Permissions
	if override, ok, _ := s.AuthStore.GetWorkspaceRoleOverride(c.Request().Context(), workspaceID, wsRole); ok {
		creatorPerms = override
	}
	if !creatorPerms.Has(perms) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "cannot grant permissions beyond your own"})
	}

	rt := &platauth.RoleTemplate{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Permissions: perms,
		Position:    req.Position,
	}

	if err := s.AuthStore.CreateRoleTemplate(c.Request().Context(), rt); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventRoleTemplateCreated,
		ResourceType: "role_template",
		ResourceID:   rt.ID,
		Data:         map[string]string{"name": rt.Name},
		After:        map[string]string{"permissions": rt.Permissions.String()},
	})
	return c.JSON(http.StatusCreated, rt)
}

// HandleUpdateRoleTemplate updates a role template. Admin/owner only.
func (s *Server) HandleUpdateRoleTemplate(c echo.Context) error {
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	var req RoleTemplateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	roleID := c.Param("rid")

	ctx := c.Request().Context()
	rt, err := s.AuthStore.GetRoleTemplate(ctx, workspaceID, roleID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "role template not found"})
	}
	beforePerms := rt.Permissions.String()

	if req.Name != "" {
		rt.Name = req.Name
	}
	if req.DisplayName != "" {
		rt.DisplayName = req.DisplayName
	}
	if req.Description != "" {
		rt.Description = req.Description
	}
	if len(req.Permissions) > 0 {
		rt.Permissions = platauth.ParsePermissions(req.Permissions)
	}
	rt.Position = req.Position

	if err := s.AuthStore.UpdateRoleTemplate(ctx, rt); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventRoleTemplateUpdated,
		ResourceType: "role_template",
		ResourceID:   rt.ID,
		Data:         map[string]string{"name": rt.Name},
		Before:       map[string]string{"permissions": beforePerms},
		After:        map[string]string{"permissions": rt.Permissions.String()},
	})
	return c.JSON(http.StatusOK, rt)
}

// HandleDeleteRoleTemplate deletes a custom (non-builtin) role template. Admin/owner only.
func (s *Server) HandleDeleteRoleTemplate(c echo.Context) error {
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	roleID := c.Param("rid")

	if err := s.AuthStore.DeleteRoleTemplate(c.Request().Context(), workspaceID, roleID); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type:         platev.EventRoleTemplateDeleted,
		ResourceType: "role_template",
		ResourceID:   roleID,
	})
	return c.NoContent(http.StatusNoContent)
}
