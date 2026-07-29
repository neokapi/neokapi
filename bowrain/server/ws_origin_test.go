package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWSOriginPatterns covers the allowlist itself: what it contains, and — the
// part that matters — what it deliberately leaves out.
func TestWSOriginPatterns(t *testing.T) {
	tests := []struct {
		name          string
		cfg           Config
		forwardedHost string
		want          []string
	}{
		{
			name: "development allows localhost so a dev server on another port can connect",
			cfg:  Config{},
			want: []string{
				"localhost", "localhost:*",
				"127.0.0.1", "127.0.0.1:*",
				"[::1]", "[::1]:*",
			},
		},
		{
			name: "production allows nothing beyond the request's own origin",
			cfg:  Config{OIDCPublicURL: "https://auth.bowrain.cloud/realms/bowrain"},
			want: nil,
		},
		{
			name:          "production honours the proxy's public host",
			cfg:           Config{OIDCPublicURL: "https://auth.bowrain.cloud/realms/bowrain"},
			forwardedHost: "app.bowrain.cloud",
			want:          []string{"app.bowrain.cloud"},
		},
		{
			name: "the landing origin is not authorized for a socket",
			cfg: Config{
				OIDCPublicURL: "https://auth.bowrain.cloud/realms/bowrain",
				PublicSiteURL: "https://bowrain.cloud",
			},
			want: nil,
		},
		{
			name: "the identity provider's origin is not authorized either",
			cfg:  Config{OIDCPublicURL: "https://auth.bowrain.cloud/realms/bowrain"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{Config: tt.cfg}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			c := e.NewContext(req, httptest.NewRecorder())

			got := s.wsOriginPatterns(c)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "*", "a wildcard would authorize every origin")
		})
	}
}

// wsOriginTestCase is one dial against a live handler.
type wsOriginTestCase struct {
	name      string
	origin    string
	cfg       Config
	wantAllow bool
}

var wsOriginCases = []wsOriginTestCase{
	{
		name:      "no Origin — a non-browser client such as the CLI",
		origin:    "",
		wantAllow: true,
	},
	{
		name:      "a localhost origin in development",
		origin:    "http://localhost:5173",
		wantAllow: true,
	},
	{
		name:      "a sibling host under the same registrable domain",
		origin:    "https://docs.bowrain.cloud",
		wantAllow: false,
	},
	{
		name:      "user content on a subdomain",
		origin:    "https://user-content.bowrain.cloud",
		wantAllow: false,
	},
	{
		name:      "an unrelated origin",
		origin:    "https://attacker.example",
		wantAllow: false,
	},
	{
		name:      "a localhost origin once a production OIDC URL is configured",
		origin:    "http://localhost:5173",
		cfg:       Config{OIDCPublicURL: "https://auth.bowrain.cloud/realms/bowrain"},
		wantAllow: false,
	},
}

// dialWS opens a WebSocket against handler with the given Origin, returning
// whether the upgrade succeeded.
func dialWS(t *testing.T, s *Server, handler echo.HandlerFunc, path, origin string, subprotocols []string) bool {
	t.Helper()

	e := echo.New()
	e.GET("/*", handler, func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// The sockets require an authenticated user; the finding is that a
			// session cookie satisfies that from any origin, so the test grants
			// the identity and varies only the origin.
			c.Set("user_id", "user1")
			c.Set("user_name", "Alice")
			c.SetParamNames("ws", "pid")
			c.SetParamValues("acme", "proj1")
			return next(c)
		}
	})

	srv := httptest.NewServer(e)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}

	conn, resp, err := websocket.Dial(ctx, "ws"+srv.URL[4:]+path, &websocket.DialOptions{
		Subprotocols: subprotocols,
		HTTPHeader:   headers,
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return false
	}
	_ = conn.CloseNow()
	return true
}

// TestCollabWebSocket_OriginPolicy pins that a collaboration room — join, read,
// and write access to a document other people are editing — is reachable only
// from the app's own origin.
func TestCollabWebSocket_OriginPolicy(t *testing.T) {
	for _, tt := range wsOriginCases {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{collabHub: newCollabHub(), Config: tt.cfg}
			ok := dialWS(t, s, s.HandleCollabWebSocket,
				"/ws/acme/proj1/file.html?locale=fr", tt.origin, []string{"yjs"})
			assert.Equal(t, tt.wantAllow, ok)
		})
	}
}

// TestNotificationWebSocket_OriginPolicy pins the same boundary for the
// notification stream, which carries a user's workspace activity.
func TestNotificationWebSocket_OriginPolicy(t *testing.T) {
	for _, tt := range wsOriginCases {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{notificationHub: newNotificationHub(), Config: tt.cfg}
			ok := dialWS(t, s, s.HandleNotificationWebSocket,
				"/ws/acme/notifications/ws", tt.origin, nil)
			assert.Equal(t, tt.wantAllow, ok)
		})
	}
}

// TestWebSocketsRequireAuthenticationBeforeOrigin keeps the two gates
// independent: an unauthenticated caller is refused even from a good origin.
func TestWebSocketsRequireAuthenticationBeforeOrigin(t *testing.T) {
	s := &Server{collabHub: newCollabHub(), notificationHub: newNotificationHub()}
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws1/p1/collab/main?item=f.html&locale=fr", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("ws", "pid")
	c.SetParamValues("ws1", "p1")

	err := s.HandleCollabWebSocket(c)
	require.Error(t, err)
	var he *echo.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusUnauthorized, he.Code)
}
