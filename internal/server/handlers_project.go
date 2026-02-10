package server

import (
	"net/http"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
	"github.com/labstack/echo/v4"
)

// ProjectRequest is the request body for creating/updating a project.
type ProjectRequest struct {
	Name          string   `json:"name"`
	SourceLocale  string   `json:"source_locale"`
	TargetLocales []string `json:"target_locales"`
}

// BlocksRequest is the request body for storing blocks.
type BlocksRequest struct {
	Blocks []BlockInput `json:"blocks"`
}

// BlockInput represents a block in the API input.
type BlockInput struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// VersionRequest is the request body for creating a version.
type VersionRequest struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (s *Server) HandleCreateProject(c echo.Context) error {
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

	p := &store.Project{
		Name:          req.Name,
		SourceLocale:  model.LocaleID(req.SourceLocale),
		TargetLocales: locales,
	}
	if err := s.Services.Project.CreateProject(c.Request().Context(), p); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusCreated, p)
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

	locales := make([]model.LocaleID, len(req.TargetLocales))
	for i, l := range req.TargetLocales {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		ID:            c.Param("id"),
		Name:          req.Name,
		SourceLocale:  model.LocaleID(req.SourceLocale),
		TargetLocales: locales,
	}
	if err := s.Services.Project.UpdateProject(c.Request().Context(), p); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
