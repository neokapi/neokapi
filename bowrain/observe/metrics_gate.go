package observe

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// MetricsAccessMiddlewareStd is the net/http twin of the server's echo-based
// metrics gate, for the worker's standalone health/metrics mux. Same policy:
//
//   - token != "" → require "Authorization: Bearer <token>" (constant-time).
//   - token == "" → allow only loopback/private/link-local source IPs (an
//     in-cluster scraper reaching the pod directly), refuse public callers.
//
// Refused requests get 404 so the endpoint's existence is not confirmed.
func MetricsAccessMiddlewareStd(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			if metricsBearerMatchesStd(r, token) {
				next.ServeHTTP(w, r)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if isInternalIPStd(remoteIP(r)) {
			next.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

func metricsBearerMatchesStd(r *http.Request, want string) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// remoteIP extracts the source IP from RemoteAddr. The worker's metrics port is
// not behind the app's trusted-proxy IP extractor, so RemoteAddr is the direct
// peer — exactly what an in-cluster scraper presents; XFF is intentionally not
// consulted here.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isInternalIPStd(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast()
}
