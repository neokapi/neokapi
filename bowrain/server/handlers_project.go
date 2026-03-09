package server

import (
	"net/http"
	"strings"

	"github.com/gokapi/gokapi/core/model"
	platauth "github.com/gokapi/gokapi/platform/auth"
	apiclient "github.com/gokapi/gokapi/platform/client"
	platev "github.com/gokapi/gokapi/platform/event"
	"github.com/gokapi/gokapi/platform/store"
	"github.com/labstack/echo/v4"
)

// ProjectRequest is the request body for creating/updating a project.
type ProjectRequest struct {
	Name          string   `json:"name"`
	SourceLocale  string   `json:"source_locale"`
	TargetLocales []string `json:"target_locales"`
	Workspace     string   `json:"workspace,omitempty"`
}

// BlocksRequest is the request body for storing blocks.
type BlocksRequest struct {
	Blocks []apiclient.BlockInput `json:"blocks"`
}

// VersionRequest is the request body for creating a version.
type VersionRequest struct {
	Label       string `json:"label"`
	Description string `json:"description"`
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
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list workspaces: " + err.Error()})
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

	locales := make([]model.LocaleID, len(req.TargetLocales))
	for i, l := range req.TargetLocales {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		Name:          req.Name,
		SourceLocale:  model.LocaleID(req.SourceLocale),
		TargetLocales: locales,
		WorkspaceID:   targetWS.ID,
	}
	if err := s.Services.Project.CreateProject(ctx, p); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"id":             p.ID,
		"name":           p.Name,
		"source_locale":  string(p.SourceLocale),
		"target_locales": req.TargetLocales,
		"workspace_id":   p.WorkspaceID,
		"workspace_slug": targetWS.Slug,
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

	locales := make([]model.LocaleID, len(req.TargetLocales))
	for i, l := range req.TargetLocales {
		locales[i] = model.LocaleID(l)
	}

	workspaceID := existing.WorkspaceID // preserve existing workspace association
	if req.Workspace != "" && s.AuthStore != nil {
		// Allow setting/changing workspace via slug.
		ws, err := s.AuthStore.GetWorkspaceBySlug(ctx, req.Workspace)
		if err == nil {
			workspaceID = ws.ID
		}
	}

	p := &store.Project{
		ID:            projectID,
		Name:          req.Name,
		SourceLocale:  model.LocaleID(req.SourceLocale),
		TargetLocales: locales,
		WorkspaceID:   workspaceID,
	}
	if err := s.Services.Project.UpdateProject(ctx, p); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Detect new locales and publish event.
	if s.EventBus != nil {
		oldLocales := make(map[model.LocaleID]bool)
		for _, l := range existing.TargetLocales {
			oldLocales[l] = true
		}
		var newLocales []string
		for _, l := range locales {
			if !oldLocales[l] {
				newLocales = append(newLocales, string(l))
			}
		}
		if len(newLocales) > 0 {
			wsSlug := ""
			if ws, ok := c.Get("workspace_slug").(string); ok {
				wsSlug = ws
			}
			s.EventBus.Publish(platev.Event{
				Type:      platev.EventProjectUpdated,
				Source:    "api",
				ProjectID: projectID,
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
	if err := s.Services.Project.DeleteProject(c.Request().Context(), c.Param("id")); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleStoreBlocks(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req BlocksRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	blocks := make([]*model.Block, len(req.Blocks))
	for i, bi := range req.Blocks {
		b := model.NewBlock(bi.ID, bi.Text)
		b.Name = bi.Name
		b.Type = bi.Type
		blocks[i] = b
	}

	projectID := c.Param("id")
	if err := s.Services.Project.StoreBlocks(c.Request().Context(), projectID, blocks); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]int{"stored": len(blocks)})
}

func (s *Server) HandleGetBlocks(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	blocks, err := s.Services.Project.GetBlocks(c.Request().Context(), store.BlockQuery{
		ProjectID: c.Param("id"),
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, blocks)
}

func (s *Server) HandleCreateVersion(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req VersionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	v, err := s.Services.Project.CreateVersion(c.Request().Context(), c.Param("id"), req.Label, req.Description)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, v)
}

func (s *Server) HandleListVersions(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	versions, err := s.Services.Project.ListVersions(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, versions)
}
