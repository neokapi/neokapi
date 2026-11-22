package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The answers below come from the router and the middleware chain rather than
// from a handler, so they are what an installed client sees for a path,
// method or body the server does not serve. Each is pinned to the response
// the server has always given.

func wireEcho(t *testing.T) *echo.Echo {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "router-wire-test"
	return shutdownOnCleanup(t, NewServer(cfg)).GetEcho()
}

func serveWire(e *echo.Echo, r *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestRouterWire_UnknownPathIs404Envelope(t *testing.T) {
	rec, body := serveWire(wireEcho(t), httptest.NewRequest(http.MethodGet, "/no/such/path", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "Not Found", body["error"])
	assert.Equal(t, "Not Found", body["message"])
	assert.NotEmpty(t, body["reference"])
}

func TestRouterWire_WrongMethodIs405EnvelopeWithAllow(t *testing.T) {
	rec, body := serveWire(wireEcho(t), httptest.NewRequest(http.MethodPost, "/metrics", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "Method Not Allowed", body["error"])
	assert.Equal(t, "Method Not Allowed", body["message"])
	assert.Equal(t, "OPTIONS, GET", rec.Header().Get(echo.HeaderAllow))
}

// The limit is 50,000,000 bytes: a declared length at the limit passes the
// body limit, one byte more is refused before the handler runs.
func TestRouterWire_BodyLimitBoundary(t *testing.T) {
	e := wireEcho(t)

	over := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader("{}"))
	over.ContentLength = requestBodyLimit + 1
	rec, body := serveWire(e, over)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, "Request Entity Too Large", body["error"])

	at := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader("{}"))
	at.ContentLength = requestBodyLimit
	rec, _ = serveWire(e, at)
	assert.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code)
}

// A panic is answered as the 500 envelope, with no internal detail.
func TestRouterWire_PanicIs500Envelope(t *testing.T) {
	e := wireEcho(t)
	e.GET("/test/panic", func(*echo.Context) error { panic("handler exploded") })

	rec, body := serveWire(e, httptest.NewRequest(http.MethodGet, "/test/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "internal server error", body["error"])
	assert.NotContains(t, rec.Body.String(), "exploded")
	assert.NotEmpty(t, body["reference"])
}

// The server_error_count alarm counts ERROR log lines. A panic writes two, as a
// returned 500 does: httpErrorHandler's "request failed" and the request log
// record. The stack goes out at WARN.
func TestRouterWire_PanicLogsTwoErrorLines(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	e := wireEcho(t)
	e.GET("/test/panic-log", func(*echo.Context) error { panic("handler exploded") })
	buf.Reset()

	rec, _ := serveWire(e, httptest.NewRequest(http.MethodGet, "/test/panic-log", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	levels := map[string][]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var r struct{ Level, Msg string }
		require.NoError(t, json.Unmarshal([]byte(line), &r))
		levels[r.Level] = append(levels[r.Level], r.Msg)
	}
	assert.Len(t, levels["ERROR"], 2, "ERROR lines: %v", levels["ERROR"])
	assert.Contains(t, levels["WARN"], "panic recovered")
}

// Every OPTIONS request is answered 204 before route middleware runs, so an
// OPTIONS to an authenticated route without an Origin is not refused as
// unauthenticated.
func TestRouterWire_OptionsWithoutOriginIs204(t *testing.T) {
	e := wireEcho(t)

	rec, _ := serveWire(e, httptest.NewRequest(http.MethodOptions, "/api/v1/auth/me", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, echo.HeaderOrigin, rec.Header().Get(echo.HeaderVary))
	assert.Empty(t, rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
	assert.Zero(t, rec.Body.Len())

	// On a route that has no OPTIONS handler of its own, the router's Allow
	// header is passed on.
	rec, _ = serveWire(e, httptest.NewRequest(http.MethodOptions, "/metrics", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "OPTIONS, GET", rec.Header().Get(echo.HeaderAllow))

	// The same route without OPTIONS still requires a token.
	rec, _ = serveWire(e, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// With no allowed origin configured, an origin is answered with "*", which a
// browser never sends credentials to.
func TestRouterWire_CORSWithNoAllowedOrigin(t *testing.T) {
	e := wireEcho(t)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	r.Header.Set(echo.HeaderOrigin, "https://elsewhere.example")
	rec, _ := serveWire(e, r)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
	assert.Equal(t, "true", rec.Header().Get(echo.HeaderAccessControlAllowCredentials))
}
