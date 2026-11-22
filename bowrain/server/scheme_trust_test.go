package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

// The server builds sign-in redirect URLs, email links and the HSTS header
// from c.Scheme(). Behind a TLS-terminating proxy the connection is plain
// HTTP, so the scheme comes from X-Forwarded-Proto, which is believed only
// from the proxies configureIPExtractor trusts: loopback, link-local, private
// ranges (the production ALB sits in 10.40.0.0/16) and BOWRAIN_TRUSTED_PROXIES.
func TestSchemeFollowsTrustedProxies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		remoteAddr string
		trusted    string
		proto      string
		want       string
	}{
		{"ALB in the private VPC", "10.40.11.23:41234", "", "https", "https"},
		{"proxy on the same host", "127.0.0.1:41234", "", "https", "https"},
		{"private proxy, plain HTTP", "10.40.11.23:41234", "", "http", "http"},
		{"no proxy header", "10.40.11.23:41234", "", "", "http"},
		{"public peer forging the header", "203.0.113.5:41234", "", "https", "http"},
		{"public proxy listed in BOWRAIN_TRUSTED_PROXIES", "203.0.113.5:41234", "203.0.113.0/24", "https", "https"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BOWRAIN_TRUSTED_PROXIES", tc.trusted)
			e := wireEcho(t)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			r.RemoteAddr = tc.remoteAddr
			r.Host = "app.bowrain.example"
			if tc.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tc.proto)
			}

			// The URL the server builds for a redirect or an email link.
			c := e.NewContext(r, httptest.NewRecorder())
			assert.Equal(t, tc.want+"://app.bowrain.example", requestBaseURL(c))

			// HSTS is sent exactly when the scheme is https.
			rec, _ := serveWire(e, r)
			assert.Equal(t, http.StatusOK, rec.Code)
			if tc.want == "https" {
				assert.Equal(t, "max-age=31536000", rec.Header().Get("Strict-Transport-Security"))
			} else {
				assert.Empty(t, rec.Header().Get("Strict-Transport-Security"))
			}
		})
	}
}

// Session cookies are Secure in production whatever the scheme says.
func TestCookieSecureForcedInProduction(t *testing.T) {
	s := &Server{Config: Config{ForceSecureCookies: true}}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.5:41234"
	c := echo.New().NewContext(r, httptest.NewRecorder())
	assert.True(t, s.cookieSecure(c))
}
