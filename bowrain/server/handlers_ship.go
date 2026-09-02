package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

// ShipFeedProperty is the project property that opts a project into the public
// ship-status feed. Absent, or anything but "true", keeps the feed off: a public
// feed is a disclosure the project owner turns on deliberately, so the default
// is always closed.
const ShipFeedProperty = "ship_feed_public"

// shipFeedCacheMaxAge is how long a picker or a CDN may reuse a ship manifest
// before revalidating. Short by design: the feed tracks translation progress,
// which moves as a loop runs, and the ETag makes revalidation a cheap 304.
const shipFeedCacheMaxAge = 60 * time.Second

// shipManifestEntry is one locale's standing in the public feed — the same
// two-gate shape the CLI's ship.json carries (host.ShipEntry) and the
// @neokapi/i18n-react picker consumes: shippable (safe to offer) and verified
// (human-reviewed, so no AI badge).
type shipManifestEntry struct {
	Shippable bool `json:"shippable"`
	Verified  bool `json:"verified"`
}

// HandlePublicShipManifest serves a project's per-locale ship manifest over a
// public, cacheable URL — the hosted twin of `kapi status --ship --emit
// ship.json`. The body is byte-shape-identical to that file (locale →
// {shippable, verified}), so @neokapi/i18n-react's loadShipStatus consumes it
// unchanged by pointing at this URL instead of a bundled file. No auth: a
// language picker on a public site must be able to read it. Off unless the
// project opts in via ShipFeedProperty.
func (s *Server) HandlePublicShipManifest(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}
	ctx := c.Request().Context()
	pid := projectParam(c)

	proj, err := s.ContentStore.GetProject(ctx, pid)
	// A project that has not opted in is answered exactly like one that does not
	// exist — 404, never 403 — so the feed reveals nothing about projects that
	// did not choose to publish it.
	if err != nil || proj == nil || proj.Properties[ShipFeedProperty] != "true" {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
	}
	stream := streamParamWithProject(c, proj)

	stats, err := s.shipDashboardStats(ctx, proj, stream)
	if err != nil {
		return serverErr(c, err)
	}
	body, err := json.Marshal(shipManifestFromStats(stats))
	if err != nil {
		return serverErr(c, err)
	}

	etag := shipETag(body)
	h := c.Response().Header()
	h.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(shipFeedCacheMaxAge.Seconds())))
	h.Set("ETag", etag)
	if c.Request().Header.Get("If-None-Match") == etag {
		return c.NoContent(http.StatusNotModified)
	}
	return c.JSONBlob(http.StatusOK, body)
}

// shipDashboardStats returns the project's ship-state-enriched dashboard stats,
// sharing the authenticated dashboard's cache so an anonymous feed hit does not
// re-run the bounded check pass the dashboard already paid for.
func (s *Server) shipDashboardStats(ctx context.Context, proj *store.Project, stream string) (*store.TranslationDashboardStats, error) {
	key := dashboardCacheKey(proj.WorkspaceID, proj.ID, stream)
	if cached, ok := s.dashboardCache.Load(key); ok {
		entry := cached.(*dashboardCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.stats, nil
		}
		s.dashboardCache.Delete(key)
	}
	stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, stream)
	if err != nil {
		return nil, err
	}
	gate := s.resolveTermGate(ctx, proj, stream, proj.WorkspaceID)
	if err := applyShipStates(ctx, s.ContentStore, s.VoiceStore, proj.ID, stream, gate, stats); err != nil {
		return nil, err
	}
	s.dashboardCache.Store(key, &dashboardCacheEntry{stats: stats, expiresAt: time.Now().Add(dashboardCacheTTL)})
	return stats, nil
}

// shipManifestFromStats projects the project-wide per-locale ship states to the
// picker manifest. shippable means the locale ships on at least machine review
// (governed or ai_shippable); verified means it is human-reviewed (governed).
// The project-wide LocaleStats already carry the weakest-scope answer — governed
// requires every block approved — so this matches the CLI's per-collection
// BuildShipManifest without re-deriving per collection.
func shipManifestFromStats(stats *store.TranslationDashboardStats) map[string]shipManifestEntry {
	out := make(map[string]shipManifestEntry, len(stats.LocaleStats))
	for _, ls := range stats.LocaleStats {
		out[ls.Locale] = shipManifestEntry{
			Shippable: ls.ShipState == store.ShipStateGoverned || ls.ShipState == store.ShipStateAIShippable,
			Verified:  ls.ShipState == store.ShipStateGoverned,
		}
	}
	return out
}

// shipETag is a strong validator over the manifest body — a truncated SHA-256,
// enough to distinguish any two manifests a picker would revalidate against.
func shipETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}
