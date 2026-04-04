package observe

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// MetricsMiddleware returns Echo middleware that records Prometheus metrics
// for every HTTP request: total count, latency histogram, and in-flight gauge.
//
// Labels use the registered route pattern (c.Path()) — e.g. "/api/v1/projects/:id"
// — NOT the actual URL, to keep label cardinality bounded.
func MetricsMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			HTTPRequestsInFlight.Inc()
			defer HTTPRequestsInFlight.Dec()

			start := time.Now()
			err := next(c)
			duration := time.Since(start).Seconds()

			status := strconv.Itoa(c.Response().Status)
			route := c.Path()
			if route == "" {
				route = "unknown"
			}
			method := c.Request().Method

			HTTPRequestsTotal.WithLabelValues(method, route, status).Inc()
			HTTPRequestDuration.WithLabelValues(method, route, status).Observe(duration)

			return err
		}
	}
}
