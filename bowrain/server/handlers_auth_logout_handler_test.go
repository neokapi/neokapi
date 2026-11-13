package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLogoutTestServer returns a server whose identity provider advertises an
// end_session_endpoint, and the URL of that endpoint.
func newLogoutTestServer(t *testing.T, provider string) (*Server, string) {
	t.Helper()
	var issuer string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/certs",
			"end_session_endpoint":   issuer + "/logout",
		})
	}))
	t.Cleanup(idp.Close)
	issuer = idp.URL

	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = issuer
	cfg.OIDCClientID = "test-client"
	cfg.AuthProvider = provider
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	return srv, issuer + "/logout"
}

func postLogout(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.POST("/api/v1/auth/logout", srv.HandleAuthLogout)
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Host = "app.example.com"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func endSessionQuery(t *testing.T, rec *httptest.ResponseRecorder, endpoint string) url.Values {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	u, err := url.Parse(resp["end_session_url"])
	require.NoError(t, err)
	assert.Equal(t, endpoint, u.Scheme+"://"+u.Host+u.Path)
	return u.Query()
}

func TestLogoutWithoutBodyReturnsToTheAppOrigin(t *testing.T) {
	srv, endpoint := newLogoutTestServer(t, AuthProviderKeycloak)
	q := endSessionQuery(t, postLogout(t, srv, ""), endpoint)
	assert.Equal(t, "http://app.example.com/", q.Get("post_logout_redirect_uri"))
}

func TestLogoutReturnsToTheNativeAppRedirect(t *testing.T) {
	t.Run("keycloak", func(t *testing.T) {
		srv, endpoint := newLogoutTestServer(t, AuthProviderKeycloak)
		rec := postLogout(t, srv, `{"post_logout_redirect_uri":"bowrain://auth/signed-out"}`)
		q := endSessionQuery(t, rec, endpoint)
		assert.Equal(t, "bowrain://auth/signed-out", q.Get("post_logout_redirect_uri"))
		assert.Equal(t, "test-client", q.Get("client_id"))
	})
	t.Run("cognito", func(t *testing.T) {
		srv, endpoint := newLogoutTestServer(t, AuthProviderCognito)
		rec := postLogout(t, srv, `{"post_logout_redirect_uri":"bowrain://auth/signed-out"}`)
		q := endSessionQuery(t, rec, endpoint)
		assert.Equal(t, "bowrain://auth/signed-out", q.Get("logout_uri"))
		assert.Equal(t, "test-client", q.Get("client_id"))
		assert.Empty(t, q.Get("post_logout_redirect_uri"))
	})
}

func TestLogoutRefusesARedirectOutsideTheAllowlist(t *testing.T) {
	srv, _ := newLogoutTestServer(t, AuthProviderKeycloak)
	for _, redirect := range []string{"https://evil.example.com/", "javascript:alert(1)", "myapp://signed-out"} {
		t.Run(redirect, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"post_logout_redirect_uri": redirect})
			require.NoError(t, err)
			rec := postLogout(t, srv, string(body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.NotContains(t, rec.Body.String(), "end_session_url")
		})
	}
}
