package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
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
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

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
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

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
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

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
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

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

// HandleLockStream locks a stream to prevent further content changes.
// POST /api/v1/projects/:id/streams/:stream/lock
func (s *Server) HandleLockStream(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	userID := ""
	if claims, ok := c.Get("user_claims").(map[string]interface{}); ok {
		if sub, ok := claims["sub"].(string); ok {
			userID = sub
		}
	}

	if err := s.ContentStore.LockStream(c.Request().Context(), projectID, streamName, userID); err != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}

	st, err := s.ContentStore.GetStream(c.Request().Context(), projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, st)
}

// HandleUnlockStream unlocks a previously locked stream.
// POST /api/v1/projects/:id/streams/:stream/unlock
func (s *Server) HandleUnlockStream(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	if err := s.ContentStore.UnlockStream(c.Request().Context(), projectID, streamName); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	st, err := s.ContentStore.GetStream(c.Request().Context(), projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, st)
}

// HandleListStreamTags returns all tags for a stream.
// GET /api/v1/projects/:id/streams/:stream/tags
func (s *Server) HandleListStreamTags(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream")

	tags, err := s.ContentStore.ListStreamTags(c.Request().Context(), projectID, streamName)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if tags == nil {
		tags = []*store.StreamTag{}
	}

	return c.JSON(http.StatusOK, tags)
}

// HandleCreateStreamTag creates a new tag on a stream.
// POST /api/v1/projects/:id/streams/:stream/tags
func (s *Server) HandleCreateStreamTag(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")

	var req struct {
		Name     string            `json:"name"`
		Stream   string            `json:"stream"` // AD-040: stream specified in body, not URL
		Kind     string            `json:"kind"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "tag name is required"})
	}
	// AD-040: stream comes from body (tags are project-level peers to streams).
	// Fall back to :stream param for backward compat with old nested routes.
	streamName := req.Stream
	if streamName == "" {
		streamName = c.Param("stream")
	}
	if streamName == "" {
		streamName = "main"
	}

	kind := store.TagKindCustom
	if req.Kind != "" {
		kind = store.StreamTagKind(req.Kind)
	}

	createdBy := ""
	if claims, ok := c.Get("user_claims").(map[string]interface{}); ok {
		if sub, ok := claims["sub"].(string); ok {
			createdBy = sub
		}
	}

	cursor, _ := s.ContentStore.LatestCursor(c.Request().Context(), projectID, streamName)

	tag := &store.StreamTag{
		ProjectID: projectID,
		Stream:    streamName,
		Name:      req.Name,
		Kind:      kind,
		Cursor:    cursor,
		Metadata:  req.Metadata,
		CreatedBy: createdBy,
	}

	if err := s.ContentStore.CreateStreamTag(c.Request().Context(), tag); err != nil {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, tag)
}

// HandleGetStreamTag returns a single tag.
// AD-040: GET /:ws/:id/tags/:tag (project-level, no stream in URL)
func (s *Server) HandleGetStreamTag(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream") // empty for new routes, present for legacy
	tagName := c.Param("tag")

	tag, err := s.ContentStore.GetStreamTag(c.Request().Context(), projectID, streamName, tagName)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, tag)
}

// HandleDeleteStreamTag removes a tag.
// AD-040: DELETE /:ws/:id/tags/:tag (project-level, no stream in URL)
func (s *Server) HandleDeleteStreamTag(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageStreams); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	streamName := c.Param("stream") // empty for new routes, present for legacy
	tagName := c.Param("tag")

	if err := s.ContentStore.DeleteStreamTag(c.Request().Context(), projectID, streamName, tagName); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleListProjectTags returns all tags across all streams in a project.
// GET /api/v1/projects/:id/tags
func (s *Server) HandleListProjectTags(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	projectID := c.Param("id")
	kind := store.StreamTagKind(c.QueryParam("kind"))

	tags, err := s.ContentStore.ListProjectTags(c.Request().Context(), projectID, kind)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if tags == nil {
		tags = []*store.StreamTag{}
	}

	return c.JSON(http.StatusOK, tags)
}
