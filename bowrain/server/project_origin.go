package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// Project type and collection origin determine who owns source content and
// whether Bowrain may modify it. guardSourceMutation rejects source changes to
// connector-owned content because a later sync would overwrite them. The API
// exposes this restriction so clients can render the source read-only.
//
// UI labels:
//   - Connected: connector-owned source; review and configuration remain editable.
//   - Managed: Bowrain-owned source; uploads, edits and deletions are allowed.
//   - Hybrid: both connected and managed collections.
//
// Origin is stored per collection; project type is derived from those origins.
const (
	collectionOriginConnected = "connected"
	collectionOriginManaged   = "managed"

	projectTypeConnected = "connected"
	projectTypeManaged   = "managed"
	projectTypeHybrid    = "hybrid"
)

// sourceConnectorTypes are the connector types that make a whole project
// connector-sourced: their content is a repository / filesystem that is the
// source of truth (kapi push, the GitHub App, a git or file connector). A
// project bound to one of these is Connected regardless of collection kind.
// The remote CMS connectors (wordpress/figma/hubspot) instead bind per
// collection and are detected by collectionConnected below.
var sourceConnectorTypes = map[string]bool{
	"forge": true,
	"git":   true,
	"file":  true,
}

// collectionConnected reports a collection's own connector-sourced signal
// (signal #2): a connected-kind collection or one carrying a connector_config —
// the CMS-collection case. The project-level source-connector signal (#1) is
// applied on top by the caller.
func collectionConnected(coll *store.Collection) bool {
	if coll == nil {
		return false
	}
	return coll.Kind == store.CollectionConnected || len(coll.ConnectorConfig) > 0
}

// projectSourceConnector returns the first source connector (forge/git/file)
// bound to the project, if any — signal #1, the authoritative "this project's
// source is synced from a repository" query. The binding is the connector
// config's project_id (set by the forge bind + GitHub-App setup and by the
// generic connector bind), exactly as the forge delivery path enumerates it.
//
// It reads the workspace-scoped connector list when a workspace is known
// (cheaper, no cross-tenant scan) and falls back to the cross-workspace list
// otherwise; project ids are globally unique, so the project_id filter is safe
// either way.
func (s *Server) projectSourceConnector(ctx context.Context, workspaceID, projectID string) (bstore.ConnectorConfig, bool) {
	if s.ConnectorConfigStore == nil || projectID == "" {
		return bstore.ConnectorConfig{}, false
	}
	var (
		configs []bstore.ConnectorConfig
		err     error
	)
	if workspaceID != "" {
		configs, err = s.ConnectorConfigStore.List(ctx, workspaceID)
	} else {
		configs, err = s.ConnectorConfigStore.ListAll(ctx)
	}
	if err != nil {
		slog.WarnContext(ctx, "project origin: connector config lookup failed",
			"project", projectID, "workspace", workspaceID, "error", err)
		return bstore.ConnectorConfig{}, false
	}
	for _, cfg := range configs {
		if sourceConnectorTypes[cfg.Type] && cfg.Config["project_id"] == projectID {
			return cfg, true
		}
	}
	return bstore.ConnectorConfig{}, false
}

// projectHasSourceConnector reports whether the project is bound to at least one
// source connector (signal #1).
func (s *Server) projectHasSourceConnector(ctx context.Context, workspaceID, projectID string) bool {
	_, ok := s.projectSourceConnector(ctx, workspaceID, projectID)
	return ok
}

// annotateProjectOrigin fills the project-type rollup and the per-collection
// origin/editable fields on a project-detail response. Each collection response
// already carries its own origin (collectionToResponse); this applies the
// project-level source-connector override (which forces every collection to
// Connected) and derives the project type + project editability.
func (s *Server) annotateProjectOrigin(ctx context.Context, workspaceID string, info *ProjectInfoResponse) {
	if info == nil {
		return
	}
	projectSourced := s.projectHasSourceConnector(ctx, workspaceID, info.ID)

	anyConnected, anyManaged := false, false
	for i := range info.Collections {
		if projectSourced {
			info.Collections[i].Origin = collectionOriginConnected
			info.Collections[i].Editable = false
		}
		if info.Collections[i].Origin == collectionOriginConnected {
			anyConnected = true
		} else {
			anyManaged = true
		}
	}

	switch {
	case projectSourced:
		info.Type = projectTypeConnected
	case anyConnected && anyManaged:
		info.Type = projectTypeHybrid
	case anyConnected:
		info.Type = projectTypeConnected
	default:
		info.Type = projectTypeManaged
	}
	info.Editable = info.Type != projectTypeConnected
}

// guardSourceMutation refuses a SOURCE-content mutation (file upload, item
// delete, direct source-text edit) when the target is connector-sourced —
// either the whole project is bound to a source connector (signal #1) or the
// specific collection is connector-backed (signal #2). Review/approve, target
// edits and configuration are never routed through here — only source content
// is gated.
//
// refused reports that the request has been answered with a 409 and the caller
// must return immediately. The refusal cannot be carried by err alone: writing
// the response is what c.JSON reports on, and it returns nil when the write
// succeeds — a caller keyed on err would refuse in the response and mutate
// anyway.
func (s *Server) guardSourceMutation(c echo.Context, projectID string, coll *store.Collection) (refused bool, err error) {
	ctx := c.Request().Context()
	workspaceID, _ := c.Get("workspace_id").(string)

	srcConn, projectSourced := s.projectSourceConnector(ctx, workspaceID, projectID)
	if !projectSourced && !collectionConnected(coll) {
		return false, nil
	}
	return true, c.JSON(http.StatusConflict, ErrorResponse{
		Error: sourceSyncedMessage(srcConn, projectSourced, coll),
	})
}

// sourceSyncedMessage explains why a source mutation was refused, naming the
// connector the source is synced from so the user knows where to edit instead.
func sourceSyncedMessage(srcConn bstore.ConnectorConfig, projectSourced bool, coll *store.Collection) string {
	switch {
	case projectSourced:
		return fmt.Sprintf(
			"this project's source is synced from %s and is read-only in Bowrain; edit it at source, then it re-syncs here.",
			connectorLabel(srcConn))
	case coll != nil:
		return fmt.Sprintf(
			"collection %q is synced from a connector and is read-only in Bowrain; edit it at source.",
			coll.Name)
	default:
		return "this content is synced from a connector and is read-only in Bowrain; edit it at source."
	}
}

// connectorLabel renders a human label for a source connector: the repository
// for a forge, else its name, else its type.
func connectorLabel(cfg bstore.ConnectorConfig) string {
	if repo := cfg.Config["repo"]; repo != "" {
		return repo
	}
	if cfg.Name != "" {
		return cfg.Name
	}
	if cfg.Type != "" {
		return "a " + cfg.Type + " connector"
	}
	return "a connector"
}

// collectionForItem resolves the collection an item belongs to, so an item-level
// mutation can be gated on that collection's origin. Returns nil when the item
// or its collection cannot be resolved (the caller still applies the project-
// level source-connector gate).
func (s *Server) collectionForItem(ctx context.Context, projectID, stream, itemName string) *store.Collection {
	if s.ContentStore == nil {
		return nil
	}
	items, err := s.ContentStore.ListItems(ctx, projectID, stream)
	if err != nil {
		return nil
	}
	for _, it := range items {
		if it.Name == itemName && it.CollectionID != "" {
			coll, err := s.ContentStore.GetCollection(ctx, projectID, it.CollectionID)
			if err == nil {
				return coll
			}
			return nil
		}
	}
	return nil
}
