package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/core/model"
	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/neokapi/neokapi/platform/store"
)

// WorkspaceRequest is the request body for creating/updating a workspace.
type WorkspaceRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
}

// MemberRequest is the request body for adding a member to a workspace.
type MemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// RoleUpdateRequest is the request body for updating a member's role.
type RoleUpdateRequest struct {
	Role string `json:"role"`
}

func (s *Server) HandleCreateWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req WorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	w := &platauth.Workspace{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		LogoURL:     req.LogoURL,
	}

	// Add the creator as owner of the new workspace.
	userID, _ := c.Get("user_id").(string)
	ctx := c.Request().Context()
	if s.Services != nil && s.Services.Auth != nil && userID != "" {
		if err := s.Services.Auth.CreateWorkspaceWithOwner(ctx, w, userID); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
		w.Role = platauth.RoleOwner
	} else {
		if err := s.AuthStore.CreateWorkspace(ctx, w); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	// Set up 14-day Pro trial for new workspaces.
	billing.SetupTrial(ctx, s.BillingStore, w.ID)

	return c.JSON(http.StatusCreated, w)
}

func (s *Server) HandleListWorkspaces(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	userID := c.Get("user_id")
	if userID == nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}
	workspaces, err := s.AuthStore.ListWorkspaces(c.Request().Context(), userID.(string))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, workspaces)
}

func (s *Server) HandleGetWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	// Enrich with the current user's role if available.
	if userID, ok := c.Get("user_id").(string); ok && userID != "" {
		if m, err := s.AuthStore.GetMembership(c.Request().Context(), w.ID, userID); err == nil {
			w.Role = m.Role
		}
	}
	return c.JSON(http.StatusOK, w)
}

func (s *Server) HandleUpdateWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	// Verify the calling user has admin or owner role.
	if err := s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner); err != nil {
		return err
	}

	var req WorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Look up workspace by slug.
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	w.Name = req.Name
	w.Slug = req.Slug
	w.Description = req.Description
	w.LogoURL = req.LogoURL
	if err := s.AuthStore.UpdateWorkspace(c.Request().Context(), w); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, w)
}

func (s *Server) HandleDeleteWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	// Verify the calling user has owner role.
	if err := s.requireRole(c, platauth.RoleOwner); err != nil {
		return err
	}

	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if w.Type == platauth.WorkspaceTypePersonal {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "cannot delete personal workspace"})
	}
	if err := s.AuthStore.DeleteWorkspace(c.Request().Context(), w.ID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleListMembers(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	members, err := s.AuthStore.ListMembers(c.Request().Context(), w.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, members)
}

func (s *Server) HandleAddMember(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req MemberRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	w, err := s.AuthStore.GetWorkspaceBySlug(ctx, c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	// Enforce seat limit based on workspace plan.
	if w.Plan != "" {
		limit := billing.GetLimit(billing.Plan(w.Plan), "max-seats")
		if limit > 0 {
			members, err := s.AuthStore.ListMembers(ctx, w.ID)
			if err == nil && len(members) >= limit {
				return c.JSON(http.StatusForbidden, map[string]any{
					"error":   "seat_limit_reached",
					"current": len(members),
					"limit":   limit,
				})
			}
		}
	}

	role := platauth.Role(req.Role)
	if err := s.AuthStore.AddMember(ctx, w.ID, req.UserID, role); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]string{"status": "added"})
}

func (s *Server) HandleUpdateMemberRole(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req RoleUpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	role := platauth.Role(req.Role)
	if err := s.AuthStore.UpdateRole(c.Request().Context(), w.ID, c.Param("uid"), role); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) HandleRemoveMember(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if err := s.AuthStore.RemoveMember(c.Request().Context(), w.ID, c.Param("uid")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// HandleListWorkspaceProjects lists projects in a workspace, filtered by workspace_id.
func (s *Server) HandleListWorkspaceProjects(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	workspaceID, _ := c.Get("workspace_id").(string)
	allProjects, err := s.Services.Project.ListProjects(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	// Filter to only projects belonging to this workspace.
	filtered := make([]*store.Project, 0)
	for _, p := range allProjects {
		if p.WorkspaceID == workspaceID {
			filtered = append(filtered, p)
		}
	}
	return c.JSON(http.StatusOK, filtered)
}

// HandleCreateWorkspaceProject creates a project in a workspace.
func (s *Server) HandleCreateWorkspaceProject(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req ProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	locales := make([]model.LocaleID, len(req.TargetLanguages))
	for i, l := range req.TargetLanguages {
		locales[i] = model.LocaleID(l)
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	// Enforce project limit based on workspace plan.
	plan, _ := c.Get("workspace_plan").(string)
	if plan != "" {
		limit := billing.GetLimit(billing.Plan(plan), "max-projects")
		if limit > 0 {
			allProjects, err := s.Services.Project.ListProjects(ctx)
			if err == nil {
				count := 0
				for _, p := range allProjects {
					if p.WorkspaceID == workspaceID {
						count++
					}
				}
				if count >= limit {
					return c.JSON(http.StatusForbidden, map[string]any{
						"error":   "project_limit_reached",
						"current": count,
						"limit":   limit,
					})
				}
			}
		}
	}

	p := &store.Project{
		Name:                  req.Name,
		DefaultSourceLanguage: model.LocaleID(req.DefaultSourceLanguage),
		TargetLanguages:       locales,
		WorkspaceID:           workspaceID,
	}
	if err := s.Services.Project.CreateProject(ctx, p); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if s.ContentStore != nil {
		_ = EnsureDefaultCollection(ctx, s.ContentStore, p.ID)
		_ = EnsureMainStream(ctx, s.ContentStore, p.ID)
	}
	return c.JSON(http.StatusCreated, p)
}
