package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedRefreshToken creates a user and a persisted refresh token, returning the
// raw token to present to the refresh endpoint.
func seedRefreshToken(t *testing.T, srv *Server) string {
	t.Helper()
	ctx := t.Context()
	user := &platauth.User{ID: "u-refresh", Email: "refresh@example.com", Name: "Refresh User"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	raw, err := platauth.GenerateRefreshToken()
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(raw))
	_, err = srv.AuthStore.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(sum[:]), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	return raw
}

// TestTokenRefresh_BodyPathReturnsTokens: the CLI/desktop path presents the
// refresh token in the JSON body and, authenticating by the token itself,
// receives the rotated tokens in the body.
func TestTokenRefresh_BodyPathReturnsTokens(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)
	raw := seedRefreshToken(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+raw+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp platauth.TokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken, "CLI path must receive an access token in the body")
	assert.NotEmpty(t, resp.RefreshToken, "CLI path must receive the rotated refresh token in the body")
	assert.Empty(t, resp.IDToken, "a browser-facing TokenResponse must never carry an id_token")
}

// TestTokenRefresh_CookiePathRequiresCSRF: a cookie-carried refresh with no
// CSRF header is refused, and one with the header succeeds but returns NO
// tokens in the body — they ride back only as HttpOnly cookies, so an XSS
// cannot exfiltrate the 30-day refresh token.
func TestTokenRefresh_CookiePathRequiresCSRF(t *testing.T) {
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)

	// Missing CSRF header → refused before any rotation.
	raw := seedRefreshToken(t, srv)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: raw})
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), raw, "a rejected refresh must not echo the token")

	// With the CSRF header → 200, but no tokens in the body.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req2.AddCookie(&http.Cookie{Name: refreshCookieName, Value: raw})
	req2.Header.Set(csrfHeaderName, "1")
	rec2 := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	var resp platauth.TokenResponse
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	assert.Empty(t, resp.AccessToken, "the cookie path must not echo the access token in the body")
	assert.Empty(t, resp.RefreshToken, "the cookie path must not echo the 30-day refresh token in the body")
	assert.Empty(t, resp.IDToken, "a browser-facing TokenResponse must never carry an id_token")

	// The rotated tokens ride back as cookies instead.
	var sawRefreshCookie bool
	for _, ck := range rec2.Result().Cookies() {
		if ck.Name == refreshCookieName && ck.Value != "" {
			sawRefreshCookie = true
			assert.True(t, ck.HttpOnly, "the refresh cookie must be HttpOnly")
		}
	}
	assert.True(t, sawRefreshCookie, "the cookie path must set a new refresh cookie")
}
