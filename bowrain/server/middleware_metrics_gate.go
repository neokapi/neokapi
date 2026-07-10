package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// MetricsAccessMiddleware gates the Prometheus /metrics endpoint, which would
// otherwise expose internal route cardinality, workspace/label counts, and
// request timings to any unauthenticated client that can reach the server.
//
// It mirrors how observe.StartPprofServer restricts pprof (never openly
// reachable) with the smallest change that keeps in-cluster scraping working:
//
//   - When token != "" (BOWRAIN_METRICS_TOKEN is set): require a matching
//     "Authorization: Bearer <token>" (constant-time compare). This lets an
//     external, token-authenticated scraper reach /metrics.
//   - When token == "": allow only loopback / private / link-local source IPs.
//     A Prometheus sidecar scraping the pod directly presents a private RealIP
//     and is admitted; a client arriving through the public ingress presents a
//     routable RealIP and is refused.
//
// Refused requests get 404 (not 401/403) so the endpoint's existence is not
// confirmed to an unauthenticated prober (anti-enumeration, matching the
// server's other tenancy 404s).
func MetricsAccessMiddleware(token string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if token != "" {
				if metricsBearerMatches(c, token) {
					return next(c)
				}
				return c.NoContent(http.StatusNotFound)
			}
			if isInternalIP(c.RealIP()) {
				return next(c)
			}
			return c.NoContent(http.StatusNotFound)
		}
	}
}

// metricsBearerMatches reports whether the request carries a Bearer token equal
// to want, compared in constant time to avoid leaking the token via timing.
func metricsBearerMatches(c echo.Context, want string) bool {
	got, ok := strings.CutPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// isInternalIP reports whether ip is a loopback, RFC1918/ULA private, or
// link-local address — i.e. reachable only from inside the cluster/host, not
// routable from the public internet.
func isInternalIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast()
}
