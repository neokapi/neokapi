package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAdminRoutes_NotMountedWithoutVerifier verifies that when no admin OIDC
// verifier is configured, the /api/admin/* routes are not mounted at all —
// closing the fail-open fallback where any regular-user JWT was accepted by
// the admin control plane (finding #2). A request returns 404 (no such route),
// never reaching an admin handler.
func TestAdminRoutes_NotMountedWithoutVerifier(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret" // normal production user-auth state
	// AdminVerifier is nil and AllowInsecureAdminAuth defaults to false.
	s := NewServer(cfg)
	e := s.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"admin routes must not be mounted without an admin verifier")
}

// TestAdminRoutes_MountedWithInsecureOptIn verifies the explicit local-dev
// opt-in still mounts the routes (behind the user-JWT middleware), so the
// escape hatch remains available but only by deliberate configuration.
func TestAdminRoutes_MountedWithInsecureOptIn(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	cfg.AllowInsecureAdminAuth = true
	s := NewServer(cfg)
	e := s.GetEcho()

	// No Authorization header → the AuthMiddleware rejects with 401. The key
	// assertion is that the route IS mounted (not 404), so the middleware runs.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"with the insecure opt-in the route is mounted and the auth middleware runs")
	assert.NotEqual(t, http.StatusNotFound, rec.Code)
}
