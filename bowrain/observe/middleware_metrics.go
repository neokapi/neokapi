package observe

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
)

// MetricsMiddleware returns Echo middleware that records Prometheus metrics
// for every HTTP request: total count, latency histogram, and in-flight gauge.
//
// Labels use the registered route pattern (c.Path()), e.g. "/api/v1/projects/:id",
// NOT the actual URL, to keep label cardinality bounded.
//
// sizeRoutes opts specific route patterns into the response-size histogram
// (HTTPResponseSizeBytes). Only listed routes are observed so the extra
// histogram stays cheap; pass the payload-hot endpoints (e.g. the workspace
// project list and the translation dashboard).
func MetricsMiddleware(sizeRoutes ...string) echo.MiddlewareFunc {
	sized := make(map[string]bool, len(sizeRoutes))
	for _, r := range sizeRoutes {
		sized[r] = true
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			HTTPRequestsInFlight.Inc()
			defer HTTPRequestsInFlight.Dec()

			start := time.Now()
			err := next(c)
			duration := time.Since(start).Seconds()

			var status int
			var size int64
			if r, rerr := echo.UnwrapResponse(c.Response()); rerr == nil {
				status, size = r.Status, r.Size
			}
			route := c.Path()
			if route == "" {
				route = "unknown"
			}
			method := c.Request().Method

			code := strconv.Itoa(status)
			HTTPRequestsTotal.WithLabelValues(method, route, code).Inc()
			HTTPRequestDuration.WithLabelValues(method, route, code).Observe(duration)
			if sized[route] {
				HTTPResponseSizeBytes.WithLabelValues(method, route, code).
					Observe(float64(size))
			}

			return err
		}
	}
}
