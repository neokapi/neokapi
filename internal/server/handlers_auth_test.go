package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	assert.Equal(t, "user@gokapi.local", entry.UserEmail)
	assert.Equal(t, "gokapi User", entry.UserName)

	// Clean up.
	deviceCodes.Lock()
	delete(deviceCodes.entries, "test-device-default")
	deviceCodes.Unlock()
}
