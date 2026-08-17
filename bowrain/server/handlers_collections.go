package server

import (
	"context"
	"maps"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
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
	// ConnectorSecretKeys names the connector_config keys whose values were
	// redacted (credentials are never echoed back over the API). A client learns
	// a secret is set from this list, not its value.
	ConnectorSecretKeys []string `json:"connector_secret_keys,omitempty"`
	ItemCount           int      `json:"item_count"`
	// Origin classifies the collection's source of truth: "connected" (content
	// synced from a connector — read-only source) or "managed" (UI-native, so
	// uploads/edits/deletes are allowed). Editable is the derived gate the UI
	// reads to hide/disable source-mutation affordances. Both fold in the
	// project-level source-connector signal (see annotateProjectOrigin).
	Origin   string `json:"origin"`
	Editable bool   `json:"editable"`
	// Coordinates is the point this collection's content occupies in the
	// project's context space — axis → value, as the recipe declares it
	// (`product`, `channel`). Named as the dashboard rollups and the context
	// profiles name it, because it is the same coordinate.
	//
	// A client reads it to group collections the way the project is actually
	// shaped: a flat list of names spells the coordinate out ("bowrain-app",
	// "neokapi-docs") and then loses it, so two products with six surfaces each
	// read as twelve unrelated tabs. Empty for a collection that sits at no
	// declared point.
	Coordinates map[string]string `json:"coordinates,omitempty"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
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
	cfg, secretKeys := redactConnectorConfig(c.ConnectorConfig)
	origin := collectionOriginManaged
	if collectionConnected(c) {
		origin = collectionOriginConnected
	}
	return CollectionResponse{
		ID:                  c.ID,
		ProjectID:           c.ProjectID,
		Name:                c.Name,
		Kind:                string(c.Kind),
		ItemLabel:           c.ItemLabel,
		IsDefault:           c.IsDefault,
		Stream:              c.Stream,
		ConnectorConfig:     cfg,
		ConnectorSecretKeys: secretKeys,
		Origin:              origin,
		Editable:            origin == collectionOriginManaged,
		Coordinates:         cloneCoordinates(c.Context),
		CreatedAt:           c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// isSecretConnectorKey reports whether a connector_config key holds a
// credential that must never be returned over the API (passwords, tokens, API
// keys, client/private secrets). Matching is on a separator-stripped,
// lower-cased key so api_key / access-token / clientSecret all match.
func isSecretConnectorKey(key string) bool {
	k := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	for _, marker := range []string{
		"password", "passwd", "secret", "token", "credential",
		"apikey", "accesskey", "privatekey",
	} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

// redactConnectorConfig returns a copy of cfg with secret-valued keys removed,
// plus the sorted names of the keys that were redacted. Connector credentials
// must never be echoed back over the API.
func redactConnectorConfig(cfg map[string]string) (map[string]string, []string) {
	if len(cfg) == 0 {
		return cfg, nil
	}
	redacted := make(map[string]string, len(cfg))
	var secretKeys []string
	for k, v := range cfg {
		if isSecretConnectorKey(k) {
			secretKeys = append(secretKeys, k)
			continue
		}
		redacted[k] = v
	}
	sort.Strings(secretKeys)
	return redacted, secretKeys
}

// mergeConnectorConfig folds an incoming connector_config (from an update
// request) onto the stored one. Non-secret keys follow the request (so a key
// can be changed or dropped); secret keys the request omits are carried forward
// from the stored config, so a client that received a redacted config and PUTs
// it back does not wipe stored credentials. A secret the client does resend
// (a rotation) overwrites the stored value.
func mergeConnectorConfig(existing, incoming map[string]string) map[string]string {
	if incoming == nil {
		return existing
	}
	merged := make(map[string]string, len(incoming))
	maps.Copy(merged, incoming)
	for k, v := range existing {
		if _, present := merged[k]; !present && isSecretConnectorKey(k) {
			merged[k] = v
		}
	}
	return merged
}

// HandleListCollections returns all collections for a project, filtered by stream.
func (s *Server) HandleListCollections(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := projectParam(c)
	stream := streamParam(c)
	ctx := c.Request().Context()

	colls, err := s.ContentStore.ListCollections(ctx, pid, stream)
	if err != nil {
		return serverErr(c, err)
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

	// A project bound to a source connector (kapi push / GitHub App / git) makes
	// every collection connector-sourced — read-only source — regardless of its
	// own kind. Apply that project-level override to the origin/editable fields.
	wsID, _ := c.Get("workspace_id").(string)
	if s.projectHasSourceConnector(ctx, wsID, pid) {
		for i := range result {
			result[i].Origin = collectionOriginConnected
			result[i].Editable = false
		}
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

	pid := projectParam(c)
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
		return serverErr(c, err)
	}

	return c.JSON(http.StatusCreated, collectionToResponse(coll))
}

// HandleGetCollection returns a single collection.
func (s *Server) HandleGetCollection(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := projectParam(c)
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

	pid := projectParam(c)
	cid := c.Param("cid")
	ctx := c.Request().Context()

	coll, err := s.ContentStore.GetCollection(ctx, pid, cid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	if refused, rerr := guardRecipeOwned(c, coll); refused {
		return rerr
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
		coll.ConnectorConfig = mergeConnectorConfig(coll.ConnectorConfig, req.ConnectorConfig)
	}
	// Stream is intentionally not updatable after creation.

	if err := s.ContentStore.UpdateCollection(ctx, coll); err != nil {
		return serverErr(c, err)
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

	pid := projectParam(c)
	cid := c.Param("cid")
	ctx := c.Request().Context()

	// A collection the store cannot find is left to DeleteCollection to report,
	// so a missing id still answers the way it always has.
	if coll, err := s.ContentStore.GetCollection(ctx, pid, cid); err == nil {
		if refused, rerr := guardRecipeOwned(c, coll); refused {
			return rerr
		}
	}

	if err := s.ContentStore.DeleteCollection(ctx, pid, cid); err != nil {
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

	pid := projectParam(c)
	cid := c.Param("cid")
	ctx := c.Request().Context()

	// Verify collection exists, then refuse the upload if its source is
	// connector-sourced — either this collection is connector-backed or the
	// whole project is bound to a source connector (kapi push / GitHub App).
	coll, err := s.ContentStore.GetCollection(ctx, pid, cid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	// Ownership first: for a recipe-owned collection the recipe is the more
	// precise authority to name, whatever the project's connector binding says.
	if refused, rerr := guardRecipeOwned(c, coll); refused {
		return rerr
	}
	if refused, rerr := s.guardSourceMutation(c, pid, coll); refused {
		return rerr
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "multipart form required"})
	}

	files, answered, err := readMultipartUploads(c, form)
	if answered {
		return err
	}

	info, err := editorAddFilesToCollection(ctx, s.ContentStore, s.FormatRegistry, pid, streamParam(c), cid, files)
	if err != nil {
		return serverErr(c, err)
	}

	wsID, _ := c.Get("workspace_id").(string)
	s.annotateProjectOrigin(ctx, wsID, info)
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
