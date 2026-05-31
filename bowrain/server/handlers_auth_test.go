package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeRefreshFailingAuthStore wraps a real auth.AuthStore but forces
// StoreRefreshToken to fail. It is used to verify that auth handlers never
// hand a client a refresh token that could not be persisted.
type storeRefreshFailingAuthStore struct {
	auth.AuthStore
}

func (s *storeRefreshFailingAuthStore) StoreRefreshToken(context.Context, string, string, time.Time) (string, error) {
	return "", errors.New("simulated store failure")
}

func TestDeviceAuthStartRespectsForwardedHeaders(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	startForm := url.Values{"client_id": {"test-client"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start",
		strings.NewReader(startForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Forwarded-Host", "bowrain.mymac")
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp platauth.DeviceAuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "https://bowrain.mymac/device/verify", resp.VerificationURI)
}

func TestHandleDeviceVerificationFormValues(t *testing.T) {
	// No OIDC configured → uses direct authorization with form values.
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Step 1: Start device auth to get a user_code.
	startForm := url.Values{"client_id": {"test-client"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start",
		strings.NewReader(startForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var startResp platauth.DeviceAuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	userCode := startResp.UserCode
	deviceCode := startResp.DeviceCode
	require.NotEmpty(t, userCode)
	require.NotEmpty(t, deviceCode)

	// Step 2: Verify using form values (not query params) with email/name.
	verifyForm := url.Values{
		"user_code": {userCode},
		"email":     {"test@example.com"},
		"name":      {"Test User"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/verify",
		strings.NewReader(verifyForm.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusFound, rec2.Code)
	assert.Equal(t, "/device/authorized", rec2.Header().Get("Location"))

	// Verify the entry was authorized with the correct email/name.
	ctx := t.Context()
	entry, err := sessionGet[deviceCodeEntry](ctx, srv.SessionStore, prefixDeviceCode, deviceCode)
	if err == nil {
		assert.True(t, entry.Authorized)
		assert.Equal(t, "test@example.com", entry.UserEmail)
		assert.Equal(t, "Test User", entry.UserName)
	}
}

func TestHandleDeviceAuthPollStoreRefreshTokenError(t *testing.T) {
	// When persisting the refresh token fails, the poll handler must return a
	// 500 rather than hand the client an unredeemable refresh token.
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	// Wrap the wired AuthStore so StoreRefreshToken always fails.
	srv.AuthStore = &storeRefreshFailingAuthStore{AuthStore: srv.AuthStore}
	e := srv.GetEcho()

	// Step 1: Start device auth to get a device_code + user_code.
	startForm := url.Values{"client_id": {"test-client"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start",
		strings.NewReader(startForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var startResp platauth.DeviceAuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	require.NotEmpty(t, startResp.UserCode)
	require.NotEmpty(t, startResp.DeviceCode)

	// Step 2: Authorize the device directly (no OIDC) with explicit email.
	verifyForm := url.Values{
		"user_code": {startResp.UserCode},
		"email":     {"test@example.com"},
		"name":      {"Test User"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/verify",
		strings.NewReader(verifyForm.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusFound, rec2.Code)

	// Step 3: Poll — the device is authorized, so the handler reaches
	// StoreRefreshToken, which fails. Expect a 500 and no refresh token.
	pollForm := url.Values{"device_code": {startResp.DeviceCode}}
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/poll",
		strings.NewReader(pollForm.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec3 := httptest.NewRecorder()
	e.ServeHTTP(rec3, req3)

	assert.Equal(t, http.StatusInternalServerError, rec3.Code)
	assert.Contains(t, rec3.Body.String(), "failed to store refresh token")
	assert.NotContains(t, rec3.Body.String(), "refresh_token")
}

func TestHandleDeviceVerificationDefaultValues(t *testing.T) {
	// No OIDC configured, no email/name provided → defaults should be used.
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	// Manually create a device code entry and user code index.
	ctx := t.Context()
	entry := &deviceCodeEntry{UserCode: "aaaa-bbbb"}
	require.NoError(t, sessionSet(ctx, srv.SessionStore, prefixDeviceCode, "test-device-default", entry, authStateTTL))
	require.NoError(t, srv.SessionStore.Set(ctx, prefixUserCode+"aaaa-bbbb", []byte("test-device-default"), authStateTTL))

	e := echo.New()
	e.POST("/verify", func(c echo.Context) error {
		return srv.handleDeviceVerification(c, "aaaa-bbbb")
	})

	req := httptest.NewRequest(http.MethodPost, "/verify", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/device/authorized", rec.Header().Get("Location"))

	updated, err := sessionGet[deviceCodeEntry](ctx, srv.SessionStore, prefixDeviceCode, "test-device-default")
	require.NoError(t, err)
	assert.Equal(t, "user@bowrain.local", updated.UserEmail)
	assert.Equal(t, "Bowrain User", updated.UserName)
}

func TestHandleDeviceVerificationOIDCRedirect(t *testing.T) {
	// With OIDC configured, device verification should redirect to the OIDC provider.
	// We use a mock OIDC server that returns a discovery document.
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + "ISSUER_PLACEHOLDER" + `",
				"authorization_endpoint": "` + "ISSUER_PLACEHOLDER" + `/auth",
				"token_endpoint": "` + "ISSUER_PLACEHOLDER" + `/token",
				"jwks_uri": "` + "ISSUER_PLACEHOLDER" + `/certs"
			}`))
		}
	}))
	defer mockOIDC.Close()

	// Patch the discovery response to use the real mock URL.
	mockOIDC.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + mockOIDC.URL + `",
				"authorization_endpoint": "` + mockOIDC.URL + `/auth",
				"token_endpoint": "` + mockOIDC.URL + `/token",
				"jwks_uri": "` + mockOIDC.URL + `/certs"
			}`))
		}
	})

	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = mockOIDC.URL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	// Create a pending device code entry with user code index.
	ctx := t.Context()
	entry := &deviceCodeEntry{
		UserCode: "cccc-dddd",
		ClientID: "kapi-cli",
	}
	require.NoError(t, sessionSet(ctx, srv.SessionStore, prefixDeviceCode, "test-oidc-device", entry, authStateTTL))
	require.NoError(t, srv.SessionStore.Set(ctx, prefixUserCode+"cccc-dddd", []byte("test-oidc-device"), authStateTTL))

	e := echo.New()
	e.POST("/verify", func(c echo.Context) error {
		return srv.handleDeviceVerification(c, "cccc-dddd")
	})

	req := httptest.NewRequest(http.MethodPost, "/verify", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Should redirect (302) to the OIDC authorization endpoint.
	assert.Equal(t, http.StatusFound, rec.Code)
	location := rec.Header().Get("Location")
	assert.Contains(t, location, mockOIDC.URL+"/auth")
	assert.Contains(t, location, "response_type=code")
	assert.Contains(t, location, "scope=openid+profile+email")

	// Parse the redirect URL and verify the redirect_uri parameter.
	parsed, _ := url.Parse(location)
	redirectURI := parsed.Query().Get("redirect_uri")
	assert.Contains(t, redirectURI, "/api/v1/auth/device/callback")
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)

	// Verify PKCE and nonce are present in redirect URL.
	assert.NotEmpty(t, parsed.Query().Get("code_challenge"), "code_challenge must be set")
	assert.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
	assert.NotEmpty(t, parsed.Query().Get("nonce"), "nonce must be set")

	verifyEntry, err := sessionGet[deviceVerifyEntry](ctx, srv.SessionStore, prefixDeviceVerify, state)
	require.NoError(t, err, "state should be stored in session store")
	assert.Equal(t, "test-oidc-device", verifyEntry.DeviceCode)
	assert.NotEmpty(t, verifyEntry.CodeVerifier, "entry should have CodeVerifier")
	assert.NotEmpty(t, verifyEntry.Nonce, "entry should have Nonce")
}

func TestHandleDeviceAuthCallbackMissingParams(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Missing code and state → should redirect to verify page with error.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/callback", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/device/verify?error=")
}

func TestHandleDeviceAuthCallbackInvalidState(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Valid code but unknown state → should redirect to verify page with error.
	params := url.Values{
		"code":  {"some-code"},
		"state": {"nonexistent-state"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/callback?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/device/verify?error=")
}

func TestDeviceVerifyStatesCleanup(t *testing.T) {
	// Verify state entries are consumed after use (even on OIDC exchange failure).
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := t.Context()
	state := "test-device-cleanup"
	verifyEntry := &deviceVerifyEntry{DeviceCode: "dev-code-123"}
	require.NoError(t, sessionSet(ctx, srv.SessionStore, prefixDeviceVerify, state, verifyEntry, authStateTTL))

	e := echo.New()
	e.GET("/callback", func(c echo.Context) error {
		return srv.HandleDeviceAuthCallback(c)
	})

	// Trigger callback — will fail at OIDC exchange but state should be consumed.
	params := url.Values{
		"code":  {"some-code"},
		"state": {state},
	}
	req := httptest.NewRequest(http.MethodGet, "/callback?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// State should have been consumed regardless of OIDC exchange outcome.
	_, err := sessionGet[deviceVerifyEntry](ctx, srv.SessionStore, prefixDeviceVerify, state)
	assert.ErrorIs(t, err, ErrSessionNotFound, "state entry should be consumed after callback")
}

// --- Desktop auth (PKCE) tests ---

func TestHandleDesktopLoginMissingParams(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180" // needed to pass OIDC check
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// No redirect_uri or code_challenge → should get 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "redirect_uri and code_challenge required")
}

func TestHandleDesktopLoginNonLocalhostRedirect(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Non-localhost redirect_uri → should get 400.
	params := url.Values{
		"redirect_uri":          {"http://evil.com/callback"},
		"code_challenge":        {"test-challenge"},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "redirect_uri must be")
}

func TestHandleDesktopLoginAcceptsBowrainScheme(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// bowrain:// scheme should be accepted (will fail at OIDC discovery, but not at redirect_uri validation).
	params := url.Values{
		"redirect_uri":          {"bowrain://auth/callback"},
		"code_challenge":        {"test-challenge"},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	// Should NOT be 400 "redirect_uri must be..." — it should proceed to OIDC discovery (and fail there since no real OIDC).
	assert.NotEqual(t, http.StatusBadRequest, rec.Code)
}

func TestHandleDesktopLoginUnsupportedChallengeMethod(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	params := url.Values{
		"redirect_uri":          {"http://127.0.0.1:12345/callback"},
		"code_challenge":        {"test-challenge"},
		"code_challenge_method": {"plain"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "only S256")
}

func TestHandleDesktopCallbackMissingState(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Missing code and state → should get 400 HTML error.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Authentication Failed")
}

func TestHandleDesktopCallbackInvalidState(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Valid code but unknown state → should get 400.
	params := url.Values{
		"code":  {"some-code"},
		"state": {"nonexistent-state"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid or Expired")
}

func TestDesktopAuthStatesCleanup(t *testing.T) {
	// Verify that state entries are cleaned up after use.
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := t.Context()
	state := "test-cleanup-state"
	desktopEntry := &desktopAuthEntry{
		RedirectURI:   "http://127.0.0.1:54321/callback",
		CodeChallenge: "challenge",
	}
	require.NoError(t, sessionSet(ctx, srv.SessionStore, prefixDesktopAuth, state, desktopEntry, authStateTTL))

	e := echo.New()
	e.GET("/callback", func(c echo.Context) error {
		return srv.HandleDesktopCallback(c)
	})

	// Trigger callback — will fail at OIDC exchange but state should be consumed.
	params := url.Values{
		"code":  {"some-code"},
		"state": {state},
	}
	req := httptest.NewRequest(http.MethodGet, "/callback?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// State should have been consumed regardless of OIDC exchange outcome.
	_, err := sessionGet[desktopAuthEntry](ctx, srv.SessionStore, prefixDesktopAuth, state)
	assert.ErrorIs(t, err, ErrSessionNotFound, "state entry should be consumed after callback")
}

// --- Web flow state validation tests ---

func TestWebFlowStateStoredAndConsumed(t *testing.T) {
	// HandleAuthLogin should store a state entry; handleOIDCCodeExchange should consume it.
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + "PLACEHOLDER" + `",
				"authorization_endpoint": "` + "PLACEHOLDER" + `/auth",
				"token_endpoint": "` + "PLACEHOLDER" + `/token",
				"jwks_uri": "` + "PLACEHOLDER" + `/certs"
			}`))
		}
	}))
	defer mockOIDC.Close()

	// Patch discovery to use real URL.
	mockOIDC.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + mockOIDC.URL + `",
				"authorization_endpoint": "` + mockOIDC.URL + `/auth",
				"token_endpoint": "` + mockOIDC.URL + `/token",
				"jwks_uri": "` + mockOIDC.URL + `/certs"
			}`))
		}
	})

	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = mockOIDC.URL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Step 1: Call HandleAuthLogin — should redirect and store state.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)

	location := rec.Header().Get("Location")
	parsed, err := url.Parse(location)
	require.NoError(t, err)

	state := parsed.Query().Get("state")
	require.NotEmpty(t, state, "redirect URL should have a state parameter")

	// Verify state is stored in session store.
	ctx := t.Context()
	entry, err := sessionGet[webAuthEntry](ctx, srv.SessionStore, prefixWebAuth, state)
	require.NoError(t, err, "state should be stored in session store")
	assert.NotEmpty(t, entry.CodeVerifier)
	assert.NotEmpty(t, entry.Nonce)

	// Step 2: Calling handleOIDCCodeExchange with the state should consume it
	// (will fail at OIDC exchange, but state should still be consumed).
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=fake-code&state="+state, nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	// State should have been consumed.
	_, err = sessionGet[webAuthEntry](ctx, srv.SessionStore, prefixWebAuth, state)
	assert.ErrorIs(t, err, ErrSessionNotFound, "state entry should be consumed after callback")
}

func TestWebFlowCallbackWithoutState(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// Callback with code but empty state → should get error HTML.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=fake-code&state=nonexistent", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid or Expired")
}

// --- PKCE + Nonce in OIDC redirects ---

func TestOIDCRedirectIncludesPKCEAndNonce(t *testing.T) {
	// All 3 flows (web, desktop, device) should include code_challenge,
	// code_challenge_method, and nonce in the OIDC redirect URL.
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "PLACEHOLDER",
				"authorization_endpoint": "PLACEHOLDER/auth",
				"token_endpoint": "PLACEHOLDER/token",
				"jwks_uri": "PLACEHOLDER/certs"
			}`))
		}
	}))
	defer mockOIDC.Close()

	mockOIDC.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + mockOIDC.URL + `",
				"authorization_endpoint": "` + mockOIDC.URL + `/auth",
				"token_endpoint": "` + mockOIDC.URL + `/token",
				"jwks_uri": "` + mockOIDC.URL + `/certs"
			}`))
		}
	})

	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = mockOIDC.URL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	assertPKCEAndNonce := func(t *testing.T, location string) {
		t.Helper()
		parsed, err := url.Parse(location)
		require.NoError(t, err)
		q := parsed.Query()
		assert.NotEmpty(t, q.Get("code_challenge"), "code_challenge must be set")
		assert.Equal(t, "S256", q.Get("code_challenge_method"))
		assert.NotEmpty(t, q.Get("nonce"), "nonce must be set")
		assert.NotEmpty(t, q.Get("state"), "state must be set")
	}

	t.Run("web flow", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusFound, rec.Code)
		assertPKCEAndNonce(t, rec.Header().Get("Location"))
	})

	t.Run("desktop flow", func(t *testing.T) {
		params := url.Values{
			"redirect_uri":          {"http://127.0.0.1:12345/callback"},
			"code_challenge":        {"client-challenge"},
			"code_challenge_method": {"S256"},
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login?"+params.Encode(), nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusFound, rec.Code)
		assertPKCEAndNonce(t, rec.Header().Get("Location"))
	})

	t.Run("device flow", func(t *testing.T) {
		// Set up a pending device code via session store.
		ctx := t.Context()
		entry := &deviceCodeEntry{
			UserCode: "eeee-ffff",
			ClientID: "kapi-cli",
		}
		require.NoError(t, sessionSet(ctx, srv.SessionStore, prefixDeviceCode, "pkce-test-device", entry, authStateTTL))
		require.NoError(t, srv.SessionStore.Set(ctx, prefixUserCode+"eeee-ffff", []byte("pkce-test-device"), authStateTTL))

		ep := echo.New()
		ep.POST("/verify", func(c echo.Context) error {
			return srv.handleDeviceVerification(c, "eeee-ffff")
		})

		req := httptest.NewRequest(http.MethodPost, "/verify", nil)
		rec := httptest.NewRecorder()
		ep.ServeHTTP(rec, req)
		require.Equal(t, http.StatusFound, rec.Code)
		assertPKCEAndNonce(t, rec.Header().Get("Location"))
	})
}

func TestDesktopEntryHasPKCEAndNonce(t *testing.T) {
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	// Patch handler with correct URL now that the server is running.
	oidcURL := mockOIDC.URL
	mockOIDC.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + oidcURL + `",
				"authorization_endpoint": "` + oidcURL + `/auth",
				"token_endpoint": "` + oidcURL + `/token",
				"jwks_uri": "` + oidcURL + `/certs"
			}`))
		}
	})
	defer mockOIDC.Close()

	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = oidcURL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	params := url.Values{
		"redirect_uri":          {"http://127.0.0.1:12345/callback"},
		"code_challenge":        {"client-challenge"},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)

	// Extract state from redirect.
	parsed, _ := url.Parse(rec.Header().Get("Location"))
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)

	// Verify the stored entry has PKCE and nonce.
	ctx := t.Context()
	entry, err := sessionGet[desktopAuthEntry](ctx, srv.SessionStore, prefixDesktopAuth, state)
	require.NoError(t, err)
	assert.NotEmpty(t, entry.CodeVerifier, "desktop entry must have CodeVerifier")
	assert.NotEmpty(t, entry.Nonce, "desktop entry must have Nonce")
}

func TestDeviceVerifyEntryHasPKCEAndNonce(t *testing.T) {
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	oidcURL := mockOIDC.URL
	mockOIDC.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"issuer": "` + oidcURL + `",
				"authorization_endpoint": "` + oidcURL + `/auth",
				"token_endpoint": "` + oidcURL + `/token",
				"jwks_uri": "` + oidcURL + `/certs"
			}`))
		}
	})
	defer mockOIDC.Close()

	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = oidcURL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	// Create a pending device code via session store.
	ctx := t.Context()
	entry := &deviceCodeEntry{
		UserCode: "gggg-hhhh",
		ClientID: "kapi-cli",
	}
	require.NoError(t, sessionSet(ctx, srv.SessionStore, prefixDeviceCode, "pkce-nonce-test-device", entry, authStateTTL))
	require.NoError(t, srv.SessionStore.Set(ctx, prefixUserCode+"gggg-hhhh", []byte("pkce-nonce-test-device"), authStateTTL))

	e := echo.New()
	e.POST("/verify", func(c echo.Context) error {
		return srv.handleDeviceVerification(c, "gggg-hhhh")
	})

	req := httptest.NewRequest(http.MethodPost, "/verify", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)

	// Extract state from redirect.
	parsed, _ := url.Parse(rec.Header().Get("Location"))
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)

	// Verify the stored entry has PKCE and nonce.
	verifyEntry, err := sessionGet[deviceVerifyEntry](ctx, srv.SessionStore, prefixDeviceVerify, state)
	require.NoError(t, err)
	assert.NotEmpty(t, verifyEntry.CodeVerifier, "device verify entry must have CodeVerifier")
	assert.NotEmpty(t, verifyEntry.Nonce, "device verify entry must have Nonce")
}

func TestMemorySessionStoreExpiry(t *testing.T) {
	store := NewMemorySessionStore()
	defer store.Close()

	ctx := t.Context()

	// Set a value with very short TTL.
	require.NoError(t, store.Set(ctx, "test-key", []byte("value"), 1*time.Millisecond))

	// Should be available immediately.
	val, err := store.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val)

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	// Should be expired.
	_, err = store.Get(ctx, "test-key")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

// --- Cookie auth tests ---

// generateTestToken creates a signed JWT for testing.
func generateTestToken(t *testing.T, secret string) string {
	t.Helper()
	user := &platauth.User{ID: "user-1", Email: "test@example.com", Name: "Test User"}
	token, err := platauth.GenerateToken(user, secret, 24*time.Hour)
	require.NoError(t, err)
	return token
}

func TestAuthMiddlewareCookieFallback(t *testing.T) {
	jwtSecret := "test-secret"
	token := generateTestToken(t, jwtSecret)

	e := echo.New()
	e.Use(AuthMiddleware(jwtSecret, nil))
	e.GET("/api/v1/auth/me", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"user_id": c.Get("user_id").(string),
			"email":   c.Get("email").(string),
		})
	})

	// Request with cookie, no Authorization header → should succeed.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "user-1", body["user_id"])
	assert.Equal(t, "test@example.com", body["email"])
}

func TestAuthMiddlewareBearerPrecedence(t *testing.T) {
	jwtSecret := "test-secret"

	// Generate two tokens with different user IDs.
	bearerUser := &platauth.User{ID: "bearer-user", Email: "bearer@example.com", Name: "Bearer"}
	bearerToken, err := platauth.GenerateToken(bearerUser, jwtSecret, 24*time.Hour)
	require.NoError(t, err)

	cookieUser := &platauth.User{ID: "cookie-user", Email: "cookie@example.com", Name: "Cookie"}
	cookieToken, err := platauth.GenerateToken(cookieUser, jwtSecret, 24*time.Hour)
	require.NoError(t, err)

	e := echo.New()
	e.Use(AuthMiddleware(jwtSecret, nil))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"user_id": c.Get("user_id").(string)})
	})

	// Request with both Bearer header and cookie → Bearer wins.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieToken})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "bearer-user", body["user_id"])
}

func TestAuthMiddlewareNeitherHeaderNorCookie(t *testing.T) {
	e := echo.New()
	e.Use(AuthMiddleware("test-secret", nil))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLogoutClearsCookies(t *testing.T) {
	cfg := DefaultConfig()

	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	token := generateTestToken(t, cfg.JWTSecret)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify Set-Cookie headers clear the session cookies.
	cookies := rec.Result().Cookies()
	var foundSession, foundRefresh bool
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			foundSession = true
			assert.Equal(t, -1, c.MaxAge, "session cookie should have MaxAge=-1")
			assert.True(t, c.HttpOnly, "session cookie should be HttpOnly")
		}
		if c.Name == refreshCookieName {
			foundRefresh = true
			assert.Equal(t, -1, c.MaxAge, "refresh cookie should have MaxAge=-1")
			assert.True(t, c.HttpOnly, "refresh cookie should be HttpOnly")
		}
	}
	assert.True(t, foundSession, "logout should set session cookie with MaxAge=-1")
	assert.True(t, foundRefresh, "logout should set refresh cookie with MaxAge=-1")
}
