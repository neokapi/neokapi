package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/platform/store"
)

// HandleListStreams returns all streams for a project.
// GET /api/v1/projects/:id/streams?include_archived=true
func (s *Server) HandleListStreams(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	includeArchived := c.QueryParam("include_archived") == "true"

	streams, err := s.ContentStore.ListStreams(c.Request().Context(), projectID, includeArchived)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, streams)
}

// HandleCreateStream creates a new stream in a project.
// POST /api/v1/projects/:id/streams
func (s *Server) HandleCreateStream(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")

	var req struct {
		Name        string `json:"name"`
		Parent      string `json:"parent"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "stream name is required"})
	}
	if req.Parent == "" {
		req.Parent = "main"
	}

	visibility := store.StreamPublic
	if req.Visibility != "" {
		visibility = store.StreamVisibility(req.Visibility)
	}

	// Get the parent's latest cursor for branching.
	baseCursor, _ := s.ContentStore.LatestCursor(c.Request().Context(), projectID, req.Parent)

	createdBy := ""
	if claims, ok := c.Get("user_claims").(map[string]interface{}); ok {
		if sub, ok := claims["sub"].(string); ok {
			createdBy = sub
		}
	}

	st := &store.Stream{
		ProjectID:   projectID,
		Name:        req.Name,
		Parent:      req.Parent,
		BaseCursor:  baseCursor,
		Visibility:  visibility,
		Description: req.Description,
		CreatedBy:   createdBy,
	}

	if err := s.ContentStore.CreateStream(c.Request().Context(), st); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, st)
}

// HandleGetStream returns a single stream.
// GET /api/v1/projects/:id/streams/:stream
func (s *Server) HandleGetStream(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	st, err := s.ContentStore.GetStream(c.Request().Context(), projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, st)
}

// HandleUpdateStream updates a stream's metadata.
// PATCH /api/v1/projects/:id/streams/:stream
func (s *Server) HandleUpdateStream(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")
	ctx := c.Request().Context()

	st, err := s.ContentStore.GetStream(ctx, projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	var req struct {
		Description *string `json:"description"`
		Visibility  *string `json:"visibility"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Description != nil {
		st.Description = *req.Description
	}
	if req.Visibility != nil {
		st.Visibility = store.StreamVisibility(*req.Visibility)
	}

	if err := s.ContentStore.UpdateStream(ctx, st); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, st)
}

// HandleArchiveStream archives (soft-deletes) a stream.
// DELETE /api/v1/projects/:id/streams/:stream
func (s *Server) HandleArchiveStream(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	if streamName == "main" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cannot archive the main stream"})
	}

	ctx := c.Request().Context()
	stream, err := s.ContentStore.GetStream(ctx, projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	stream.Archived = true
	if err := s.ContentStore.UpdateStream(ctx, stream); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleMergeStream merges a stream into its parent.
// POST /api/v1/projects/:id/streams/:stream/merge
func (s *Server) HandleMergeStream(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	var req struct {
		DryRun bool `json:"dry_run"`
	}
	_ = c.Bind(&req)

	result, err := s.ContentStore.MergeStream(c.Request().Context(), projectID, streamName, store.MergeOptions{
		DryRun: req.DryRun,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

// HandleDiffStream returns the diff between a stream and its parent.
// GET /api/v1/projects/:id/streams/:stream/diff
func (s *Server) HandleDiffStream(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	diff, err := s.ContentStore.DiffStream(c.Request().Context(), projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, diff)
}
