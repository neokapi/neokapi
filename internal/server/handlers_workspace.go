package server

import (
	"net/http"

	"github.com/gokapi/gokapi/core/auth"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
	"github.com/labstack/echo/v4"
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

	w := &auth.Workspace{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		LogoURL:     req.LogoURL,
	}

	// Add the creator as owner of the new workspace.
	userID, _ := c.Get("user_id").(string)
	if s.Services != nil && s.Services.Auth != nil && userID != "" {
		if err := s.Services.Auth.CreateWorkspaceWithOwner(c.Request().Context(), w, userID); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	} else {
		if err := s.AuthStore.CreateWorkspace(c.Request().Context(), w); err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}
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
	return c.JSON(http.StatusOK, w)
}

func (s *Server) HandleUpdateWorkspace(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
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
	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
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

	w, err := s.AuthStore.GetWorkspaceBySlug(c.Request().Context(), c.Param("ws"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	role := auth.Role(req.Role)
	if err := s.AuthStore.AddMember(c.Request().Context(), w.ID, req.UserID, role); err != nil {
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

	role := auth.Role(req.Role)
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

	locales := make([]model.LocaleID, len(req.TargetLocales))
	for i, l := range req.TargetLocales {
		locales[i] = model.LocaleID(l)
	}

	workspaceID, _ := c.Get("workspace_id").(string)
	p := &store.Project{
		Name:          req.Name,
		SourceLocale:  model.LocaleID(req.SourceLocale),
		TargetLocales: locales,
		WorkspaceID:   workspaceID,
	}
	if err := s.Services.Project.CreateProject(c.Request().Context(), p); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, p)
}
