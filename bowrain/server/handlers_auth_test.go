package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDeviceVerificationFormValues(t *testing.T) {
	// No OIDC configured → uses direct authorization with form values.
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Step 1: Start device auth to get a user_code.
	startForm := url.Values{"client_id": {"test-client"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start",
		strings.NewReader(startForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Parse user_code from response.
	body := rec.Body.String()
	require.Contains(t, body, "user_code")

	// Extract user_code and device_code.
	var userCode, deviceCode string
	deviceCodes.Lock()
	for dc, entry := range deviceCodes.entries {
		deviceCode = dc
		userCode = entry.UserCode
		break
	}
	deviceCodes.Unlock()
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
	deviceCodes.Lock()
	entry, ok := deviceCodes.entries[deviceCode]
	deviceCodes.Unlock()

	if ok {
		// Entry might have been consumed by poll, but if still there, check fields.
		assert.True(t, entry.Authorized)
		assert.Equal(t, "test@example.com", entry.UserEmail)
		assert.Equal(t, "Test User", entry.UserName)
	}
}

func TestHandleDeviceVerificationDefaultValues(t *testing.T) {
	// No OIDC configured, no email/name provided → defaults should be used.
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)

	// Manually create a device code entry.
	deviceCodes.Lock()
	deviceCodes.entries["test-device-default"] = &deviceCodeEntry{
		UserCode: "aaaa-bbbb",
	}
	deviceCodes.Unlock()

	e := echo.New()
	e.POST("/verify", func(c echo.Context) error {
		return srv.handleDeviceVerification(c, "aaaa-bbbb")
	})

	req := httptest.NewRequest(http.MethodPost, "/verify", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/device/authorized", rec.Header().Get("Location"))

	deviceCodes.Lock()
	entry := deviceCodes.entries["test-device-default"]
	deviceCodes.Unlock()

	require.NotNil(t, entry)
	assert.Equal(t, "user@bowrain.local", entry.UserEmail)
	assert.Equal(t, "Bowrain User", entry.UserName)

	// Clean up.
	deviceCodes.Lock()
	delete(deviceCodes.entries, "test-device-default")
	deviceCodes.Unlock()
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

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = mockOIDC.URL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)

	// Create a pending device code entry.
	deviceCodes.Lock()
	deviceCodes.entries["test-oidc-device"] = &deviceCodeEntry{
		UserCode:  "cccc-dddd",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		ClientID:  "kapi-cli",
	}
	deviceCodes.Unlock()
	defer func() {
		deviceCodes.Lock()
		delete(deviceCodes.entries, "test-oidc-device")
		deviceCodes.Unlock()
	}()

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

	deviceVerifyStates.Lock()
	entry, ok := deviceVerifyStates.entries[state]
	deviceVerifyStates.Unlock()
	require.True(t, ok, "state should be stored in deviceVerifyStates")
	assert.Equal(t, "test-oidc-device", entry.DeviceCode)
	assert.NotEmpty(t, entry.CodeVerifier, "entry should have CodeVerifier")
	assert.NotEmpty(t, entry.Nonce, "entry should have Nonce")

	// Clean up state.
	deviceVerifyStates.Lock()
	delete(deviceVerifyStates.entries, state)
	deviceVerifyStates.Unlock()
}

func TestHandleDeviceAuthCallbackMissingParams(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Missing code and state → should redirect to verify page with error.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/callback", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/device/verify?error=")
}

func TestHandleDeviceAuthCallbackInvalidState(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
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

func TestHandleDeviceAuthCallbackExpiredState(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Insert an expired state entry.
	deviceVerifyStates.Lock()
	deviceVerifyStates.entries["expired-device-state"] = &deviceVerifyEntry{
		DeviceCode: "some-device",
		ExpiresAt:  time.Now().Add(-time.Hour),
	}
	deviceVerifyStates.Unlock()

	params := url.Values{
		"code":  {"some-code"},
		"state": {"expired-device-state"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/callback?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/device/verify?error=")

	// State should have been consumed.
	deviceVerifyStates.Lock()
	_, exists := deviceVerifyStates.entries["expired-device-state"]
	deviceVerifyStates.Unlock()
	assert.False(t, exists)
}

func TestDeviceVerifyStatesCleanup(t *testing.T) {
	// Verify state entries are consumed after use (even on OIDC exchange failure).
	state := "test-device-cleanup"
	deviceVerifyStates.Lock()
	deviceVerifyStates.entries[state] = &deviceVerifyEntry{
		DeviceCode: "dev-code-123",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}
	deviceVerifyStates.Unlock()

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)

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
	deviceVerifyStates.Lock()
	_, exists := deviceVerifyStates.entries[state]
	deviceVerifyStates.Unlock()
	assert.False(t, exists, "state entry should be consumed after callback")
}

// --- Desktop auth (PKCE) tests ---

func TestHandleDesktopLoginMissingParams(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180" // needed to pass OIDC check
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// No redirect_uri or code_challenge → should get 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/login", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "redirect_uri and code_challenge required")
}

func TestHandleDesktopLoginNonLocalhostRedirect(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
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
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
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
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
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
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Missing code and state → should get 400 HTML error.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Authentication Failed")
}

func TestHandleDesktopCallbackInvalidState(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
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

func TestHandleDesktopCallbackExpiredState(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Insert an expired state entry.
	desktopAuthStates.Lock()
	desktopAuthStates.entries["expired-state"] = &desktopAuthEntry{
		RedirectURI:   "http://127.0.0.1:12345/callback",
		CodeChallenge: "test",
		ExpiresAt:     time.Now().Add(-time.Hour),
	}
	desktopAuthStates.Unlock()

	params := url.Values{
		"code":  {"some-code"},
		"state": {"expired-state"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop/callback?"+params.Encode(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid or Expired")

	// State should have been consumed.
	desktopAuthStates.Lock()
	_, exists := desktopAuthStates.entries["expired-state"]
	desktopAuthStates.Unlock()
	assert.False(t, exists)
}

func TestDesktopAuthStatesCleanup(t *testing.T) {
	// Verify that state entries are cleaned up after use.
	state := "test-cleanup-state"
	desktopAuthStates.Lock()
	desktopAuthStates.entries[state] = &desktopAuthEntry{
		RedirectURI:   "http://127.0.0.1:54321/callback",
		CodeChallenge: "challenge",
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	desktopAuthStates.Unlock()

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)

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
	desktopAuthStates.Lock()
	_, exists := desktopAuthStates.entries[state]
	desktopAuthStates.Unlock()
	assert.False(t, exists, "state entry should be consumed after callback")
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

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = mockOIDC.URL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
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

	// Verify state is stored in webAuthStates.
	webAuthStates.Lock()
	entry, ok := webAuthStates.entries[state]
	webAuthStates.Unlock()
	require.True(t, ok, "state should be stored in webAuthStates")
	assert.NotEmpty(t, entry.CodeVerifier)
	assert.NotEmpty(t, entry.Nonce)
	assert.False(t, entry.ExpiresAt.IsZero())

	// Step 2: Calling handleOIDCCodeExchange with the state should consume it
	// (will fail at OIDC exchange, but state should still be consumed).
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=fake-code&state="+state, nil)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	// State should have been consumed.
	webAuthStates.Lock()
	_, exists := webAuthStates.entries[state]
	webAuthStates.Unlock()
	assert.False(t, exists, "state entry should be consumed after callback")
}

func TestWebFlowCallbackWithoutState(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Callback with code but empty state → should get error HTML.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=fake-code&state=nonexistent", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid or Expired")
}

func TestWebFlowCallbackExpiredState(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = "http://localhost:8180"
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
	e := srv.GetEcho()

	// Insert an expired state entry.
	webAuthStates.Lock()
	webAuthStates.entries["expired-web-state"] = &webAuthEntry{
		CodeVerifier: "verifier",
		Nonce:        "nonce",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	webAuthStates.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?code=fake-code&state=expired-web-state", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Invalid or Expired")

	// State should have been consumed.
	webAuthStates.Lock()
	_, exists := webAuthStates.entries["expired-web-state"]
	webAuthStates.Unlock()
	assert.False(t, exists)
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

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = mockOIDC.URL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
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

		// Clean up the web auth state.
		parsed, _ := url.Parse(rec.Header().Get("Location"))
		state := parsed.Query().Get("state")
		webAuthStates.Lock()
		delete(webAuthStates.entries, state)
		webAuthStates.Unlock()
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

		// Clean up.
		parsed, _ := url.Parse(rec.Header().Get("Location"))
		state := parsed.Query().Get("state")
		desktopAuthStates.Lock()
		delete(desktopAuthStates.entries, state)
		desktopAuthStates.Unlock()
	})

	t.Run("device flow", func(t *testing.T) {
		// Set up a pending device code.
		deviceCodes.Lock()
		deviceCodes.entries["pkce-test-device"] = &deviceCodeEntry{
			UserCode:  "eeee-ffff",
			ExpiresAt: time.Now().Add(10 * time.Minute),
			ClientID:  "kapi-cli",
		}
		deviceCodes.Unlock()
		defer func() {
			deviceCodes.Lock()
			delete(deviceCodes.entries, "pkce-test-device")
			deviceCodes.Unlock()
		}()

		ep := echo.New()
		ep.POST("/verify", func(c echo.Context) error {
			return srv.handleDeviceVerification(c, "eeee-ffff")
		})

		req := httptest.NewRequest(http.MethodPost, "/verify", nil)
		rec := httptest.NewRecorder()
		ep.ServeHTTP(rec, req)
		require.Equal(t, http.StatusFound, rec.Code)
		assertPKCEAndNonce(t, rec.Header().Get("Location"))

		// Clean up.
		parsed, _ := url.Parse(rec.Header().Get("Location"))
		state := parsed.Query().Get("state")
		deviceVerifyStates.Lock()
		delete(deviceVerifyStates.entries, state)
		deviceVerifyStates.Unlock()
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

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = oidcURL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)
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
	desktopAuthStates.Lock()
	entry, ok := desktopAuthStates.entries[state]
	desktopAuthStates.Unlock()
	require.True(t, ok)
	assert.NotEmpty(t, entry.CodeVerifier, "desktop entry must have CodeVerifier")
	assert.NotEmpty(t, entry.Nonce, "desktop entry must have Nonce")

	// Clean up.
	desktopAuthStates.Lock()
	delete(desktopAuthStates.entries, state)
	desktopAuthStates.Unlock()
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

	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	cfg.OIDCIssuerURL = oidcURL
	cfg.OIDCClientID = "test-client"
	srv := NewServer(cfg)

	// Create a pending device code.
	deviceCodes.Lock()
	deviceCodes.entries["pkce-nonce-test-device"] = &deviceCodeEntry{
		UserCode:  "gggg-hhhh",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		ClientID:  "kapi-cli",
	}
	deviceCodes.Unlock()
	defer func() {
		deviceCodes.Lock()
		delete(deviceCodes.entries, "pkce-nonce-test-device")
		deviceCodes.Unlock()
	}()

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
	deviceVerifyStates.Lock()
	entry, ok := deviceVerifyStates.entries[state]
	deviceVerifyStates.Unlock()
	require.True(t, ok)
	assert.NotEmpty(t, entry.CodeVerifier, "device verify entry must have CodeVerifier")
	assert.NotEmpty(t, entry.Nonce, "device verify entry must have Nonce")

	// Clean up.
	deviceVerifyStates.Lock()
	delete(deviceVerifyStates.entries, state)
	deviceVerifyStates.Unlock()
}

func TestCleanupExpiredAuthStates(t *testing.T) {
	now := time.Now()

	// Insert expired entries in all 4 stores.
	deviceCodes.Lock()
	deviceCodes.entries["expired-dc"] = &deviceCodeEntry{ExpiresAt: now.Add(-time.Hour)}
	deviceCodes.entries["valid-dc"] = &deviceCodeEntry{ExpiresAt: now.Add(time.Hour)}
	deviceCodes.Unlock()

	webAuthStates.Lock()
	webAuthStates.entries["expired-web"] = &webAuthEntry{ExpiresAt: now.Add(-time.Hour)}
	webAuthStates.entries["valid-web"] = &webAuthEntry{ExpiresAt: now.Add(time.Hour)}
	webAuthStates.Unlock()

	desktopAuthStates.Lock()
	desktopAuthStates.entries["expired-desktop"] = &desktopAuthEntry{ExpiresAt: now.Add(-time.Hour)}
	desktopAuthStates.entries["valid-desktop"] = &desktopAuthEntry{ExpiresAt: now.Add(time.Hour)}
	desktopAuthStates.Unlock()

	deviceVerifyStates.Lock()
	deviceVerifyStates.entries["expired-device"] = &deviceVerifyEntry{ExpiresAt: now.Add(-time.Hour)}
	deviceVerifyStates.entries["valid-device"] = &deviceVerifyEntry{ExpiresAt: now.Add(time.Hour)}
	deviceVerifyStates.Unlock()

	// Trigger cleanup.
	cleanupExpiredAuthStates()

	// Verify expired entries are removed and valid entries remain.
	deviceCodes.Lock()
	_, expiredDC := deviceCodes.entries["expired-dc"]
	_, validDC := deviceCodes.entries["valid-dc"]
	deviceCodes.Unlock()
	assert.False(t, expiredDC, "expired device code should be removed")
	assert.True(t, validDC, "valid device code should remain")

	webAuthStates.Lock()
	_, expiredWeb := webAuthStates.entries["expired-web"]
	_, validWeb := webAuthStates.entries["valid-web"]
	webAuthStates.Unlock()
	assert.False(t, expiredWeb, "expired web state should be removed")
	assert.True(t, validWeb, "valid web state should remain")

	desktopAuthStates.Lock()
	_, expiredDesktop := desktopAuthStates.entries["expired-desktop"]
	_, validDesktop := desktopAuthStates.entries["valid-desktop"]
	desktopAuthStates.Unlock()
	assert.False(t, expiredDesktop, "expired desktop state should be removed")
	assert.True(t, validDesktop, "valid desktop state should remain")

	deviceVerifyStates.Lock()
	_, expiredDV := deviceVerifyStates.entries["expired-device"]
	_, validDV := deviceVerifyStates.entries["valid-device"]
	deviceVerifyStates.Unlock()
	assert.False(t, expiredDV, "expired device verify state should be removed")
	assert.True(t, validDV, "valid device verify state should remain")

	// Clean up valid entries.
	deviceCodes.Lock()
	delete(deviceCodes.entries, "valid-dc")
	deviceCodes.Unlock()
	webAuthStates.Lock()
	delete(webAuthStates.entries, "valid-web")
	webAuthStates.Unlock()
	desktopAuthStates.Lock()
	delete(desktopAuthStates.entries, "valid-desktop")
	desktopAuthStates.Unlock()
	deviceVerifyStates.Lock()
	delete(deviceVerifyStates.entries, "valid-device")
	deviceVerifyStates.Unlock()
}

// --- Cookie auth tests ---

// generateTestToken creates a signed JWT for testing.
func generateTestToken(t *testing.T, secret string) string {
	t.Helper()
	user := &auth.User{ID: "user-1", Email: "test@example.com", Name: "Test User"}
	token, err := auth.GenerateToken(user, secret, 24*time.Hour)
	require.NoError(t, err)
	return token
}

func TestAuthMiddlewareCookieFallback(t *testing.T) {
	jwtSecret := "test-secret"
	token := generateTestToken(t, jwtSecret)

	e := echo.New()
	e.Use(AuthMiddleware(jwtSecret))
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
	bearerUser := &auth.User{ID: "bearer-user", Email: "bearer@example.com", Name: "Bearer"}
	bearerToken, err := auth.GenerateToken(bearerUser, jwtSecret, 24*time.Hour)
	require.NoError(t, err)

	cookieUser := &auth.User{ID: "cookie-user", Email: "cookie@example.com", Name: "Cookie"}
	cookieToken, err := auth.GenerateToken(cookieUser, jwtSecret, 24*time.Hour)
	require.NoError(t, err)

	e := echo.New()
	e.Use(AuthMiddleware(jwtSecret))
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
	e.Use(AuthMiddleware("test-secret"))
	e.GET("/test", func(c echo.Context) error {
		return c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestLogoutClearsCookies(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.StorePath = t.TempDir() + "/test.db"
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
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
