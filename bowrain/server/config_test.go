package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

// The Secure flag on a session cookie must not depend only on a forwarded
// header.
//
// echo's Scheme() believes four different forwarded-proto headers and has no
// trusted-proxy gate, so behind a TLS-terminating edge the flag is decided by
// something the task does not control. ForceSecureCookies is the override that
// removes that dependency, and it is now defaulted on for production
// configurations — which is what this pins.
func TestCookieSecureDoesNotDependOnAForwardedHeaderAlone(t *testing.T) {
	req := func(proto string) echo.Context {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if proto != "" {
			r.Header.Set("X-Forwarded-Proto", proto)
		}
		return echo.New().NewContext(r, httptest.NewRecorder())
	}

	for _, tc := range []struct {
		name      string
		cfg       Config
		forwarded string
		want      bool
	}{
		{
			name: "forced on: a plain-http request still gets Secure",
			cfg:  Config{ForceSecureCookies: true},
			want: true,
		},
		{
			name:      "forced on: a stripped or forged proto header cannot clear it",
			cfg:       Config{ForceSecureCookies: true},
			forwarded: "http",
			want:      true,
		},
		{
			name:      "not forced: the header alone decides, which is the exposure",
			cfg:       Config{},
			forwarded: "http",
			want:      false,
		},
		{
			name:      "not forced: an https hop is still honoured",
			cfg:       Config{},
			forwarded: "https",
			want:      true,
		},
		{
			// Development serves plain http, and browsers drop Secure cookies
			// over it — so the default must not reach here.
			name: "development leaves it to the scheme",
			cfg:  Config{DevMode: true},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{Config: tc.cfg}
			assert.Equal(t, tc.want, s.cookieSecure(req(tc.forwarded)))
		})
	}
}

// DevMode is the whole dev/prod discriminator, and its zero value is
// production. A config that names only the OIDC issuer — the shape the
// self-hosting documentation describes — must not select the development
// origin policy, which accepts credentialed cross-origin requests from any
// localhost page.
func TestZeroConfigIsProductionForOriginPolicy(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{OIDCIssuerURL: "https://auth.example.com/realms/bowrain"},
		{OIDCIssuerURL: "https://auth.example.com/realms/bowrain", OIDCClientID: "bowrain"},
	} {
		s := &Server{Config: cfg}

		cors := s.corsConfig()
		assert.Nil(t, cors.AllowOriginFunc,
			"the dynamic localhost policy must be reachable only via DevMode")
		assert.NotContains(t, cors.AllowOrigins, "*")

		e := echo.New()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		assert.Empty(t, s.wsOriginPatterns(c),
			"no localhost socket origin without DevMode")
	}
}
