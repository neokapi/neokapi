package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// TestMetricsAccessMiddleware proves /metrics is no longer public: with a token
// configured it requires a matching bearer; without a token it is restricted to
// internal (loopback/private) source IPs.
func TestMetricsAccessMiddleware(t *testing.T) {
	serve := func(token, authHeader, remoteIP string) int {
		e := echo.New()
		e.GET("/metrics", func(c echo.Context) error { return c.String(http.StatusOK, "metrics") },
			MetricsAccessMiddleware(token))
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		req.RemoteAddr = remoteIP + ":33333"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("token set, correct bearer, external IP → allowed", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, serve("s3cr3t", "Bearer s3cr3t", "203.0.113.9"))
	})
	t.Run("token set, wrong bearer → 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, serve("s3cr3t", "Bearer nope", "203.0.113.9"))
	})
	t.Run("token set, missing bearer → 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, serve("s3cr3t", "", "127.0.0.1"))
	})
	t.Run("no token, external public IP → 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, serve("", "", "203.0.113.9"))
	})
	t.Run("no token, loopback → allowed", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, serve("", "", "127.0.0.1"))
	})
	t.Run("no token, private RFC1918 → allowed", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, serve("", "", "10.1.2.3"))
	})
}
