package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDeviceVerificationFormValues(t *testing.T) {
	// Create a server with auth configured so the device flow routes are set up.
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
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "Device authorized")

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
	// When no email/name provided, defaults should be used.
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

	assert.Equal(t, http.StatusOK, rec.Code)

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
	assert.Contains(t, rec.Body.String(), "redirect_uri must be http://127.0.0.1")
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
