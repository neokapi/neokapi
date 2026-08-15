package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveWithSecurityHeaders runs one request through the middleware and the
// given handler, returning the recorder.
func serveWithSecurityHeaders(t *testing.T, method, target string, tls bool, h echo.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	e.Use(securityHeaders())
	e.Add(method, "/*", h)

	req := httptest.NewRequest(method, target, nil)
	if tls {
		req.Header.Set("X-Forwarded-Proto", "https")
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestSecurityHeadersBaseline pins the headers every response carries.
func TestSecurityHeadersBaseline(t *testing.T) {
	rec := serveWithSecurityHeaders(t, http.MethodGet, "/api/v1/health", false, func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	h := rec.Header()
	assert.Equal(t, "nosniff", h.Get(headerNosniff))
	assert.Equal(t, "DENY", h.Get(headerFrameOpts))
	assert.Equal(t, "strict-origin-when-cross-origin", h.Get(headerReferrer))
	assert.NotEmpty(t, h.Get(headerPermissions))
	assert.Equal(t, defaultCSP, h.Get(headerCSP))
}

// TestSecurityHeadersCSPShape checks the directives the default policy is
// meant to carry, so a future edit cannot quietly widen it.
func TestSecurityHeadersCSPShape(t *testing.T) {
	for _, directive := range []string{
		"default-src 'none'",
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'none'",
	} {
		assert.Contains(t, defaultCSP, directive)
	}
	// Inline styles are permitted deliberately — the OIDC callback pages carry a
	// style attribute — but nothing may load or execute script.
	assert.Contains(t, defaultCSP, "style-src 'unsafe-inline'")
	assert.NotContains(t, defaultCSP, "script-src")
	assert.NotContains(t, defaultCSP, "unsafe-eval")
}

// TestHSTSOnlyOverTLS keeps the transport header off plain-HTTP development
// responses, where it would be noise, and on once a terminating proxy reports
// HTTPS via X-Forwarded-Proto.
func TestHSTSOnlyOverTLS(t *testing.T) {
	plain := serveWithSecurityHeaders(t, http.MethodGet, "/api/v1/health", false, func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	assert.Empty(t, plain.Header().Get(headerHSTS))

	secure := serveWithSecurityHeaders(t, http.MethodGet, "/api/v1/health", true, func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	assert.Contains(t, secure.Header().Get(headerHSTS), "max-age=")
	// Left off deliberately: this server cannot know every sibling host is
	// HTTPS-only, and the directive would bind them all.
	assert.NotContains(t, secure.Header().Get(headerHSTS), "includeSubDomains")
}

// TestPreviewResponsesAreSandboxed pins the policy on document-derived HTML.
// The preview pipeline writes uploaded, translated or connector-sourced markup
// verbatim by design, and these routes sit on the authenticated workspace group
// — the application's own origin — so a direct navigation must land in an
// opaque origin with scripts withdrawn rather than on the app origin.
func TestPreviewResponsesAreSandboxed(t *testing.T) {
	for _, tt := range []struct {
		name    string
		handler echo.HandlerFunc
	}{
		{"document preview", func(c echo.Context) error {
			applySandboxCSP(c)
			return c.HTML(http.StatusOK, "<p>hello</p>")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveWithSecurityHeaders(t, http.MethodGet, "/api/v1/ws/p/preview/main", false, tt.handler)
			assert.Equal(t, sandboxCSP, rec.Header().Get(headerCSP),
				"the sandbox policy must replace the default, not sit alongside it")
			// A sandbox with no tokens is the point: no allow-scripts.
			assert.NotContains(t, rec.Header().Get(headerCSP), "allow-scripts")
			// The baseline still applies.
			assert.Equal(t, "nosniff", rec.Header().Get(headerNosniff))
		})
	}
}

// TestPreviewHandlersSetSandbox drives the real handlers, so the policy cannot
// be lost by an edit to either one.
func TestPreviewHandlersSetSandbox(t *testing.T) {
	srv := &Server{}
	e := echo.New()
	e.Use(securityHeaders())
	e.GET("/preview", srv.HandleRenderDocumentPreview)
	e.GET("/block", srv.HandleRenderBlockHTML)

	for _, path := range []string{"/preview", "/block"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			// ContentStore is nil, so the handler returns early — the header
			// must already be set by then, since a later edit that moved the
			// call below a return would silently drop it on the success path.
			assert.Equal(t, sandboxCSP, rec.Header().Get(headerCSP))
		})
	}
}

// TestSPAResponsesCarryNoContentPolicy pins the deliberate exemption.
//
// In production the SPAs are built in CI and served from S3 behind CloudFront,
// so a policy set here never reaches the document that runs the application —
// that policy belongs to the CDN. The Go server serves the SPA only in
// development and E2E, which is exactly where leaving the default policy on
// would break the app while protecting nothing: the editor's preview iframe
// uses srcDoc, and an about:srcdoc document inherits its embedder's CSP, so a
// script-restricting policy on the SPA silently disables the preview's click,
// height-sync and presence protocol without raising an error.
func TestSPAResponsesCarryNoContentPolicy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"),
		[]byte("<!doctype html><html><body><div id=root></div></body></html>"), 0o600))

	e := echo.New()
	e.Use(securityHeaders())
	e.GET("/*", func(c echo.Context) error { return serveSPAFile(c, dir) })

	for _, path := range []string{"/", "/some/spa/route", "/index.html"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Empty(t, rec.Header().Get(headerCSP),
				"the application's content policy is owned by the CDN, not by this server")
			// Everything else still applies — these cannot break a SPA.
			assert.Equal(t, "nosniff", rec.Header().Get(headerNosniff))
			assert.Equal(t, "DENY", rec.Header().Get(headerFrameOpts))
		})
	}
}

// TestSecurityHeadersOnRealRoutes runs the assembled server so the middleware
// is proven to be wired into SetupRoutes rather than merely to exist.
func TestSecurityHeadersOnRealRoutes(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))

	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get(headerNosniff))
	assert.Equal(t, "DENY", rec.Header().Get(headerFrameOpts))
	assert.True(t, strings.HasPrefix(rec.Header().Get(headerCSP), "default-src 'none'"))
}
