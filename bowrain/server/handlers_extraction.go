package server

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
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

	var req ExtractionSettings
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	_, _, err := s.ContentStore.EditProject(ctx, projectID, func(proj *store.Project) (bool, error) {
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

		return true, nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return apiErr(c, http.StatusNotFound, errNoProject.Error())
	}
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, req)
}
