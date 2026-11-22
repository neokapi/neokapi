package observe

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loggedEcho is an Echo with RequestLogMiddleware logging JSON into log.
type loggedEcho struct {
	*echo.Echo
	log *bytes.Buffer
}

// serve serves one request and returns the JSON records logged for it and the
// response.
func (e loggedEcho) serve(t *testing.T, r *http.Request) ([]map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	var records []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(e.log.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m))
		records = append(records, m)
	}
	return records, rec
}

func newLoggedEcho() loggedEcho {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))
	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(RequestLogMiddleware(logger, RequestLogConfig{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
		SkipPaths:        []string{"/health"},
	}))
	return loggedEcho{Echo: e, log: buf}
}

func group(t *testing.T, rec map[string]any, key string) map[string]any {
	t.Helper()
	g, ok := rec[key].(map[string]any)
	require.Truef(t, ok, "record has no %q group: %v", key, rec)
	return g
}

// The monitoring dashboard and the CloudWatch latency filters read
// request.route, request.path, response.latency (nanoseconds) and
// response.status off this record.
func TestRequestLog_RecordShape(t *testing.T) {
	e := newLoggedEcho()
	e.POST("/api/v1/:ws/projects", func(c *echo.Context) error {
		return c.JSON(http.StatusCreated, map[string]string{"ok": "yes"})
	})

	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/projects?x=1", strings.NewReader(`{"a":1}`))
	r.Header.Set(echo.HeaderXRequestID, "req-123")
	records, rec := e.serve(t, r)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, records, 1)
	got := records[0]

	assert.Equal(t, "Incoming request", got["msg"])
	assert.Equal(t, "INFO", got["level"])
	assert.Equal(t, "req-123", got["id"])

	req := group(t, got, "request")
	assert.Equal(t, http.MethodPost, req["method"])
	assert.Equal(t, "/api/v1/acme/projects", req["path"])
	assert.Equal(t, "x=1", req["query"])
	assert.Equal(t, "/api/v1/:ws/projects", req["route"])
	assert.Equal(t, map[string]any{"ws": "acme"}, req["params"])
	assert.NotContains(t, req, "ip", "the record carries no client address")
	assert.InDelta(t, 0, req["length"], 0, "the handler read none of the body")
	for _, k := range []string{"time", "host", "referer"} {
		assert.Contains(t, req, k)
	}

	res := group(t, got, "response")
	assert.InDelta(t, float64(http.StatusCreated), res["status"], 0)
	assert.Greater(t, res["latency"], float64(0), "latency is a duration in nanoseconds")
	assert.InDelta(t, float64(len(rec.Body.String())), res["length"], 0)
	assert.Contains(t, res, "time")
}

// A handler error with no HTTP status is answered here as a 500 through the
// error handler, and logged with that status at the server-error level.
func TestRequestLog_BareErrorIsAnsweredAs500(t *testing.T) {
	e := newLoggedEcho()
	var handled []error
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		handled = append(handled, err)
		if r, _ := echo.UnwrapResponse(c.Response()); r != nil && r.Committed {
			return
		}
		_ = c.NoContent(echo.StatusCode(err))
	}
	cause := errors.New("database is gone")
	e.GET("/x", func(*echo.Context) error { return cause })

	records, rec := e.serve(t, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Len(t, handled, 2, "answered by the log middleware, then found committed at the end of the chain")
	he, ok := errors.AsType[*echo.HTTPError](handled[0])
	require.True(t, ok, "the error handler gets a 500 *echo.HTTPError wrapping the cause")
	assert.Equal(t, http.StatusInternalServerError, he.Code)
	require.ErrorIs(t, he, cause)

	require.Len(t, records, 1)
	assert.Equal(t, "ERROR", records[0]["level"])
	assert.Equal(t, "Internal Server Error", records[0]["msg"])
	assert.Equal(t, "database is gone", records[0]["internal"])
	assert.InDelta(t, float64(http.StatusInternalServerError), group(t, records[0], "response")["status"], 0)
}

// The router's own 404 is logged as a 404 with no route, at the client-error
// level, and returned for the error handler to answer.
func TestRequestLog_UnmatchedRoute(t *testing.T) {
	e := newLoggedEcho()
	e.GET("/known", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })

	records, rec := e.serve(t, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.Len(t, records, 1)
	assert.Equal(t, "WARN", records[0]["level"])
	assert.Equal(t, "Not Found", records[0]["msg"])
	assert.Empty(t, group(t, records[0], "request")["route"])
	assert.Empty(t, group(t, records[0], "request")["params"])
	assert.InDelta(t, float64(http.StatusNotFound), group(t, records[0], "response")["status"], 0)
}

// A path served for another method is logged as a 405 with the route pattern
// and no parameters.
func TestRequestLog_MethodNotAllowed(t *testing.T) {
	e := newLoggedEcho()
	e.GET("/files/*", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })

	records, rec := e.serve(t, httptest.NewRequest(http.MethodPost, "/files/a/b", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Len(t, records, 1)
	assert.Equal(t, "Method Not Allowed", records[0]["msg"])
	assert.Equal(t, "/files/*", group(t, records[0], "request")["route"])
	assert.Empty(t, group(t, records[0], "request")["params"])
}

func TestRequestLog_SkipPaths(t *testing.T) {
	e := newLoggedEcho()
	e.GET("/health", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })

	records, rec := e.serve(t, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, records)
}

// The request length counts the body bytes the handler read.
func TestRequestLog_RequestLength(t *testing.T) {
	e := newLoggedEcho()
	e.POST("/x", func(c *echo.Context) error {
		var v map[string]any
		if err := c.Bind(&v); err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	})

	body := `{"name":"value"}`
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	records, rec := e.serve(t, r)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, records, 1)
	assert.InDelta(t, float64(len(body)), group(t, records[0], "request")["length"], 0)
}
