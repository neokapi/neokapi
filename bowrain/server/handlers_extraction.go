package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// ExtractionSettings holds extraction configuration for a project.
// These are stored as project properties with the "extraction_" prefix.
type ExtractionSettings struct {
	Enabled  bool   `json:"enabled"`  // auto_extract != "false"
	Model    string `json:"model"`    // extraction_model
	Provider string `json:"provider"` // extraction_provider
}

// HandleGetExtractionSettings returns the extraction configuration for a project.
func (s *Server) HandleGetExtractionSettings(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	proj, err := s.ContentStore.GetProject(c.Request().Context(), projectID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}

	settings := ExtractionSettings{
		Enabled: true, // default
	}

	if proj.Properties != nil {
		if proj.Properties["auto_extract"] == "false" {
			settings.Enabled = false
		}
		settings.Model = proj.Properties["extraction_model"]
		settings.Provider = proj.Properties["extraction_provider"]
	}

	return c.JSON(http.StatusOK, settings)
}

// HandleUpdateExtractionSettings updates the extraction configuration for a project.
func (s *Server) HandleUpdateExtractionSettings(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageProject); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	ctx := c.Request().Context()

	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}

	var req ExtractionSettings
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if proj.Properties == nil {
		proj.Properties = make(map[string]string)
	}

	if req.Enabled {
		delete(proj.Properties, "auto_extract")
	} else {
		proj.Properties["auto_extract"] = "false"
	}

	if req.Model != "" {
		proj.Properties["extraction_model"] = req.Model
	} else {
		delete(proj.Properties, "extraction_model")
	}

	if req.Provider != "" {
		proj.Properties["extraction_provider"] = req.Provider
	} else {
		delete(proj.Properties, "extraction_provider")
	}

	if err := s.ContentStore.UpdateProject(ctx, proj); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, req)
}
