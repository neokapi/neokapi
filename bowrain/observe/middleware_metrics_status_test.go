package observe

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestCount(t *testing.T, method, route, status string) float64 {
	t.Helper()
	counter, ok := HTTPRequestsTotal.WithLabelValues(method, route, status).(prometheus.Metric)
	require.True(t, ok)
	pb := &dto.Metric{}
	require.NoError(t, counter.Write(pb))
	return pb.GetCounter().GetValue()
}

// The status label is the status the client received, also when the response
// is written by the error handler after a handler or the router returns an
// error.
func TestMetricsMiddlewareStatusIsTheAnswer(t *testing.T) {
	e := echo.New()
	e.Use(MetricsMiddleware())
	g := e.Group("/metrics-status-test")
	g.Use(func(next echo.HandlerFunc) echo.HandlerFunc { return next })
	g.GET("/missing", func(*echo.Context) error { return echo.ErrNotFound })
	g.GET("/broken", func(*echo.Context) error { return errors.New("database is gone") })
	g.GET("/declined", func(*echo.Context) error {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "not configured")
	})
	g.GET("/teapot", func(c *echo.Context) error { return c.NoContent(http.StatusTeapot) })

	for _, tc := range []struct {
		name, path, route string
		want              int
	}{
		{"returned 404", "/metrics-status-test/missing", "/metrics-status-test/missing", http.StatusNotFound},
		{"router 404 under a group", "/metrics-status-test/nowhere", "/metrics-status-test/*", http.StatusNotFound},
		{"returned bare error", "/metrics-status-test/broken", "/metrics-status-test/broken", http.StatusInternalServerError},
		{"returned HTTPError", "/metrics-status-test/declined", "/metrics-status-test/declined", http.StatusServiceUnavailable},
		{"handler writes its own status", "/metrics-status-test/teapot", "/metrics-status-test/teapot", http.StatusTeapot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := strconv.Itoa(tc.want)
			before := requestCount(t, http.MethodGet, tc.route, code)
			before200 := requestCount(t, http.MethodGet, tc.route, "200")

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			require.Equal(t, tc.want, rec.Code)

			assert.Equal(t, before+1, requestCount(t, http.MethodGet, tc.route, code),
				"counted under the status the client received")
			assert.Equal(t, before200, requestCount(t, http.MethodGet, tc.route, "200"),
				"not counted as a success")
		})
	}
}
