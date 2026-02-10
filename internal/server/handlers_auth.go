package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gokapi/gokapi/core/auth"
	"github.com/labstack/echo/v4"
)

// deviceCodeEntry stores the state of a pending device authorization.
type deviceCodeEntry struct {
	UserCode   string
	ExpiresAt  time.Time
	Interval   int
	ClientID   string
	Authorized bool // set to true when user approves via callback
	UserEmail  string
	UserName   string
}

// deviceCodeStore is an in-memory store for pending device codes.
// In production this would be backed by Redis or the database.
var deviceCodes = struct {
	sync.Mutex
	entries map[string]*deviceCodeEntry
}{entries: make(map[string]*deviceCodeEntry)}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomUserCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	code := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", code[:4], code[4:])
}

// HandleDeviceAuthStart starts the device authorization flow (RFC 8628).
// The client receives a device_code and user_code. The user opens the
// verification_uri in a browser and enters the user_code to authorize.
func (s *Server) HandleDeviceAuthStart(c echo.Context) error {
	clientID := c.FormValue("client_id")
	if clientID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "client_id required"})
	}

	deviceCode := randomHex(16)
	userCode := randomUserCode()

	deviceCodes.Lock()
	deviceCodes.entries[deviceCode] = &deviceCodeEntry{
		UserCode:  userCode,
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Interval:  5,
		ClientID:  clientID,
	}
	deviceCodes.Unlock()

	baseURL := fmt.Sprintf("%s://%s", c.Scheme(), c.Request().Host)

	return c.JSON(http.StatusOK, auth.DeviceAuthResponse{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURI: baseURL + "/api/v1/auth/device/verify",
		ExpiresIn:       600,
		Interval:        5,
	})
}

// HandleDeviceAuthPoll is called by the CLI to poll for a token.
// Returns authorization_pending until the user authorizes via the callback.
func (s *Server) HandleDeviceAuthPoll(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	deviceCode := c.FormValue("device_code")
	if deviceCode == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "device_code required"})
	}

	deviceCodes.Lock()
	entry, ok := deviceCodes.entries[deviceCode]
	deviceCodes.Unlock()

	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
	}

	if time.Now().After(entry.ExpiresAt) {
		deviceCodes.Lock()
		delete(deviceCodes.entries, deviceCode)
		deviceCodes.Unlock()
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "expired_token"})
	}

	if !entry.Authorized {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
	}

	// User authorized — create or retrieve user and generate token.
	ctx := c.Request().Context()
	user, err := s.Services.Auth.GetOrCreateUser(ctx, entry.UserEmail, entry.UserName, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create user: " + err.Error()})
	}

	token, err := s.Services.Auth.GenerateToken(user, 24*time.Hour)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "generate token: " + err.Error()})
	}

	// Clean up the device code.
	deviceCodes.Lock()
	delete(deviceCodes.entries, deviceCode)
	deviceCodes.Unlock()

	return c.JSON(http.StatusOK, auth.TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400,
	})
}

// HandleAuthCallback handles the OIDC redirect callback from Dex.
// After the user authenticates with Dex, they are redirected here.
// For the device flow, this also verifies the user_code and authorizes the pending device.
func (s *Server) HandleAuthCallback(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	// If this is a device verification request (GET with user_code param or form POST)
	userCode := c.QueryParam("user_code")
	if userCode == "" {
		userCode = c.FormValue("user_code")
	}

	// For browser-based OIDC callback (authorization code flow)
	code := c.QueryParam("code")

	if userCode != "" {
		return s.handleDeviceVerification(c, userCode)
	}

	if code != "" {
		return s.handleOIDCCodeExchange(c, code)
	}

	// Show device verification form
	return c.HTML(http.StatusOK, `<!DOCTYPE html>
<html><head><title>gokapi - Authorize Device</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#1a1a2e}
.card{background:#16213e;padding:40px;border-radius:12px;text-align:center;color:#e0e0e0;max-width:400px}
h1{color:#58a6ff;margin-bottom:8px}input{padding:12px;font-size:18px;border:2px solid #333;border-radius:8px;
background:#0f3460;color:#fff;text-align:center;letter-spacing:4px;width:200px;margin:16px 0}
button{padding:12px 32px;font-size:16px;background:#58a6ff;color:#fff;border:none;border-radius:8px;cursor:pointer;font-weight:600}
button:hover{background:#79b8ff}</style></head>
<body><div class="card"><h1>gokapi</h1><p>Enter the code shown in your terminal:</p>
<form method="POST" action="/api/v1/auth/device/verify"><input name="user_code" placeholder="xxxx-xxxx" required autofocus>
<br><button type="submit">Authorize</button></form></div></body></html>`)
}

// handleDeviceVerification matches a user_code to a pending device and authorizes it.
func (s *Server) handleDeviceVerification(c echo.Context, userCode string) error {
	// Find the matching device code entry.
	deviceCodes.Lock()
	var matchedCode string
	var matchedEntry *deviceCodeEntry
	for code, entry := range deviceCodes.entries {
		if entry.UserCode == userCode && !entry.Authorized {
			matchedCode = code
			matchedEntry = entry
			break
		}
	}
	deviceCodes.Unlock()

	if matchedEntry == nil {
		return c.HTML(http.StatusBadRequest, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1>Invalid or expired code</h1><p>Please check the code and try again.</p></body></html>`)
	}

	// For now, authorize with a default user (in production, this would come from the Dex OIDC session).
	// When Dex is configured, the user would already be authenticated via the OIDC session cookie.
	email := c.QueryParam("email")
	name := c.QueryParam("name")
	if email == "" {
		email = "user@gokapi.local"
	}
	if name == "" {
		name = "gokapi User"
	}

	deviceCodes.Lock()
	matchedEntry.Authorized = true
	matchedEntry.UserEmail = email
	matchedEntry.UserName = name
	deviceCodes.entries[matchedCode] = matchedEntry
	deviceCodes.Unlock()

	return c.HTML(http.StatusOK, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1 style="color:#58a6ff">Device authorized!</h1><p>You can close this window and return to your terminal.</p></body></html>`)
}

// handleOIDCCodeExchange performs the OAuth2 authorization code exchange with Dex.
func (s *Server) handleOIDCCodeExchange(c echo.Context, code string) error {
	ctx := c.Request().Context()

	if s.Config.DexIssuerURL == "" || s.Config.DexClientID == "" {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "OIDC not configured"})
	}

	oidcCfg := auth.OIDCConfig{
		IssuerURL:    s.Config.DexIssuerURL,
		ClientID:     s.Config.DexClientID,
		ClientSecret: s.Config.DexClientSecret,
		RedirectURL:  fmt.Sprintf("%s://%s/api/v1/auth/callback", c.Scheme(), c.Request().Host),
	}

	oauth2Cfg, err := auth.NewOAuth2Config(ctx, oidcCfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "OIDC config: " + err.Error()})
	}

	oauth2Token, err := oauth2Cfg.Exchange(ctx, code)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "code exchange: " + err.Error()})
	}

	// Verify the ID token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "no id_token in response"})
	}

	verifier, err := auth.NewOIDCVerifier(ctx, s.Config.DexIssuerURL, s.Config.DexClientID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create verifier: " + err.Error()})
	}

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "verify id_token: " + err.Error()})
	}

	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "extract claims: " + err.Error()})
	}

	user, err := s.Services.Auth.GetOrCreateUser(ctx, claims.Email, claims.Name, "")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create user: " + err.Error()})
	}

	token, err := s.Services.Auth.GenerateToken(user, 24*time.Hour)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "generate token: " + err.Error()})
	}

	// Redirect to frontend with token.
	return c.Redirect(http.StatusFound, "/?token="+token+"&user="+user.Email)
}

// HandleAuthMe returns the current authenticated user.
func (s *Server) HandleAuthMe(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}

	ctx := c.Request().Context()
	user, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "user not found"})
	}

	return c.JSON(http.StatusOK, user)
}

// HandleAuthLogout invalidates the current session.
// Since JWTs are stateless, this is a no-op on the server side.
// The client is expected to discard the token.
func (s *Server) HandleAuthLogout(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "logged out"})
}
