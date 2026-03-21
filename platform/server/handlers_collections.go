package server

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/neokapi/neokapi/platform/store"
)

// CollectionResponse is the API response for a collection.
type CollectionResponse struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id"`
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	ItemLabel       string            `json:"item_label"`
	IsDefault       bool              `json:"is_default"`
	Stream          string            `json:"stream,omitempty"`
	ConnectorConfig map[string]string `json:"connector_config,omitempty"`
	ItemCount       int               `json:"item_count"`
	CreatedAt       string            `json:"created_at"`
	UpdatedAt       string            `json:"updated_at"`
}

// CreateCollectionRequest is the request body for creating a collection.
type CreateCollectionRequest struct {
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	ItemLabel       string            `json:"item_label,omitempty"`
	Stream          string            `json:"stream,omitempty"`
	ConnectorConfig map[string]string `json:"connector_config,omitempty"`
}

func collectionToResponse(c *store.Collection) CollectionResponse {
	return CollectionResponse{
		ID:              c.ID,
		ProjectID:       c.ProjectID,
		Name:            c.Name,
		Kind:            string(c.Kind),
		ItemLabel:       c.ItemLabel,
		IsDefault:       c.IsDefault,
		Stream:          c.Stream,
		ConnectorConfig: c.ConnectorConfig,
		CreatedAt:       c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// HandleListCollections returns all collections for a project, filtered by stream.
func (s *Server) HandleListCollections(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("pid")
	stream := streamParam(c)
	ctx := c.Request().Context()

	colls, err := s.ContentStore.ListCollections(ctx, pid, stream)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Count items per collection.
	items, _ := s.ContentStore.ListItems(ctx, pid, stream)
	itemCounts := map[string]int{}
	for _, item := range items {
		itemCounts[item.CollectionID]++
	}

	result := make([]CollectionResponse, len(colls))
	for i, coll := range colls {
		result[i] = collectionToResponse(coll)
		result[i].ItemCount = itemCounts[coll.ID]
	}

	return c.JSON(http.StatusOK, result)
}

// HandleCreateCollection creates a new collection in a project.
func (s *Server) HandleCreateCollection(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("pid")
	var req CreateCollectionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}
	if req.Kind == "" {
		req.Kind = "uploaded"
	}
	if req.Kind != "uploaded" && req.Kind != "connected" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "kind must be 'uploaded' or 'connected'"})
	}

	coll := &store.Collection{
		ProjectID:       pid,
		Name:            req.Name,
		Kind:            store.CollectionKind(req.Kind),
		ItemLabel:       req.ItemLabel,
		Stream:          req.Stream,
		ConnectorConfig: req.ConnectorConfig,
	}

	if err := s.ContentStore.CreateCollection(c.Request().Context(), coll); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusCreated, collectionToResponse(coll))
}

// HandleGetCollection returns a single collection.
func (s *Server) HandleGetCollection(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("pid")
	cid := c.Param("cid")

	coll, err := s.ContentStore.GetCollection(c.Request().Context(), pid, cid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, collectionToResponse(coll))
}

// HandleUpdateCollection updates an existing collection.
func (s *Server) HandleUpdateCollection(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("pid")
	cid := c.Param("cid")
	ctx := c.Request().Context()

	coll, err := s.ContentStore.GetCollection(ctx, pid, cid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	var req CreateCollectionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if req.Name != "" {
		coll.Name = req.Name
	}
	if req.Kind != "" {
		coll.Kind = store.CollectionKind(req.Kind)
	}
	if req.ItemLabel != "" {
		coll.ItemLabel = req.ItemLabel
	}
	if req.ConnectorConfig != nil {
		coll.ConnectorConfig = req.ConnectorConfig
	}
	// Stream is intentionally not updatable after creation.

	if err := s.ContentStore.UpdateCollection(ctx, coll); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, collectionToResponse(coll))
}

// HandleDeleteCollection deletes a collection, reassigning its items to the default.
func (s *Server) HandleDeleteCollection(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("pid")
	cid := c.Param("cid")

	if err := s.ContentStore.DeleteCollection(c.Request().Context(), pid, cid); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// HandleUploadToCollection uploads files to a specific collection.
// Only allowed for "uploaded" collections.
func (s *Server) HandleUploadToCollection(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	pid := c.Param("pid")
	cid := c.Param("cid")
	ctx := c.Request().Context()

	// Verify collection exists and is uploaded kind.
	coll, err := s.ContentStore.GetCollection(ctx, pid, cid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if coll.Kind != store.CollectionUploaded {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "cannot upload to a connected collection"})
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "multipart form required"})
	}

	files := make(map[string][]byte)
	for _, fh := range form.File["files"] {
		f, err := fh.Open()
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("open %q: %s", fh.Filename, err)})
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: fmt.Sprintf("read %q: %s", fh.Filename, err)})
		}
		files[fh.Filename] = data
	}

	if len(files) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no files uploaded"})
	}

	info, err := editorAddFilesToCollection(ctx, s.ContentStore, s.FormatRegistry, pid, streamParam(c), cid, files)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, info)
}

// EnsureDefaultCollection creates the default collection for a project if it doesn't exist.
func EnsureDefaultCollection(ctx context.Context, cs store.ContentStore, projectID string) error {
	_, err := cs.GetDefaultCollection(ctx, projectID)
	if err == nil {
		return nil // already exists
	}
	return cs.CreateCollection(ctx, &store.Collection{
		ProjectID: projectID,
		Name:      "default",
		Kind:      store.CollectionUploaded,
		ItemLabel: "item",
		IsDefault: true,
	})
}

// EnsureMainStream creates the "main" stream for a project if it doesn't exist.
func EnsureMainStream(ctx context.Context, cs store.ContentStore, projectID string) error {
	_, err := cs.GetStream(ctx, projectID, "main")
	if err == nil {
		return nil // already exists
	}
	return cs.CreateStream(ctx, &store.Stream{
		ProjectID:  projectID,
		Name:       "main",
		Visibility: store.StreamPublic,
	})
}
