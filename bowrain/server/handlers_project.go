package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// ProjectRequest is the request body for creating/updating a project.
type ProjectRequest struct {
	Name                  string   `json:"name"`
	DefaultSourceLanguage string   `json:"default_source_language"`
	TargetLanguages       []string `json:"target_languages"`
	DefaultStream         *string  `json:"default_stream,omitempty"`
	DashboardVisibility   string   `json:"dashboard_visibility,omitempty"`
	Workspace             string   `json:"workspace,omitempty"`
}

// HandleCreateProject creates a project in the authenticated user's workspace.
// If a workspace slug is provided in the request, it verifies membership and
// uses that workspace; otherwise it defaults to the user's personal workspace.
func (s *Server) HandleCreateProject(c echo.Context) error {
	if s.Services == nil || s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	var req ProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()

	var targetWS *platauth.Workspace

	if req.Workspace != "" {
		// Resolve workspace by slug and verify membership.
		ws, err := s.AuthStore.GetWorkspaceBySlug(ctx, req.Workspace)
		if err != nil {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: "workspace not found: " + req.Workspace})
		}
		if _, err := s.AuthStore.GetMembership(ctx, ws.ID, userID); err != nil {
			return c.JSON(http.StatusForbidden, ErrorResponse{Error: "not a member of workspace: " + req.Workspace})
		}
		targetWS = ws
	} else {
		// Default to the user's personal workspace.
		workspaces, err := s.AuthStore.ListWorkspaces(ctx, userID)
		if err != nil {
			return serverErr(c, fmt.Errorf("list workspaces: %w", err))
		}
		for _, ws := range workspaces {
			if ws.Type == platauth.WorkspaceTypePersonal {
				targetWS = ws
				break
			}
		}
		if targetWS == nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no personal workspace found"})
		}
	}

	locales := make([]model.LocaleID, len(req.TargetLanguages))
	for i, l := range req.TargetLanguages {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		Name:                  req.Name,
		DefaultSourceLanguage: model.LocaleID(req.DefaultSourceLanguage),
		TargetLanguages:       locales,
		WorkspaceID:           targetWS.ID,
	}
	if err := s.Services.Project.CreateProject(ctx, p); err != nil {
		return serverErr(c, err)
	}

	// The creator becomes a review-capable project member so governed review has
	// someone to assign to (and the creator sees pending review on their
	// dashboard). Best-effort; never blocks creation.
	s.addProjectCreatorMembership(ctx, targetWS.ID, p.ID, userID)

	s.trackEvent(userID, analytics.EventProjectCreated, map[string]any{
		"project_id":      p.ID,
		"workspace_id":    targetWS.ID,
		"project_name":    p.Name,
		"source_language": string(p.DefaultSourceLanguage),
		"target_count":    len(req.TargetLanguages),
		"workspace_slug":  targetWS.Slug,
	})

	return c.JSON(http.StatusCreated, map[string]any{
		"id":                      p.ID,
		"name":                    p.Name,
		"default_source_language": string(p.DefaultSourceLanguage),
		"target_languages":        req.TargetLanguages,
		"workspace_id":            p.WorkspaceID,
		"workspace_slug":          targetWS.Slug,
	})
}

func (s *Server) HandleGetProject(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	p, err := s.Services.Project.GetProject(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, p)
}

func (s *Server) HandleListProjects(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	projects, err := s.Services.Project.ListProjects(c.Request().Context())
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, projects)
}

func (s *Server) HandleUpdateProject(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req ProjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	ctx := c.Request().Context()
	projectID := c.Param("id")

	// Fetch current project to detect new locales.
	existing, err := s.Services.Project.GetProject(ctx, projectID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	// Start from the existing project and apply only non-empty request fields.
	p := existing
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.DefaultSourceLanguage != "" {
		p.DefaultSourceLanguage = model.LocaleID(req.DefaultSourceLanguage)
	}
	if len(req.TargetLanguages) > 0 {
		locales := make([]model.LocaleID, len(req.TargetLanguages))
		for i, l := range req.TargetLanguages {
			locales[i] = model.LocaleID(l)
		}
		p.TargetLanguages = locales
	}
	if req.DefaultStream != nil {
		p.DefaultStream = *req.DefaultStream
	}
	if req.DashboardVisibility != "" {
		if req.DashboardVisibility != string(platauth.DashboardPrivate) && s.AuthStore != nil {
			if ws, wsErr := s.AuthStore.GetWorkspace(ctx, existing.WorkspaceID); wsErr == nil && ws.Type == platauth.WorkspaceTypePersonal {
				return c.JSON(http.StatusForbidden, ErrorResponse{Error: "personal workspaces cannot expose projects publicly"})
			}
		}
		p.DashboardVisibility = req.DashboardVisibility
	}
	if req.Workspace != "" && s.AuthStore != nil {
		if ws, wsErr := s.AuthStore.GetWorkspaceBySlug(ctx, req.Workspace); wsErr == nil {
			p.WorkspaceID = ws.ID
		}
	}
	if err := s.Services.Project.UpdateProject(ctx, p); err != nil {
		return serverErr(c, err)
	}

	// Detect new locales and publish event.
	if s.EventBus != nil {
		oldLocales := make(map[model.LocaleID]bool)
		for _, l := range existing.TargetLanguages {
			oldLocales[l] = true
		}
		var newLocales []string
		for _, l := range p.TargetLanguages {
			if !oldLocales[l] {
				newLocales = append(newLocales, string(l))
			}
		}
		if len(newLocales) > 0 {
			wsSlug := ""
			if ws, ok := c.Get("workspace_slug").(string); ok {
				wsSlug = ws
			}
			userID, _ := c.Get("user_id").(string)
			s.EventBus.Publish(platev.Event{
				Type:      platev.EventProjectUpdated,
				Source:    "api",
				ProjectID: projectID,
				Actor:     userID,
				Data: map[string]string{
					"new_locales":    strings.Join(newLocales, ","),
					"workspace_slug": wsSlug,
				},
			})
		}
	}

	return c.JSON(http.StatusOK, p)
}

func (s *Server) HandleDeleteProject(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	projectID := c.Param("id")
	if err := s.Services.Project.DeleteProject(c.Request().Context(), projectID); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	userID, _ := c.Get("user_id").(string)
	props := analytics.Props("", projectID)
	if wsID, ok := c.Get("workspace_id").(string); ok && wsID != "" {
		props[analytics.PropWorkspaceID] = wsID
	}
	s.trackEvent(userID, analytics.EventProjectDeleted, props)

	return c.NoContent(http.StatusNoContent)
}
