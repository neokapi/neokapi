package server

// PostHog locale-demand connector endpoints (phase 0, read-only).
//
//	GET    /:ws/:id/connectors/posthog          — config (secret never returned; masked)
//	PUT    /:ws/:id/connectors/posthog          — test connection ("SELECT 1") + save
//	DELETE /:ws/:id/connectors/posthog          — disconnect
//	GET    /:ws/:id/connectors/posthog/demand   — DemandSnapshot (?range=7d|30d|90d, ?refresh=true)
//
// The per-project config (host, PostHog project id, personal API key, path
// locale pattern) is persisted exactly like every other connector credential:
// as Collection.ConnectorConfig on a well-known "connected" collection, which
// the content store seals at rest (crypto.Cipher, "enc:v1:" AES-256-GCM on
// Postgres) and which the collections API redacts via redactConnectorConfig.
// The personal API key therefore never leaves the server; GET returns only a
// •••• + last-4 mask.

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/connector"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// posthogCollectionName is the well-known per-project collection that carries
// the PostHog connector config in its (sealed) ConnectorConfig.
const posthogCollectionName = "posthog"

// posthogDemandCacheTTL bounds how often we hit the (rate-limited) PostHog
// query API per project and range.
const posthogDemandCacheTTL = time.Hour

// Connector config keys inside Collection.ConnectorConfig. "api_key" matches
// isSecretConnectorKey, so the generic collections API redacts it too.
const (
	posthogCfgType        = "type"
	posthogCfgHost        = "host"
	posthogCfgProjectID   = "posthog_project_id"
	posthogCfgAPIKey      = "api_key"
	posthogCfgPathPattern = "path_locale_pattern"
)

// PostHogConfigRequest is the connect/update payload. An empty APIKey on
// update keeps the stored key (the client never sees it, so it cannot resend
// it); a non-empty value rotates it.
type PostHogConfigRequest struct {
	Host              string `json:"host"`
	PostHogProjectID  string `json:"posthog_project_id"`
	APIKey            string `json:"api_key,omitempty"`
	PathLocalePattern string `json:"path_locale_pattern,omitempty"`
}

// PostHogConfigResponse is the config as returned to clients. The personal
// API key is never returned; APIKeyMasked carries "••••" + its last four
// characters so the UI can show which key is on file.
type PostHogConfigResponse struct {
	Configured        bool   `json:"configured"`
	Host              string `json:"host,omitempty"`
	HostLabel         string `json:"host_label,omitempty"`
	PostHogProjectID  string `json:"posthog_project_id,omitempty"`
	APIKeyMasked      string `json:"api_key_masked,omitempty"`
	PathLocalePattern string `json:"path_locale_pattern,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

// PostHogErrorResponse extends the standard error shape with a machine code
// so the connect card can distinguish bad-host / bad-key / bad-project.
type PostHogErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// PostHogSourceInfo identifies where a demand snapshot came from, for the
// page's provenance footer.
type PostHogSourceInfo struct {
	Kind             string `json:"kind"` // "posthog"
	HostLabel        string `json:"host_label"`
	PostHogProjectID string `json:"posthog_project_id"`
}

// PostHogDemandResponse wraps the snapshot with source + cache provenance.
type PostHogDemandResponse struct {
	*connector.PostHogDemandSnapshot
	Source   PostHogSourceInfo `json:"source"`
	CachedAt string            `json:"cached_at"`
	Cached   bool              `json:"cached"`
}

// posthogDemandCache is a TTL cache keyed by (project, range). PostHog's
// query API is rate-limited, so demand snapshots are served from memory for
// posthogDemandCacheTTL unless the client passes ?refresh=true.
type posthogDemandCache struct {
	mu      sync.Mutex
	entries map[string]posthogDemandCacheEntry
}

type posthogDemandCacheEntry struct {
	resp      PostHogDemandResponse
	expiresAt time.Time
}

func newPostHogDemandCache() *posthogDemandCache {
	return &posthogDemandCache{entries: map[string]posthogDemandCacheEntry{}}
}

func posthogDemandCacheKey(projectID, demandRange string) string {
	return projectID + ":" + demandRange
}

func (c *posthogDemandCache) Get(key string) (PostHogDemandResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return PostHogDemandResponse{}, false
	}
	return entry.resp, true
}

func (c *posthogDemandCache) Set(key string, resp PostHogDemandResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = posthogDemandCacheEntry{resp: resp, expiresAt: time.Now().Add(posthogDemandCacheTTL)}
}

// Invalidate drops all cached ranges for a project (config changed/removed).
func (c *posthogDemandCache) Invalidate(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if strings.HasPrefix(key, projectID+":") {
			delete(c.entries, key)
		}
	}
}

// maskSecretTail masks a secret as "••••" + last four characters. Short
// secrets are fully masked.
func maskSecretTail(secret string) string {
	if len(secret) <= 4 {
		return "••••"
	}
	return "••••" + secret[len(secret)-4:]
}

func posthogConfigResponse(coll *store.Collection) PostHogConfigResponse {
	cfg := coll.ConnectorConfig
	return PostHogConfigResponse{
		Configured:        true,
		Host:              cfg[posthogCfgHost],
		HostLabel:         connector.PostHogHostLabel(cfg[posthogCfgHost]),
		PostHogProjectID:  cfg[posthogCfgProjectID],
		APIKeyMasked:      maskSecretTail(cfg[posthogCfgAPIKey]),
		PathLocalePattern: cfg[posthogCfgPathPattern],
		UpdatedAt:         coll.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// getPostHogCollection loads the project's PostHog config collection, or nil
// when the connector is not configured.
func (s *Server) getPostHogCollection(c echo.Context, projectID string) *store.Collection {
	coll, err := s.ContentStore.GetCollectionByName(c.Request().Context(), projectID, posthogCollectionName, "")
	if err != nil || coll == nil || coll.ConnectorConfig[posthogCfgType] != "posthog" {
		return nil
	}
	return coll
}

// HandleGetPostHogConfig returns the project's PostHog connector config with
// the personal API key masked (•••• + last-4). The raw key is never returned.
func (s *Server) HandleGetPostHogConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	coll := s.getPostHogCollection(c, projectParam(c))
	if coll == nil {
		return c.JSON(http.StatusOK, PostHogConfigResponse{Configured: false})
	}
	return c.JSON(http.StatusOK, posthogConfigResponse(coll))
}

// HandleSavePostHogConfig validates the config with a live test connection
// (a "SELECT 1" HogQL query) and persists it on success.
func (s *Server) HandleSavePostHogConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := projectParam(c)
	var req PostHogConfigRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Host) == "" {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: "host is required", Code: "bad_host"})
	}
	if strings.TrimSpace(req.PostHogProjectID) == "" {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: "posthog_project_id is required", Code: "bad_project"})
	}

	existing := s.getPostHogCollection(c, pid)

	// An omitted key keeps the stored one (clients only ever see the mask);
	// a provided key rotates it — same carry-forward rule as
	// mergeConnectorConfig on the collections API.
	apiKey := req.APIKey
	if apiKey == "" && existing != nil {
		apiKey = existing.ConnectorConfig[posthogCfgAPIKey]
	}
	if apiKey == "" {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: "personal API key is required", Code: "bad_key"})
	}

	pattern := req.PathLocalePattern
	if pattern == "" {
		pattern = connector.DefaultPostHogPathLocalePattern
	}
	if err := connector.ValidatePostHogPathPattern(pattern); err != nil {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: err.Error(), Code: "bad_pattern"})
	}

	client, err := connector.NewPostHogClient(req.Host, req.PostHogProjectID, apiKey)
	if err != nil {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: err.Error(), Code: "bad_host"})
	}
	if err := client.TestConnection(c.Request().Context()); err != nil {
		return s.posthogUpstreamErr(c, http.StatusBadRequest, err)
	}

	cfg := map[string]string{
		posthogCfgType:        "posthog",
		posthogCfgHost:        strings.TrimSpace(req.Host),
		posthogCfgProjectID:   strings.TrimSpace(req.PostHogProjectID),
		posthogCfgAPIKey:      apiKey,
		posthogCfgPathPattern: pattern,
	}

	ctx := c.Request().Context()
	if existing != nil {
		existing.ConnectorConfig = cfg
		if err := s.ContentStore.UpdateCollection(ctx, existing); err != nil {
			return serverErr(c, err)
		}
		s.posthogDemand.Invalidate(pid)
		return c.JSON(http.StatusOK, posthogConfigResponse(existing))
	}

	coll := &store.Collection{
		ProjectID:       pid,
		Name:            posthogCollectionName,
		Kind:            store.CollectionConnected,
		ItemLabel:       "query",
		ConnectorConfig: cfg,
	}
	if err := s.ContentStore.CreateCollection(ctx, coll); err != nil {
		return serverErr(c, err)
	}
	s.posthogDemand.Invalidate(pid)
	return c.JSON(http.StatusCreated, posthogConfigResponse(coll))
}

// HandleDeletePostHogConfig disconnects PostHog from the project.
func (s *Server) HandleDeletePostHogConfig(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageConnectors); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	pid := projectParam(c)
	coll := s.getPostHogCollection(c, pid)
	if coll == nil {
		return c.JSON(http.StatusNotFound, PostHogErrorResponse{Error: "PostHog connector is not configured", Code: "not_configured"})
	}
	if err := s.ContentStore.DeleteCollection(c.Request().Context(), pid, coll.ID); err != nil {
		return serverErr(c, err)
	}
	s.posthogDemand.Invalidate(pid)
	return c.NoContent(http.StatusNoContent)
}

// HandlePostHogDemand serves the locale-demand snapshot for a project.
// Snapshots are cached in memory per (project, range) for an hour;
// ?refresh=true bypasses the cache.
func (s *Server) HandlePostHogDemand(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := projectParam(c)
	demandRange := c.QueryParam("range")
	if demandRange == "" {
		demandRange = "30d"
	}
	if _, ok := connector.PostHogRangeDays(demandRange); !ok {
		return c.JSON(http.StatusBadRequest, PostHogErrorResponse{Error: "range must be 7d, 30d, or 90d", Code: "bad_range"})
	}

	coll := s.getPostHogCollection(c, pid)
	if coll == nil {
		return c.JSON(http.StatusNotFound, PostHogErrorResponse{Error: "PostHog connector is not configured", Code: "not_configured"})
	}

	cacheKey := posthogDemandCacheKey(pid, demandRange)
	if !boolParam(c, "refresh") {
		if cached, ok := s.posthogDemand.Get(cacheKey); ok {
			cached.Cached = true
			return c.JSON(http.StatusOK, cached)
		}
	}

	cfg := coll.ConnectorConfig
	client, err := connector.NewPostHogClient(cfg[posthogCfgHost], cfg[posthogCfgProjectID], cfg[posthogCfgAPIKey])
	if err != nil {
		return serverErr(c, err)
	}

	snap, err := client.FetchDemand(c.Request().Context(), demandRange, cfg[posthogCfgPathPattern])
	if err != nil {
		// Auth-class failures (rotated key, deleted project, dead host)
		// surface only their classified code; the raw detail stays in the log.
		return s.posthogUpstreamErr(c, http.StatusBadGateway, err)
	}

	resp := PostHogDemandResponse{
		PostHogDemandSnapshot: snap,
		Source: PostHogSourceInfo{
			Kind:             "posthog",
			HostLabel:        connector.PostHogHostLabel(cfg[posthogCfgHost]),
			PostHogProjectID: cfg[posthogCfgProjectID],
		},
		CachedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	s.posthogDemand.Set(cacheKey, resp)
	return c.JSON(http.StatusOK, resp)
}

// posthogUpstreamErr answers a PostHog client failure without leaking upstream
// detail. A classified error surfaces only its stable code and a fixed,
// safe sentence — pe.Message can carry up to 200 bytes of the raw upstream
// response body, the base URL, or the driver's own words, none of which belong
// on the wire — while the full detail goes to the log. An unclassified failure
// becomes a generic 502 via serverErrStatus, which logs the cause and strips it
// from the body.
func (s *Server) posthogUpstreamErr(c echo.Context, classifiedStatus int, err error) error {
	if pe, ok := errors.AsType[*connector.PostHogError](err); ok {
		slog.WarnContext(c.Request().Context(), "posthog upstream error",
			"code", pe.Code, "status", pe.Status, "detail", pe.Message)
		return c.JSON(classifiedStatus, PostHogErrorResponse{
			Error: posthogSafeMessage(pe.Code),
			Code:  string(pe.Code),
		})
	}
	return serverErrStatus(c, http.StatusBadGateway, err)
}

// posthogSafeMessage is the fixed, leak-free sentence shown for each PostHog
// failure class. The connect card renders from the code; this is the fallback
// prose, deliberately carrying no upstream-supplied text.
func posthogSafeMessage(code connector.PostHogErrorCode) string {
	switch code {
	case connector.PostHogErrBadHost:
		return "cannot reach the PostHog host"
	case connector.PostHogErrBadKey:
		return "the PostHog API key was rejected"
	case connector.PostHogErrBadProject:
		return "the PostHog project was not found or is not accessible"
	default:
		return "the PostHog query failed"
	}
}
