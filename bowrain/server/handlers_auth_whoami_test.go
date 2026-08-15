package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// whoamiEcho wires HandleAuthWhoami onto a bare Echo so the identity probe can
// be exercised without spinning up the full server or a database.
func whoamiEcho(s *Server) *echo.Echo {
	e := echo.New()
	e.GET("/api/v1/auth/whoami", s.HandleAuthWhoami)
	return e
}

// assertDisplayOnlyKeys fails if the whoami body carries anything beyond the
// four display fields — a guard against ever leaking a token, the internal user
// id, or the OIDC subject across origins.
func assertDisplayOnlyKeys(t *testing.T, body []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"authenticated", "avatar_url", "email", "name"}, keys,
		"whoami must return only display fields")
}

func TestWhoamiNoSessionReturnsUnauthenticated(t *testing.T) {
	s := &Server{Config: Config{JWTSecret: "test-secret"}}
	e := whoamiEcho(s)

	// No session cookie at all.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "whoami always answers 200")
	assertDisplayOnlyKeys(t, rec.Body.Bytes())

	var resp WhoAmIResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
	assert.Empty(t, resp.Name)
	assert.Empty(t, resp.Email)
	assert.Empty(t, resp.AvatarURL)
}

func TestWhoamiInvalidSessionReturnsUnauthenticated(t *testing.T) {
	s := &Server{Config: Config{JWTSecret: "test-secret"}}
	e := whoamiEcho(s)

	// A garbage / wrong-secret cookie must be treated as signed-out, not error.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-jwt"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp WhoAmIResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
}

func TestWhoamiWithSessionReturnsIdentity(t *testing.T) {
	// No AuthStore wired: whoami answers from the session-JWT claims alone.
	s := &Server{Config: Config{JWTSecret: "test-secret"}}
	e := whoamiEcho(s)

	token := generateTestToken(t, s.Config.JWTSecret) // user-1 / test@example.com / Test User

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertDisplayOnlyKeys(t, rec.Body.Bytes())
	// The token itself must never appear in the response body.
	assert.NotContains(t, rec.Body.String(), token)

	var resp WhoAmIResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Authenticated)
	assert.Equal(t, "Test User", resp.Name)
	assert.Equal(t, "test@example.com", resp.Email)
}

// TestWhoamiEnrichesFromUserRecord exercises the full path against a real
// (Postgres) AuthStore: whoami adds the avatar URL from the persisted user
// record. Skipped in -short mode (no testcontainers).
func TestWhoamiEnrichesFromUserRecord(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)

	require.NoError(t, srv.AuthStore.CreateUser(t.Context(), &platauth.User{
		ID:        "user-1",
		Email:     "test@example.com",
		Name:      "Test User",
		AvatarURL: "https://cdn.example/avatar.png",
	}))

	e := srv.GetEcho()
	token := generateTestToken(t, cfg.JWTSecret) // subject == "user-1"

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assertDisplayOnlyKeys(t, rec.Body.Bytes())

	var resp WhoAmIResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Authenticated)
	assert.Equal(t, "Test User", resp.Name)
	assert.Equal(t, "https://cdn.example/avatar.png", resp.AvatarURL)
}

// --- CORS: credentialed, tightly-scoped landing origin ---

// corsEcho mounts the server's real CORS config in front of the whoami route.
func corsEcho(cfg Config) *echo.Echo {
	s := &Server{Config: cfg}
	e := echo.New()
	e.Use(middleware.CORSWithConfig(s.corsConfig()))
	e.GET("/api/v1/auth/whoami", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}

func TestCORSAllowsLandingOriginWithCredentials(t *testing.T) {
	const landing = "https://bowrain.cloud"
	e := corsEcho(Config{
		AppPublicURL:  "https://app.bowrain.cloud",
		PublicSiteURL: landing,
	})

	// Preflight from the landing origin.
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/whoami", nil)
	req.Header.Set(echo.HeaderOrigin, landing)
	req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodGet)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, landing, rec.Header().Get(echo.HeaderAccessControlAllowOrigin),
		"landing origin must be reflected exactly (never *)")
	assert.Equal(t, "true", rec.Header().Get(echo.HeaderAccessControlAllowCredentials),
		"credentials must be allowed so the session cookie is sent")

	// Actual GET carries the same credentialed CORS headers.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	req.Header.Set(echo.HeaderOrigin, landing)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, landing, rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
	assert.Equal(t, "true", rec.Header().Get(echo.HeaderAccessControlAllowCredentials))
}

func TestCORSAllowsAppOrigin(t *testing.T) {
	// The app's OWN origin, which is what AppPublicURL names. This used to be
	// read from OIDCPublicURL — the identity provider's browser-facing URL —
	// so the production allowlist credentialed the IdP's host rather than the
	// app's.
	e := corsEcho(Config{
		AppPublicURL:  "https://app.bowrain.cloud",
		PublicSiteURL: "https://bowrain.cloud",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	req.Header.Set(echo.HeaderOrigin, "https://app.bowrain.cloud")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "https://app.bowrain.cloud", rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
	assert.Equal(t, "true", rec.Header().Get(echo.HeaderAccessControlAllowCredentials))
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	e := corsEcho(Config{
		AppPublicURL:  "https://app.bowrain.cloud",
		PublicSiteURL: "https://bowrain.cloud",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
	req.Header.Set(echo.HeaderOrigin, "https://evil.example.com")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// An origin outside the allowlist gets no ACAO header — the browser blocks
	// it from reading the response.
	assert.Empty(t, rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
}

func TestCORSDevLandingOrigin(t *testing.T) {
	// DevMode: dynamic allow-list — localhost plus the landing. Development is
	// opted into explicitly, never inferred from an unset field.
	e := corsEcho(Config{DevMode: true, PublicSiteURL: "https://bowrain.cloud"})

	for _, origin := range []string{"http://localhost:5173", "https://bowrain.cloud"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/whoami", nil)
		req.Header.Set(echo.HeaderOrigin, origin)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, origin, rec.Header().Get(echo.HeaderAccessControlAllowOrigin), "origin %s should be allowed", origin)
		assert.Equal(t, "true", rec.Header().Get(echo.HeaderAccessControlAllowCredentials))
	}
}

// --- hint cookie: parent-domain derivation ---

func TestParentCookieDomain(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"app.bowrain.cloud", ".bowrain.cloud"},
		{"app.dev.bowrain.cloud", ".dev.bowrain.cloud"},
		{"app.bowrain.cloud:8443", ".bowrain.cloud"},
		{"bowrain.cloud", ""},  // 2 labels — host-only
		{"localhost", ""},      // dev — host-only
		{"localhost:8080", ""}, // dev with port — host-only
		{"127.0.0.1", ""},      // bare IP — host-only
		{"127.0.0.1:8080", ""}, // bare IP with port — host-only
	}
	for _, tc := range cases {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = tc.host
		c := e.NewContext(req, httptest.NewRecorder())
		assert.Equal(t, tc.want, parentCookieDomain(c), "host %s", tc.host)
	}
}
