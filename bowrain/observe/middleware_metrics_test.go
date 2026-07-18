package observe

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// histogramFor reads the current dto snapshot of one labeled series of the
// response-size histogram.
func histogramFor(t *testing.T, method, route, status string) *dto.Histogram {
	t.Helper()
	metric, ok := HTTPResponseSizeBytes.WithLabelValues(method, route, status).(prometheus.Metric)
	require.True(t, ok)
	pb := &dto.Metric{}
	require.NoError(t, metric.Write(pb))
	return pb.GetHistogram()
}

// TestMetricsMiddlewareResponseSizeOptIn asserts the response-size histogram
// is recorded only for routes explicitly opted in via MetricsMiddleware's
// sizeRoutes, and that it observes the actual body size.
func TestMetricsMiddlewareResponseSizeOptIn(t *testing.T) {
	e := echo.New()
	e.Use(MetricsMiddleware("/hot-metrics-test"))
	e.GET("/hot-metrics-test", func(c echo.Context) error {
		return c.String(http.StatusOK, "0123456789") // 10 bytes
	})
	e.GET("/cold-metrics-test", func(c echo.Context) error {
		return c.String(http.StatusOK, "hi")
	})

	serve := func(path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	before := histogramFor(t, "GET", "/hot-metrics-test", "200").GetSampleCount()
	serve("/hot-metrics-test")
	serve("/cold-metrics-test")

	hot := histogramFor(t, "GET", "/hot-metrics-test", "200")
	assert.Equal(t, before+1, hot.GetSampleCount(), "opted-in route records one size sample")
	assert.Equal(t, float64(10), hot.GetSampleSum(), "observed value is the response body size")

	cold := histogramFor(t, "GET", "/cold-metrics-test", "200")
	assert.Zero(t, cold.GetSampleCount(), "non-listed routes record no size samples")
}

// TestMetricsMiddlewareDurationAlwaysRecorded asserts the pre-existing
// duration/count series still record for every route regardless of the
// size opt-in list.
func TestMetricsMiddlewareDurationAlwaysRecorded(t *testing.T) {
	e := echo.New()
	e.Use(MetricsMiddleware())
	e.GET("/any-metrics-test", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/any-metrics-test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	counter, ok := HTTPRequestsTotal.WithLabelValues("GET", "/any-metrics-test", "204").(prometheus.Metric)
	require.True(t, ok)
	pb := &dto.Metric{}
	require.NoError(t, counter.Write(pb))
	assert.Equal(t, float64(1), pb.GetCounter().GetValue())
}
