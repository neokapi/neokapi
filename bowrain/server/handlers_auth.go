package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gokapi/gokapi/bowrain/auth"
	platformAuth "github.com/gokapi/gokapi/platform/auth"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
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

	// Generate and store a refresh token.
	refreshToken, err := platformAuth.GenerateRefreshToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "generate refresh token: " + err.Error()})
	}
	rtHash := sha256.Sum256([]byte(refreshToken))
	_, _ = s.AuthStore.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(rtHash[:]), time.Now().Add(30*24*time.Hour))

	// Clean up the device code.
	deviceCodes.Lock()
	delete(deviceCodes.entries, deviceCode)
	deviceCodes.Unlock()

	return c.JSON(http.StatusOK, auth.TokenResponse{
		AccessToken:  token,
		TokenType:    "Bearer",
		ExpiresIn:    86400,
		RefreshToken: refreshToken,
	})
}

// oidcContext returns a context configured for OIDC operations. When
// OIDCPublicURL differs from OIDCIssuerURL (typical in Docker), it sets up
// InsecureIssuerURLContext so the provider accepts the issuer mismatch,
// and injects a custom HTTP client that rewrites public-URL requests to
// the internal Docker hostname.
func (s *Server) oidcContext(ctx context.Context) context.Context {
	publicURL := s.Config.OIDCPublicURL
	if publicURL == "" || publicURL == s.Config.OIDCIssuerURL {
		return ctx
	}
	ctx = oidc.InsecureIssuerURLContext(ctx, publicURL)
	transport := &urlRewriteTransport{
		base: http.DefaultTransport,
		from: publicURL,
		to:   s.Config.OIDCIssuerURL,
	}
	return context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: transport})
}

// urlRewriteTransport rewrites request URLs so OIDC HTTP requests
// (discovery, JWKS) go to the Docker-internal OIDC hostname.
type urlRewriteTransport struct {
	base http.RoundTripper
	from string // public URL prefix (e.g. "http://localhost:8180/realms/bowrain")
	to   string // internal URL prefix (e.g. "http://keycloak:8080/realms/bowrain")
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqURL := req.URL.String()
	if strings.HasPrefix(reqURL, t.from) {
		newURL := t.to + reqURL[len(t.from):]
		u, err := url.Parse(newURL)
		if err != nil {
			return nil, err
		}
		req = req.Clone(req.Context())
		req.URL = u
		req.Host = u.Host
	}
	return t.base.RoundTrip(req)
}

// HandleAuthLogin initiates the OIDC authorization code flow by redirecting
// the browser to the OIDC authorization URL. After the user authenticates,
// they are redirected back to /api/v1/auth/callback with a code.
func (s *Server) HandleAuthLogin(c echo.Context) error {
	if s.Config.OIDCIssuerURL == "" || s.Config.OIDCClientID == "" {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "OIDC not configured"})
	}

	// Discover the authorization endpoint from the OIDC provider.
	oidcCtx := s.oidcContext(c.Request().Context())
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "OIDC discovery failed: " + err.Error()})
	}
	endpoint := provider.Endpoint()

	// If a public URL is configured (Docker), rewrite the discovered auth
	// endpoint to use the public URL so the browser can reach it.
	authURL := endpoint.AuthURL
	if s.Config.OIDCPublicURL != "" && s.Config.OIDCPublicURL != s.Config.OIDCIssuerURL {
		authURL = strings.Replace(authURL, s.Config.OIDCIssuerURL, s.Config.OIDCPublicURL, 1)
	}

	redirectURI := fmt.Sprintf("%s://%s/api/v1/auth/callback", c.Scheme(), c.Request().Host)
	state := randomHex(16)

	params := url.Values{
		"client_id":     {s.Config.OIDCClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"openid profile email"},
		"state":         {state},
	}

	return c.Redirect(http.StatusFound, authURL+"?"+params.Encode())
}

// HandleAuthCallback handles the OIDC redirect callback.
// After the user authenticates with the OIDC provider, they are redirected here.
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
<html><head><title>Bowrain - Authorize Device</title>
<style>body{font-family:system-ui;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#1a1a2e}
.card{background:#16213e;padding:40px;border-radius:12px;text-align:center;color:#e0e0e0;max-width:400px}
h1{color:#58a6ff;margin-bottom:8px}input{padding:12px;font-size:18px;border:2px solid #333;border-radius:8px;
background:#0f3460;color:#fff;text-align:center;letter-spacing:4px;width:200px;margin:16px 0}
button{padding:12px 32px;font-size:16px;background:#58a6ff;color:#fff;border:none;border-radius:8px;cursor:pointer;font-weight:600}
button:hover{background:#79b8ff}</style></head>
<body><div class="card"><h1>Bowrain</h1><p>Enter the code shown in your terminal:</p>
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

	// For now, authorize with a default user (in production, this would come from the OIDC session).
	// When an OIDC provider is configured, the user would already be authenticated via the session cookie.
	// Check both query params and form values to support programmatic e2e testing.
	email := c.QueryParam("email")
	if email == "" {
		email = c.FormValue("email")
	}
	name := c.QueryParam("name")
	if name == "" {
		name = c.FormValue("name")
	}
	if email == "" {
		email = "user@bowrain.local"
	}
	if name == "" {
		name = "Bowrain User"
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

// handleOIDCCodeExchange performs the OAuth2 authorization code exchange.
// It uses OIDC discovery to resolve the token endpoint, making it compatible
// with any OIDC provider (Keycloak, Dex, etc.).
func (s *Server) handleOIDCCodeExchange(c echo.Context, code string) error {
	ctx := c.Request().Context()

	if s.Config.OIDCIssuerURL == "" || s.Config.OIDCClientID == "" {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "OIDC not configured"})
	}

	redirectURL := fmt.Sprintf("%s://%s/api/v1/auth/callback", c.Scheme(), c.Request().Host)

	// Use oidcContext to handle Docker URL mismatches (InsecureIssuerURL +
	// HTTP client that rewrites public→internal URLs for JWKS fetching).
	oidcCtx := s.oidcContext(ctx)

	// Discover endpoints from the OIDC provider.
	oauth2Cfg, err := auth.NewOAuth2Config(oidcCtx, auth.OIDCConfig{
		IssuerURL:    s.Config.OIDCIssuerURL,
		ClientID:     s.Config.OIDCClientID,
		ClientSecret: s.Config.OIDCClientSecret,
		RedirectURL:  redirectURL,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "OIDC discovery failed: " + err.Error()})
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

	verifier, err := auth.NewOIDCVerifier(oidcCtx, s.Config.OIDCIssuerURL, s.Config.OIDCClientID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create verifier: " + err.Error()})
	}

	idToken, err := verifier.Verify(oidcCtx, rawIDToken)
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

	// Generate and store a refresh token.
	refreshToken, rtErr := platformAuth.GenerateRefreshToken()
	if rtErr == nil {
		rtHash := sha256.Sum256([]byte(refreshToken))
		_, _ = s.AuthStore.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(rtHash[:]), time.Now().Add(30*24*time.Hour))
	}

	// Redirect to frontend with token.
	frontendURL := "/?token=" + token + "&user=" + user.Email
	if refreshToken != "" {
		frontendURL += "&refresh_token=" + refreshToken
	}
	return c.Redirect(http.StatusFound, frontendURL)
}

// RefreshRequest is the request body for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// HandleTokenRefresh exchanges a valid refresh token for a new access token
// and a rotated refresh token. The old refresh token is consumed (single-use).
func (s *Server) HandleTokenRefresh(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}

	var req RefreshRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request"})
	}
	if req.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "refresh_token required"})
	}

	// Hash the incoming token for lookup.
	hash := sha256.Sum256([]byte(req.RefreshToken))
	tokenHash := hex.EncodeToString(hash[:])

	ctx := c.Request().Context()
	userID, err := s.AuthStore.ValidateRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid or expired refresh token"})
	}

	// Get user info for the new JWT.
	user, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "user not found"})
	}

	// Generate new access token.
	accessToken, err := s.Services.Auth.GenerateToken(user, 24*time.Hour)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to generate token"})
	}

	// Generate new refresh token (rotation).
	newRefreshToken, err := platformAuth.GenerateRefreshToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to generate refresh token"})
	}
	newHash := sha256.Sum256([]byte(newRefreshToken))
	if _, err = s.AuthStore.StoreRefreshToken(ctx, userID, hex.EncodeToString(newHash[:]), time.Now().Add(30*24*time.Hour)); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to store refresh token"})
	}

	return c.JSON(http.StatusOK, auth.TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    86400,
		RefreshToken: newRefreshToken,
	})
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
