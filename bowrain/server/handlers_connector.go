package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// connectorFeature maps a connector type to the plan feature it requires. Types
// absent from the map are available on every plan. Only entries whose feature is
// actually sold belong here: gating a type nobody advertises would invent a
// paywall, and leaving out one that is advertised gives it away (which is what
// happened to git).
var connectorFeature = map[string]billing.Feature{
	"git":   billing.FeatureConnectorsGit,
	"forge": billing.FeatureConnectorsGit, // the git connector plus PR/MR delivery — same sold feature
}

// ConnectorAddRequest is the request for adding or updating a connector.
type ConnectorAddRequest struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

// FetchRequest is the request for fetching content from a connector.
type FetchRequest struct {
	ConnectorID string   `json:"connector_id"`
	ProjectID   string   `json:"project_id"`
	Paths       []string `json:"paths,omitempty"`
}

// PublishRequest is the request for publishing content to a connector.
type PublishRequest struct {
	ConnectorID string `json:"connector_id"`
	ProjectID   string `json:"project_id"`
	Message     string `json:"message,omitempty"`
}

// connectorInfo is the API view of a connector on the listing: identity and
// which config fields are set, never a secret. Config carries the persisted
// config with secret values redacted to "" (the key is kept so a UI can show
// the field is configured).
type connectorInfo struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Name     string             `json:"name"`
	Category connector.Category `json:"category"`
	Config   map[string]string  `json:"config,omitempty"`
}

// connectorCategory resolves a connector type to its category via the registry.
// Returns "" for an unknown type.
func (s *Server) connectorCategory(connectorType string) connector.Category {
	if s.ConnectorReg == nil {
		return ""
	}
	for _, info := range s.ConnectorReg.List() {
		if info.Name == connectorType {
			return info.Category
		}
	}
	return ""
}

// rehydrateConnectors re-instantiates every persisted connector into the live
// ConnectorService at boot, so remote connectors survive a restart. It is
// fail-soft: a connector that no longer instantiates (bad config, retired type)
// is logged and skipped, never aborting boot. No-op when persistence is not
// wired (the in-memory/desktop path runs connectors live-only).
func (s *Server) rehydrateConnectors(ctx context.Context) {
	if s.ConnectorConfigStore == nil || s.Services == nil || s.Services.Connector == nil {
		return
	}
	configs, err := s.ConnectorConfigStore.ListAll(ctx)
	if err != nil {
		slog.Warn("connector rehydration: list failed", "error", err)
		return
	}
	rehydrated := 0
	for _, cfg := range configs {
		config := cfg.Config
		if config == nil {
			config = map[string]string{}
		}
		// Pin the persisted id so the live instance stays addressable by the
		// same id after restart, regardless of how the connector derives one.
		config["id"] = cfg.ID
		if _, err := s.Services.Connector.AddConnector(cfg.WorkspaceID, cfg.Type, config); err != nil {
			slog.Warn("connector rehydration: skipped a connector",
				"id", cfg.ID, "type", cfg.Type, "workspace", cfg.WorkspaceID, "error", err)
			continue
		}
		rehydrated++
	}
	if rehydrated > 0 {
		slog.Info("connector rehydration complete", "count", rehydrated)
	}
}

// connectorIDParam returns the :id path parameter percent-decoded. Echo hands
// back the raw matched segment, and connector ids can carry characters the
// client must escape (the WordPress connector historically derived ids
// containing ':'), so every handler must unescape before lookups.
func connectorIDParam(c echo.Context) string {
	raw := c.Param("id")
	if id, err := url.PathUnescape(raw); err == nil {
		return id
	}
	return raw
}

func (s *Server) HandleListActiveConnectors(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)

	// When persistence is wired, the persisted set is the source of truth: it
	// carries type + name + redacted config even for connectors not yet touched
	// this process. Without a store (desktop/in-memory), fall back to the live
	// instances.
	if s.ConnectorConfigStore != nil {
		configs, err := s.ConnectorConfigStore.List(c.Request().Context(), wsID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
		result := make([]connectorInfo, len(configs))
		for i, cfg := range configs {
			result[i] = connectorInfo{
				ID:       cfg.ID,
				Type:     cfg.Type,
				Name:     cfg.Name,
				Category: s.connectorCategory(cfg.Type),
				Config:   cfg.Config, // secrets already redacted by the store
			}
		}
		return c.JSON(http.StatusOK, result)
	}

	active := s.Services.Connector.ListActive(wsID)
	result := make([]connectorInfo, len(active))
	for i, conn := range active {
		result[i] = connectorInfo{
			ID:       conn.ID(),
			Name:     conn.Name(),
			Category: conn.Category(),
		}
	}
	return c.JSON(http.StatusOK, result)
}

func (s *Server) HandleAddConnector(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}

	var req ConnectorAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Git connectors are a paid feature (AD-018), and the plan matrix has said so
	// since it was written — but nothing enforced it, so a Free workspace could
	// add one. The gate has to live here rather than on the route: every connector
	// type shares this endpoint, and the type only becomes known once the body is
	// bound. It runs before the service check so entitlement never depends on how
	// the server happens to be wired.
	if feature, ok := connectorFeature[req.Type]; ok {
		if err := billing.RequireFeature(c, feature, s.billingGuardEvent()); err != nil {
			return err
		}
	}

	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)
	conn, err := s.Services.Connector.AddConnector(wsID, req.Type, req.Config)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Persist so the connector (and its sealed credentials) survives a restart.
	// If persistence fails, undo the live registration so the two never diverge.
	if s.ConnectorConfigStore != nil {
		if _, err := s.ConnectorConfigStore.Upsert(c.Request().Context(), &bstore.ConnectorConfig{
			ID:          conn.ID(),
			WorkspaceID: wsID,
			Type:        req.Type,
			Name:        conn.Name(),
			Config:      req.Config,
		}); err != nil {
			_ = s.Services.Connector.RemoveConnector(wsID, conn.ID())
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
	}

	return c.JSON(http.StatusCreated, map[string]string{
		"id":       conn.ID(),
		"name":     conn.Name(),
		"category": string(conn.Category()),
	})
}

// HandleUpdateConnector reconfigures a connector in place: it re-instantiates
// the live instance with the new config and re-persists it. Secret fields follow
// preserve-on-blank semantics — an empty secret in the request keeps the stored
// credential (so an endpoint or name edit does not wipe the password/token).
func (s *Server) HandleUpdateConnector(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	if s.ConnectorConfigStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "connector persistence not configured"})
	}

	var req ConnectorAddRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	id := connectorIDParam(c)

	// Load the existing config to learn its type (the request may omit it) and
	// to 404 a missing/cross-tenant id before touching anything.
	existing, err := s.ConnectorConfigStore.Get(c.Request().Context(), wsID, id)
	if err != nil {
		if errors.Is(err, bstore.ErrConnectorConfigNotFound) {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	connType := req.Type
	if connType == "" {
		connType = existing.Type
	}

	// The git billing gate applies to updates too — a Free workspace must not be
	// able to reconfigure its way into a git connector.
	if feature, ok := connectorFeature[connType]; ok {
		if err := billing.RequireFeature(c, feature, s.billingGuardEvent()); err != nil {
			return err
		}
	}

	// Persist first: the store seals new secrets and preserves blank ones.
	saved, err := s.ConnectorConfigStore.Upsert(c.Request().Context(), &bstore.ConnectorConfig{
		ID:          id,
		WorkspaceID: wsID,
		Type:        connType,
		Name:        req.Config["name"],
		Config:      req.Config,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Re-read the effective (decrypted) config — this is the persisted config
	// with blank secrets resolved to their stored values — and re-instantiate
	// the live instance from it so the running connector matches what is stored.
	effective, err := s.ConnectorConfigStore.Get(c.Request().Context(), wsID, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	config := effective.Config
	if config == nil {
		config = map[string]string{}
	}
	config["id"] = id // keep the live instance addressable by the same id
	_ = s.Services.Connector.RemoveConnector(wsID, id)
	if _, err := s.Services.Connector.AddConnector(wsID, connType, config); err != nil {
		// The new config doesn't instantiate — restore the previous persisted
		// config and its live instance so a bad update can't strand a broken
		// row (and a dead connector) behind a 400.
		if _, rbErr := s.ConnectorConfigStore.Upsert(c.Request().Context(), &existing); rbErr == nil {
			prev := existing.Config
			if prev == nil {
				prev = map[string]string{}
			}
			prev["id"] = id
			_, _ = s.Services.Connector.AddConnector(wsID, existing.Type, prev)
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, connectorInfo{
		ID:       saved.ID,
		Type:     saved.Type,
		Name:     saved.Name,
		Category: s.connectorCategory(saved.Type),
		Config:   saved.Config, // secrets redacted by the store
	})
}

func (s *Server) HandleRemoveConnector(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	id := connectorIDParam(c)

	// With persistence wired, the persisted row is the source of truth: remove
	// the live instance best-effort (it is normally present via rehydration),
	// then delete the row — a missing row is a 404.
	if s.ConnectorConfigStore != nil {
		_ = s.Services.Connector.RemoveConnector(wsID, id)
		if err := s.ConnectorConfigStore.Delete(c.Request().Context(), wsID, id); err != nil {
			if errors.Is(err, bstore.ErrConnectorConfigNotFound) {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			}
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		}
		return c.NoContent(http.StatusNoContent)
	}

	// Live-only path (desktop/in-memory): the in-memory instance is authoritative.
	if err := s.Services.Connector.RemoveConnector(wsID, id); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	return c.NoContent(http.StatusNoContent)
}

// connectorIDFromRequest prefers the path :id and falls back to the body's
// connector_id for backward compatibility with the pre-path callers.
func connectorIDFromRequest(c echo.Context, bodyID string) string {
	if id := connectorIDParam(c); id != "" {
		return id
	}
	return bodyID
}

func (s *Server) HandleFetch(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req FetchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	connID := connectorIDFromRequest(c, req.ConnectorID)
	items, err := s.Services.Connector.Fetch(c.Request().Context(), wsID, connID, req.ProjectID, connector.FetchOptions{
		Paths: req.Paths,
	})
	if err != nil {
		s.setConnectorLastError(c.Request().Context(), wsID, connID, err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.touchConnectorLastSync(c.Request().Context(), wsID, connID)

	return c.JSON(http.StatusOK, map[string]int{"items_fetched": len(items)})
}

func (s *Server) HandlePublish(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	var req PublishRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	connID := connectorIDFromRequest(c, req.ConnectorID)
	err := s.Services.Connector.Publish(c.Request().Context(), wsID, connID, req.ProjectID, connector.PublishOptions{
		Message: req.Message,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.touchConnectorLastSync(c.Request().Context(), wsID, connID)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) HandleConnectorStatus(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	connID := connectorIDParam(c)
	status, statusErr := s.Services.Connector.ConnectorStatus(c.Request().Context(), wsID, connID)

	var cfg *bstore.ConnectorConfig
	if s.ConnectorConfigStore != nil {
		if got, cfgErr := s.ConnectorConfigStore.Get(c.Request().Context(), wsID, connID); cfgErr == nil {
			cfg = &got
		}
	}

	if statusErr != nil {
		// The live probe failed. For a connector the config store knows this
		// is a *status*, not a missing resource: a git/forge connector's probe
		// re-runs the same clone the ingest does, so a broken import used to
		// 404 every status poll here — and the recorded last_error (with the
		// failed import behind it) never reached the client, which kept
		// showing "importing" forever. Degrade to the stored state (real
		// last-sync, recorded error) plus the probe error instead.
		if cfg == nil {
			return apiErr(c, http.StatusNotFound, statusErr.Error())
		}
		degraded := &connector.SyncStatus{ConnectorID: connID, LastSync: cfg.LastSyncAt}
		if cfg.LastError != "" {
			degraded.Errors = append(degraded.Errors, cfg.LastError)
		}
		if msg := statusErr.Error(); cfg.LastError != msg {
			degraded.Errors = append(degraded.Errors, msg)
		}
		return c.JSON(http.StatusOK, degraded)
	}

	// Replace the connector's own (fabricated) LastSync with the authoritative
	// stored timestamp: the real time of the last successful fetch/publish, or a
	// zero value the client renders as "never synced" until the first sync. A
	// recorded sync failure (e.g. a background ingest that failed) joins the
	// reported errors so the panel and setup page can surface it.
	if cfg != nil {
		status.LastSync = cfg.LastSyncAt
		if cfg.LastError != "" {
			status.Errors = append(status.Errors, cfg.LastError)
		}
	}
	return c.JSON(http.StatusOK, status)
}

// touchConnectorLastSync records a successful sync time for a connector (and
// clears any recorded last error), best effort: a nil store (standalone) or a
// missing row must never fail the sync that just succeeded, so errors are
// logged and swallowed.
func (s *Server) touchConnectorLastSync(ctx context.Context, wsID, connID string) {
	if s.ConnectorConfigStore == nil {
		return
	}
	if err := s.ConnectorConfigStore.TouchLastSync(ctx, wsID, connID, time.Now().UTC()); err != nil {
		slog.WarnContext(ctx, "record connector last-sync", "connector", connID, "error", err)
	}
}

// setConnectorLastError records a failed sync on the connector row, best
// effort: a nil store or a missing row must never mask the original failure,
// so errors are logged and swallowed. The next successful sync clears it
// (TouchLastSync).
func (s *Server) setConnectorLastError(ctx context.Context, wsID, connID string, syncErr error) {
	if s.ConnectorConfigStore == nil {
		return
	}
	if err := s.ConnectorConfigStore.SetLastError(ctx, wsID, connID, syncErr.Error()); err != nil {
		slog.WarnContext(ctx, "record connector last-error", "connector", connID, "error", err)
	}
}

// contentListResponse wraps the content items so the payload has a stable
// top-level shape ({items: [...]}). Each item is a core/connector ContentItem
// marshaled verbatim — the web UI builds its TS types against that struct.
type contentListResponse struct {
	Items []*connector.ContentItem `json:"items"`
}

// HandleConnectorContent lists the content items a connector exposes, mirroring
// the desktop app's in-process ListContentItems binding (a connector-wide List
// that does not fetch full content into the store). It is read-only, so it is
// gated only by workspace membership (the wsSpecific group), not
// PermManageConnectors.
//
// An optional ?paths= (comma-separated paths or IDs) narrows the result. An
// optional ?project_id= is accepted for request symmetry with fetch but does
// not scope the listing — a connector's content set is connector-wide, not
// per-project.
func (s *Server) HandleConnectorContent(c echo.Context) error {
	if s.Services == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	items, err := s.Services.Connector.ListContent(c.Request().Context(), wsID, connectorIDParam(c))
	if err != nil {
		if errors.Is(err, service.ErrConnectorNotFound) {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	items = filterContentByPaths(items, c.QueryParam("paths"))
	if items == nil {
		items = []*connector.ContentItem{}
	}
	return c.JSON(http.StatusOK, contentListResponse{Items: items})
}

// filterContentByPaths narrows items to those whose Path or ID appears in the
// comma-separated paths list. An empty list is a no-op (returns items as-is).
func filterContentByPaths(items []*connector.ContentItem, paths string) []*connector.ContentItem {
	if strings.TrimSpace(paths) == "" {
		return items
	}
	want := make(map[string]struct{})
	for p := range strings.SplitSeq(paths, ",") {
		if p = strings.TrimSpace(p); p != "" {
			want[p] = struct{}{}
		}
	}
	if len(want) == 0 {
		return items
	}
	filtered := make([]*connector.ContentItem, 0, len(items))
	for _, it := range items {
		if _, ok := want[it.Path]; ok {
			filtered = append(filtered, it)
			continue
		}
		if _, ok := want[it.ID]; ok {
			filtered = append(filtered, it)
		}
	}
	return filtered
}
