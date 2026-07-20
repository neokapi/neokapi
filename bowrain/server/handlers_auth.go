package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platformAuth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"golang.org/x/oauth2"
)

// deviceCodeEntry stores the state of a pending device authorization.
type deviceCodeEntry struct {
	UserCode   string `json:"user_code"`
	Interval   int    `json:"interval"`
	ClientID   string `json:"client_id"`
	Authorized bool   `json:"authorized"` // set to true when user approves via callback
	UserEmail  string `json:"user_email"`
	UserName   string `json:"user_name"`
	OIDCSub    string `json:"oidc_sub"` // OIDC subject identifier (Keycloak UUID)
}

// webAuthEntry stores the state of a pending web OIDC authorization.
type webAuthEntry struct {
	CodeVerifier string `json:"code_verifier"`
	Nonce        string `json:"nonce"`
}

// desktopAuthEntry stores the state of a pending desktop PKCE authorization.
type desktopAuthEntry struct {
	RedirectURI   string `json:"redirect_uri"`   // desktop's localhost callback URL
	CodeChallenge string `json:"code_challenge"` // PKCE code_challenge from the desktop
	CodeVerifier  string `json:"code_verifier"`  // server-side PKCE verifier for OIDC exchange
	Nonce         string `json:"nonce"`          // OIDC nonce for ID token replay protection
}

// deviceVerifyEntry maps an OIDC state to a pending device code during
// the device verification flow (user authenticates via OIDC to authorize the device).
type deviceVerifyEntry struct {
	DeviceCode   string `json:"device_code"`
	CodeVerifier string `json:"code_verifier"` // server-side PKCE verifier for OIDC exchange
	Nonce        string `json:"nonce"`         // OIDC nonce for ID token replay protection
}

func randomHex(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read never returns an error as of Go 1.24.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomUserCode() string {
	b := make([]byte, 4)
	// crypto/rand.Read never returns an error as of Go 1.24.
	_, _ = rand.Read(b)
	code := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", code[:4], code[4:])
}

// oidcAuthParams holds the PKCE verifier+challenge, nonce, and state generated
// for a single OIDC authorization request.
type oidcAuthParams struct {
	State, CodeVerifier, Challenge, Nonce string
}

// newOIDCAuthParams generates fresh PKCE, nonce, and state values for an
// OIDC authorization redirect.
func newOIDCAuthParams() (*oidcAuthParams, error) {
	verifier, err := platformAuth.GenerateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	return &oidcAuthParams{
		State:        randomHex(16),
		CodeVerifier: verifier,
		Challenge:    platformAuth.ComputeCodeChallenge(verifier),
		Nonce:        randomHex(16),
	}, nil
}

// authStateTTL is the TTL for ephemeral auth states (device codes, OIDC states).
const authStateTTL = 10 * time.Minute

// HandleDeviceAuthStart starts the device authorization flow (RFC 8628).
// The client receives a device_code and user_code. The user opens the
// verification_uri in a browser and enters the user_code to authorize.
func (s *Server) HandleDeviceAuthStart(c echo.Context) error {
	clientID := c.FormValue("client_id")
	if clientID == "" {
		return apiErr(c, http.StatusBadRequest, "client_id required")
	}

	ctx := c.Request().Context()
	deviceCode := randomHex(16)
	userCode := randomUserCode()

	entry := &deviceCodeEntry{
		UserCode: userCode,
		Interval: 5,
		ClientID: clientID,
	}
	if err := sessionSet(ctx, s.SessionStore, prefixDeviceCode, deviceCode, entry, authStateTTL); err != nil {
		return apiErr(c, http.StatusInternalServerError, "store device code: "+err.Error())
	}

	// Store secondary index: userCode → deviceCode for lookup during verification.
	if err := s.SessionStore.Set(ctx, prefixUserCode+userCode, []byte(deviceCode), authStateTTL); err != nil {
		return apiErr(c, http.StatusInternalServerError, "store user code index: "+err.Error())
	}

	baseURL := requestBaseURL(c)

	return c.JSON(http.StatusOK, platformAuth.DeviceAuthResponse{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURI: baseURL + "/device/verify",
		ExpiresIn:       600,
		Interval:        5,
	})
}

// HandleDeviceAuthPoll is called by the CLI to poll for a token.
// Returns authorization_pending until the user authorizes via the callback.
func (s *Server) HandleDeviceAuthPoll(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	deviceCode := c.FormValue("device_code")
	if deviceCode == "" {
		return apiErr(c, http.StatusBadRequest, "device_code required")
	}

	ctx := c.Request().Context()
	entry, err := sessionGet[deviceCodeEntry](ctx, s.SessionStore, prefixDeviceCode, deviceCode)
	if errors.Is(err, ErrSessionNotFound) {
		return apiErr(c, http.StatusBadRequest, "invalid_grant")
	}
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "lookup device code: "+err.Error())
	}

	if !entry.Authorized {
		return apiErr(c, http.StatusBadRequest, "authorization_pending")
	}

	// User authorized — create or retrieve user and generate token.
	user, err := s.Services.Auth.GetOrCreateUser(ctx, entry.UserEmail, entry.UserName, "", entry.OIDCSub, requestLocale(c))
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "create user: "+err.Error())
	}
	s.trackUserLogin(user.ID, user.Email, user.CreatedAt)
	s.emitAuthEvent(c, platev.EventAuthLogin, user.ID, user.Name, "oidc")

	token, err := s.Services.Auth.GenerateToken(user, 15*time.Minute)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "generate token: "+err.Error())
	}

	// Generate and store a refresh token.
	refreshToken, err := platformAuth.GenerateRefreshToken()
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "generate refresh token: "+err.Error())
	}
	rtHash := sha256.Sum256([]byte(refreshToken))
	if _, err := s.AuthStore.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(rtHash[:]), time.Now().Add(30*24*time.Hour)); err != nil {
		// Never hand the client a refresh token that was not persisted —
		// it would be unredeemable (silent forced re-login).
		return apiErr(c, http.StatusInternalServerError, "failed to store refresh token")
	}

	// Clean up the device code and its user code index.
	_ = sessionDelete(ctx, s.SessionStore, prefixDeviceCode, deviceCode)
	_ = s.SessionStore.Delete(ctx, prefixUserCode+entry.UserCode)

	return c.JSON(http.StatusOK, platformAuth.TokenResponse{
		AccessToken:  token,
		TokenType:    "Bearer",
		ExpiresIn:    900,
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
		return apiErr(c, http.StatusServiceUnavailable, "OIDC not configured")
	}

	// Discover the authorization endpoint from the OIDC provider.
	oidcCtx := s.oidcContext(c.Request().Context())
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}
	endpoint := provider.Endpoint()

	// If a public URL is configured (Docker), rewrite the discovered auth
	// endpoint to use the public URL so the browser can reach it.
	authURL := endpoint.AuthURL
	if s.Config.OIDCPublicURL != "" && s.Config.OIDCPublicURL != s.Config.OIDCIssuerURL {
		authURL = strings.Replace(authURL, s.Config.OIDCIssuerURL, s.Config.OIDCPublicURL, 1)
	}

	redirectURI := requestBaseURL(c) + "/api/v1/auth/callback"

	ap, err := newOIDCAuthParams()
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}

	// Store state → web auth entry for validation in the callback.
	ctx := c.Request().Context()
	webEntry := &webAuthEntry{
		CodeVerifier: ap.CodeVerifier,
		Nonce:        ap.Nonce,
	}
	if err := sessionSet(ctx, s.SessionStore, prefixWebAuth, ap.State, webEntry, authStateTTL); err != nil {
		return apiErr(c, http.StatusInternalServerError, "store auth state: "+err.Error())
	}

	params := url.Values{
		"client_id":             {s.Config.OIDCClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile email"},
		"state":                 {ap.State},
		"code_challenge":        {ap.Challenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {ap.Nonce},
	}

	return c.Redirect(http.StatusFound, authURL+"?"+params.Encode())
}

// HandleAuthCallback handles the OIDC redirect callback.
// After the user authenticates with the OIDC provider, they are redirected here.
// For the device flow, this also verifies the user_code and authorizes the pending device.
func (s *Server) HandleAuthCallback(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	// If this is a device verification request (GET with user_code param or form POST)
	userCode := c.QueryParam("user_code")
	if userCode == "" {
		userCode = c.FormValue("user_code")
	}

	// For browser-based OIDC callback (authorization code flow)
	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if userCode != "" {
		return s.handleDeviceVerification(c, userCode)
	}

	if code != "" {
		return s.handleOIDCCodeExchange(c, code, state)
	}

	return apiErr(c, http.StatusBadRequest, "missing code, state, or user_code parameter")
}

// HandleDesktopLogin initiates the authorization code + PKCE flow for the
// desktop app. The desktop provides a localhost redirect_uri and a PKCE
// code_challenge. We store the state and redirect the browser to the OIDC
// provider's authorization endpoint.
func (s *Server) HandleDesktopLogin(c echo.Context) error {
	if s.Config.OIDCIssuerURL == "" || s.Config.OIDCClientID == "" {
		return apiErr(c, http.StatusServiceUnavailable, "OIDC not configured")
	}

	redirectURI := c.QueryParam("redirect_uri")
	codeChallenge := c.QueryParam("code_challenge")
	challengeMethod := c.QueryParam("code_challenge_method")

	if redirectURI == "" || codeChallenge == "" {
		return apiErr(c, http.StatusBadRequest, "redirect_uri and code_challenge required")
	}

	// Security: only allow localhost or bowrain:// redirect URIs.
	parsedURI, err := url.Parse(redirectURI)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "invalid redirect_uri")
	}
	isLocalhost := parsedURI.Hostname() == "127.0.0.1" || parsedURI.Hostname() == "localhost"
	isCustomScheme := parsedURI.Scheme == "bowrain"
	if !isLocalhost && !isCustomScheme {
		return apiErr(c, http.StatusBadRequest, "redirect_uri must be http://127.0.0.1:... or bowrain://...")
	}

	if challengeMethod != "" && challengeMethod != "S256" {
		return apiErr(c, http.StatusBadRequest, "only S256 code_challenge_method is supported")
	}

	// Discover the authorization endpoint from the OIDC provider.
	oidcCtx := s.oidcContext(c.Request().Context())
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}
	endpoint := provider.Endpoint()

	// If a public URL is configured (Docker), rewrite the auth endpoint.
	authURL := endpoint.AuthURL
	if s.Config.OIDCPublicURL != "" && s.Config.OIDCPublicURL != s.Config.OIDCIssuerURL {
		authURL = strings.Replace(authURL, s.Config.OIDCIssuerURL, s.Config.OIDCPublicURL, 1)
	}

	// The server's own callback URL — the OIDC provider redirects here.
	serverCallbackURI := requestBaseURL(c) + "/api/v1/auth/desktop/callback"

	ap, err := newOIDCAuthParams()
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}

	// Store the state mapping.
	ctx := c.Request().Context()
	desktopEntry := &desktopAuthEntry{
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		CodeVerifier:  ap.CodeVerifier,
		Nonce:         ap.Nonce,
	}
	if err := sessionSet(ctx, s.SessionStore, prefixDesktopAuth, ap.State, desktopEntry, authStateTTL); err != nil {
		return apiErr(c, http.StatusInternalServerError, "store auth state: "+err.Error())
	}

	params := url.Values{
		"client_id":             {s.Config.OIDCClientID},
		"redirect_uri":          {serverCallbackURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile email"},
		"state":                 {ap.State},
		"code_challenge":        {ap.Challenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {ap.Nonce},
	}

	return c.Redirect(http.StatusFound, authURL+"?"+params.Encode())
}

// HandleDesktopCallback handles the OIDC provider's redirect after the user
// authenticates. It exchanges the authorization code for tokens, creates/gets
// the user, generates a Bowrain JWT + refresh token, and redirects to the
// desktop app's localhost callback URI with the tokens as query parameters.
func (s *Server) HandleDesktopCallback(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if code == "" || state == "" {
		errMsg := c.QueryParam("error_description")
		if errMsg == "" {
			errMsg = c.QueryParam("error")
		}
		if errMsg == "" {
			errMsg = "missing code or state"
		}
		return c.HTML(http.StatusBadRequest, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1>Authentication Failed</h1><p>`+errMsg+`</p></body></html>`)
	}

	// Look up and consume the pending state.
	ctx := c.Request().Context()
	entry, err := sessionGet[desktopAuthEntry](ctx, s.SessionStore, prefixDesktopAuth, state)
	if err != nil {
		return c.HTML(http.StatusBadRequest, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1>Invalid or Expired Session</h1><p>Please try signing in again from the desktop app.</p></body></html>`)
	}
	_ = sessionDelete(ctx, s.SessionStore, prefixDesktopAuth, state)

	serverCallbackURI := requestBaseURL(c) + "/api/v1/auth/desktop/callback"

	// Exchange the authorization code with the OIDC provider.
	oidcCtx := s.oidcContext(ctx)
	oauth2Cfg, err := auth.NewOAuth2Config(oidcCtx, auth.OIDCConfig{
		IssuerURL:    s.Config.OIDCIssuerURL,
		ClientID:     s.Config.OIDCClientID,
		ClientSecret: s.Config.OIDCClientSecret,
		RedirectURL:  serverCallbackURI,
	})
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}

	oauth2Token, err := oauth2Cfg.Exchange(oidcCtx, code, oauth2.VerifierOption(entry.CodeVerifier))
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "code exchange: "+err.Error())
	}

	// Verify the ID token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return apiErr(c, http.StatusInternalServerError, "no id_token in response")
	}

	verifier, err := auth.NewOIDCVerifier(oidcCtx, s.Config.OIDCIssuerURL, s.Config.OIDCClientID)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "create verifier: "+err.Error())
	}

	idToken, err := verifier.Verify(oidcCtx, rawIDToken)
	if err != nil {
		return apiErr(c, http.StatusUnauthorized, "verify id_token: "+err.Error())
	}

	// Verify nonce to prevent ID token replay.
	if idToken.Nonce != entry.Nonce {
		return apiErr(c, http.StatusUnauthorized, "nonce mismatch")
	}

	claims, err := identityFromToken(idToken, !s.Config.AllowUnverifiedEmail)
	if err != nil {
		if errors.Is(err, errEmailNotVerified) {
			return apiErr(c, http.StatusForbidden, "email address is not verified")
		}
		return apiErr(c, http.StatusInternalServerError, "extract claims: "+err.Error())
	}

	user, err := s.Services.Auth.GetOrCreateUser(ctx, claims.Email, claims.Name, "", idToken.Subject, requestLocale(c))
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "create user: "+err.Error())
	}
	s.trackUserLogin(user.ID, user.Email, user.CreatedAt)
	s.emitAuthEvent(c, platev.EventAuthLogin, user.ID, user.Name, "oidc")

	token, err := s.Services.Auth.GenerateToken(user, 15*time.Minute)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "generate token: "+err.Error())
	}

	// Generate and store a refresh token.
	refreshToken, rtErr := platformAuth.GenerateRefreshToken()
	if rtErr != nil {
		return apiErr(c, http.StatusInternalServerError, "generate refresh token: "+rtErr.Error())
	}
	rtHash := sha256.Sum256([]byte(refreshToken))
	if _, err := s.AuthStore.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(rtHash[:]), time.Now().Add(30*24*time.Hour)); err != nil {
		// Never hand the client a refresh token that was not persisted —
		// it would be unredeemable (silent forced re-login).
		return apiErr(c, http.StatusInternalServerError, "failed to store refresh token")
	}

	// Redirect to the desktop app's localhost callback with tokens.
	desktopRedirect, _ := url.Parse(entry.RedirectURI)
	q := desktopRedirect.Query()
	q.Set("token", token)
	if refreshToken != "" {
		q.Set("refresh_token", refreshToken)
	}
	q.Set("user", claims.Email)
	q.Set("name", claims.Name)
	desktopRedirect.RawQuery = q.Encode()

	return c.Redirect(http.StatusFound, desktopRedirect.String())
}

// handleDeviceVerification matches a user_code to a pending device and either
// redirects the browser through OIDC (when configured) or falls back to direct
// authorization (for local dev / testing without an OIDC provider).
//
// When the request includes explicit email (and optional name) form values,
// direct authorization is used regardless of OIDC configuration. This allows
// programmatic clients (E2E tests, CLI helpers) to complete the device flow
// without driving a full browser-based OIDC login.
func (s *Server) handleDeviceVerification(c echo.Context, userCode string) error {
	// Find the matching device code via the secondary userCode → deviceCode index.
	ctx := c.Request().Context()
	deviceCodeBytes, err := s.SessionStore.Get(ctx, prefixUserCode+userCode)
	if errors.Is(err, ErrSessionNotFound) {
		return c.Redirect(http.StatusFound, "/device/verify?error="+url.QueryEscape("Invalid or expired code. Please check and try again."))
	}
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "lookup user code: "+err.Error())
	}
	matchedCode := string(deviceCodeBytes)

	// Verify the device code entry exists and is not already authorized.
	entry, err := sessionGet[deviceCodeEntry](ctx, s.SessionStore, prefixDeviceCode, matchedCode)
	if err != nil || entry.Authorized {
		return c.Redirect(http.StatusFound, "/device/verify?error="+url.QueryEscape("Invalid or expired code. Please check and try again."))
	}

	// If explicit email is provided (programmatic/test request), use direct
	// verification — the caller already knows the user identity.
	if email := c.FormValue("email"); email != "" {
		return s.handleDeviceVerificationDirect(c, matchedCode)
	}

	// If OIDC is configured, redirect through the identity provider.
	if s.Config.OIDCIssuerURL != "" && s.Config.OIDCClientID != "" {
		return s.handleDeviceVerificationOIDC(c, matchedCode)
	}

	// No OIDC configured — fall back to direct authorization (local dev / tests).
	return s.handleDeviceVerificationDirect(c, matchedCode)
}

// handleDeviceVerificationOIDC redirects the browser to the OIDC provider for
// authentication. After the user authenticates, the provider redirects to
// /api/v1/auth/device/callback, which completes the device authorization.
func (s *Server) handleDeviceVerificationOIDC(c echo.Context, deviceCode string) error {
	oidcCtx := s.oidcContext(c.Request().Context())
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}
	endpoint := provider.Endpoint()

	// If a public URL is configured (Docker), rewrite the auth endpoint.
	authURL := endpoint.AuthURL
	if s.Config.OIDCPublicURL != "" && s.Config.OIDCPublicURL != s.Config.OIDCIssuerURL {
		authURL = strings.Replace(authURL, s.Config.OIDCIssuerURL, s.Config.OIDCPublicURL, 1)
	}

	callbackURI := requestBaseURL(c) + "/api/v1/auth/device/callback"

	ap, err := newOIDCAuthParams()
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, err.Error())
	}

	// Store the state → device_code mapping.
	ctx := c.Request().Context()
	verifyEntry := &deviceVerifyEntry{
		DeviceCode:   deviceCode,
		CodeVerifier: ap.CodeVerifier,
		Nonce:        ap.Nonce,
	}
	if err := sessionSet(ctx, s.SessionStore, prefixDeviceVerify, ap.State, verifyEntry, authStateTTL); err != nil {
		return apiErr(c, http.StatusInternalServerError, "store verify state: "+err.Error())
	}

	params := url.Values{
		"client_id":             {s.Config.OIDCClientID},
		"redirect_uri":          {callbackURI},
		"response_type":         {"code"},
		"scope":                 {"openid profile email"},
		"state":                 {ap.State},
		"code_challenge":        {ap.Challenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {ap.Nonce},
	}

	return c.Redirect(http.StatusFound, authURL+"?"+params.Encode())
}

// handleDeviceVerificationDirect authorizes the device without OIDC, using
// form values or defaults. Used when no OIDC provider is configured
// (local development, CI tests).
func (s *Server) handleDeviceVerificationDirect(c echo.Context, deviceCode string) error {
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

	ctx := c.Request().Context()
	entry, err := sessionGet[deviceCodeEntry](ctx, s.SessionStore, prefixDeviceCode, deviceCode)
	if err != nil {
		return c.Redirect(http.StatusFound, "/device/verify?error="+url.QueryEscape("Invalid or expired code. Please check and try again."))
	}

	entry.Authorized = true
	entry.UserEmail = email
	entry.UserName = name

	// Re-store the updated entry (preserves remaining TTL in Redis).
	if err := sessionSet(ctx, s.SessionStore, prefixDeviceCode, deviceCode, entry, authStateTTL); err != nil {
		return apiErr(c, http.StatusInternalServerError, "update device code: "+err.Error())
	}

	return c.Redirect(http.StatusFound, "/device/authorized")
}

// HandleDeviceAuthCallback handles the OIDC redirect after the user authenticated
// to authorize a pending device code. It exchanges the authorization code,
// verifies the ID token, extracts claims, and marks the device as authorized.
func (s *Server) HandleDeviceAuthCallback(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if code == "" || state == "" {
		errMsg := c.QueryParam("error_description")
		if errMsg == "" {
			errMsg = c.QueryParam("error")
		}
		if errMsg == "" {
			errMsg = "Authentication failed."
		}
		return c.Redirect(http.StatusFound, "/device/verify?error="+url.QueryEscape(errMsg))
	}

	// Look up and consume the pending state → device_code mapping.
	ctx := c.Request().Context()
	verifyEntry, err := sessionGet[deviceVerifyEntry](ctx, s.SessionStore, prefixDeviceVerify, state)
	if err != nil {
		return c.Redirect(http.StatusFound, "/device/verify?error="+url.QueryEscape("Session expired. Please try again."))
	}
	_ = sessionDelete(ctx, s.SessionStore, prefixDeviceVerify, state)

	callbackURI := requestBaseURL(c) + "/api/v1/auth/device/callback"

	// Exchange the authorization code with the OIDC provider.
	oidcCtx := s.oidcContext(ctx)
	oauth2Cfg, err := auth.NewOAuth2Config(oidcCtx, auth.OIDCConfig{
		IssuerURL:    s.Config.OIDCIssuerURL,
		ClientID:     s.Config.OIDCClientID,
		ClientSecret: s.Config.OIDCClientSecret,
		RedirectURL:  callbackURI,
	})
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}

	oauth2Token, err := oauth2Cfg.Exchange(oidcCtx, code, oauth2.VerifierOption(verifyEntry.CodeVerifier))
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "code exchange: "+err.Error())
	}

	// Verify the ID token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return apiErr(c, http.StatusInternalServerError, "no id_token in response")
	}

	verifier, err := auth.NewOIDCVerifier(oidcCtx, s.Config.OIDCIssuerURL, s.Config.OIDCClientID)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "create verifier: "+err.Error())
	}

	idToken, err := verifier.Verify(oidcCtx, rawIDToken)
	if err != nil {
		return apiErr(c, http.StatusUnauthorized, "verify id_token: "+err.Error())
	}

	// Verify nonce to prevent ID token replay.
	if idToken.Nonce != verifyEntry.Nonce {
		return apiErr(c, http.StatusUnauthorized, "nonce mismatch")
	}

	claims, err := identityFromToken(idToken, !s.Config.AllowUnverifiedEmail)
	if err != nil {
		if errors.Is(err, errEmailNotVerified) {
			return apiErr(c, http.StatusForbidden, "email address is not verified")
		}
		return apiErr(c, http.StatusInternalServerError, "extract claims: "+err.Error())
	}

	// Mark the device as authorized with the real OIDC identity.
	dcEntry, err := sessionGet[deviceCodeEntry](ctx, s.SessionStore, prefixDeviceCode, verifyEntry.DeviceCode)
	if err == nil {
		dcEntry.Authorized = true
		dcEntry.UserEmail = claims.Email
		dcEntry.UserName = claims.Name
		dcEntry.OIDCSub = idToken.Subject
		_ = sessionSet(ctx, s.SessionStore, prefixDeviceCode, verifyEntry.DeviceCode, dcEntry, authStateTTL)
	}

	return c.Redirect(http.StatusFound, "/device/authorized")
}

const refreshCookieName = "bowrain_refresh"

// signedInHintCookie is a NON-HttpOnly, non-sensitive hint cookie set on the
// PARENT domain (e.g. ".bowrain.cloud") alongside the session. It carries no
// secret — just "1" — and exists so the same-site marketing landing can render
// the correct CTA ("Go to app" vs "Sign in") on first paint, before its
// credentialed whoami fetch resolves. whoami remains the source of truth; the
// hint only removes the flash.
const signedInHintCookie = "bowrain_signed_in"

// parentCookieDomain derives the registrable parent domain for the cross-
// subdomain hint cookie by stripping the first DNS label from the request host
// (app.bowrain.cloud → ".bowrain.cloud"), so a cookie set by the app is
// readable by the landing on the parent domain. It returns "" — meaning "omit
// the Domain attribute" (a host-only cookie) — for localhost, IP hosts, or any
// host with fewer than three labels, where a parent Domain would either be
// rejected by the browser (public suffix) or be pointless.
func parentCookieDomain(c echo.Context) string {
	host := c.Request().Header.Get("X-Forwarded-Host")
	if host == "" {
		host = c.Request().Host
	}
	// Drop any port, then a trailing dot (fully-qualified form).
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")

	// Host-only for localhost and bare IPs.
	if host == "localhost" || net.ParseIP(host) != nil {
		return ""
	}

	labels := strings.Split(host, ".")
	// Need at least sub.domain.tld (3 labels) to strip a subdomain and be left
	// with a non-public-suffix parent. Fewer → host-only.
	if len(labels) < 3 {
		return ""
	}
	return "." + strings.Join(labels[1:], ".")
}

// cookieSecure reports whether session cookies should carry the Secure flag.
// It is true on a real HTTPS request OR when ForceSecureCookies is set. The
// override matters behind CloudFront→ALB: TLS terminates at the edge and the
// ALB→task hop is plaintext, so c.Scheme() depends on X-Forwarded-Proto
// reaching the task. Forcing it in prod removes that implicit dependency so a
// proxy-header change can never silently drop Secure.
func (s *Server) cookieSecure(c echo.Context) bool {
	return c.Scheme() == "https" || s.Config.ForceSecureCookies
}

// setSessionCookies sets HttpOnly cookies for the access and refresh tokens.
func (s *Server) setSessionCookies(c echo.Context, accessToken, refreshToken string) {
	secure := s.cookieSecure(c)

	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    accessToken,
		Path:     "/api/",
		MaxAge:   900, // 15 minutes — matches access token lifetime
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	if refreshToken != "" {
		c.SetCookie(&http.Cookie{
			Name:     refreshCookieName,
			Value:    refreshToken,
			Path:     "/api/v1/auth/refresh",
			MaxAge:   30 * 24 * 60 * 60, // 30 days
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})
	}

	// No-flash hint for the landing (parent-domain, readable by JS, non-secret).
	// MaxAge tracks the session cookie so hint and whoami expire together — a
	// stale hint would otherwise flash the wrong CTA.
	c.SetCookie(&http.Cookie{
		Name:     signedInHintCookie,
		Value:    "1",
		Path:     "/",
		Domain:   parentCookieDomain(c),
		MaxAge:   900, // 15 minutes — matches the session cookie
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookies removes the session and refresh cookies.
func (s *Server) clearSessionCookies(c echo.Context) {
	secure := s.cookieSecure(c)

	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/api/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth/refresh",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	// Clear the parent-domain no-flash hint (same Domain/Path as when set).
	c.SetCookie(&http.Cookie{
		Name:     signedInHintCookie,
		Value:    "",
		Path:     "/",
		Domain:   parentCookieDomain(c),
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// handleOIDCCodeExchange performs the OAuth2 authorization code exchange for
// the web flow. It validates the state parameter, sends the PKCE verifier,
// and checks the nonce in the returned ID token.
func (s *Server) handleOIDCCodeExchange(c echo.Context, code, state string) error {
	ctx := c.Request().Context()

	if s.Config.OIDCIssuerURL == "" || s.Config.OIDCClientID == "" {
		return apiErr(c, http.StatusServiceUnavailable, "OIDC not configured")
	}

	// Look up and consume the pending web auth state.
	webEntry, err := sessionGet[webAuthEntry](ctx, s.SessionStore, prefixWebAuth, state)
	if err != nil {
		return c.HTML(http.StatusBadRequest, `<!DOCTYPE html><html><body style="font-family:system-ui;text-align:center;padding:60px">
<h1>Invalid or Expired Session</h1><p>Please try signing in again.</p></body></html>`)
	}
	_ = sessionDelete(ctx, s.SessionStore, prefixWebAuth, state)

	redirectURL := requestBaseURL(c) + "/api/v1/auth/callback"

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
		return apiErr(c, http.StatusInternalServerError, "OIDC discovery failed: "+err.Error())
	}

	oauth2Token, err := oauth2Cfg.Exchange(oidcCtx, code, oauth2.VerifierOption(webEntry.CodeVerifier))
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "code exchange: "+err.Error())
	}

	// Verify the ID token.
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return apiErr(c, http.StatusInternalServerError, "no id_token in response")
	}

	verifier, err := auth.NewOIDCVerifier(oidcCtx, s.Config.OIDCIssuerURL, s.Config.OIDCClientID)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "create verifier: "+err.Error())
	}

	idToken, err := verifier.Verify(oidcCtx, rawIDToken)
	if err != nil {
		return apiErr(c, http.StatusUnauthorized, "verify id_token: "+err.Error())
	}

	// Verify nonce to prevent ID token replay.
	if idToken.Nonce != webEntry.Nonce {
		return apiErr(c, http.StatusUnauthorized, "nonce mismatch")
	}

	claims, err := identityFromToken(idToken, !s.Config.AllowUnverifiedEmail)
	if err != nil {
		if errors.Is(err, errEmailNotVerified) {
			return apiErr(c, http.StatusForbidden, "email address is not verified")
		}
		return apiErr(c, http.StatusInternalServerError, "extract claims: "+err.Error())
	}

	user, err := s.Services.Auth.GetOrCreateUser(ctx, claims.Email, claims.Name, "", idToken.Subject, requestLocale(c))
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "create user: "+err.Error())
	}
	s.trackUserLogin(user.ID, user.Email, user.CreatedAt)
	s.emitAuthEvent(c, platev.EventAuthLogin, user.ID, user.Name, "oidc")

	token, err := s.Services.Auth.GenerateToken(user, 15*time.Minute)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "generate token: "+err.Error())
	}

	// Generate and store a refresh token.
	refreshToken, rtErr := platformAuth.GenerateRefreshToken()
	if rtErr != nil {
		return apiErr(c, http.StatusInternalServerError, "generate refresh token: "+rtErr.Error())
	}
	rtHash := sha256.Sum256([]byte(refreshToken))
	if _, err := s.AuthStore.StoreRefreshToken(ctx, user.ID, hex.EncodeToString(rtHash[:]), time.Now().Add(30*24*time.Hour)); err != nil {
		// Never hand the client a refresh token that was not persisted —
		// it would be unredeemable (silent forced re-login).
		return apiErr(c, http.StatusInternalServerError, "failed to store refresh token")
	}

	// Set HttpOnly cookies and redirect to frontend (no tokens in URL).
	s.setSessionCookies(c, token, refreshToken)

	// Store the raw OIDC ID token for RP-Initiated Logout (id_token_hint).
	_ = s.SessionStore.Set(ctx, prefixIDToken+user.ID, []byte(rawIDToken), 24*time.Hour)

	// Check for a return-path cookie (e.g. from /join/:code before OIDC redirect).
	returnPath := "/"
	if rp, err := c.Cookie("bowrain_return_path"); err == nil && rp.Value != "" {
		returnPath = sanitizeReturnPath(rp.Value)
		// Clear the return-path cookie.
		secure := s.cookieSecure(c)
		c.SetCookie(&http.Cookie{
			Name:     "bowrain_return_path",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
	return c.Redirect(http.StatusFound, returnPath)
}

// RefreshRequest is the request body for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// HandleTokenRefresh exchanges a valid refresh token for a new access token
// and a rotated refresh token. The old refresh token is consumed (single-use).
func (s *Server) HandleTokenRefresh(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	var req RefreshRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, "invalid request")
	}

	// Accept refresh token from JSON body or cookie.
	rawRefresh := req.RefreshToken
	if rawRefresh == "" {
		if rc, err := c.Cookie(refreshCookieName); err == nil {
			rawRefresh = rc.Value
		}
	}
	if rawRefresh == "" {
		return apiErr(c, http.StatusBadRequest, "refresh_token required")
	}

	// Hash the incoming token for lookup.
	hash := sha256.Sum256([]byte(rawRefresh))
	tokenHash := hex.EncodeToString(hash[:])

	// Generate the successor refresh token up front so rotation is a single
	// atomic operation (consume the old + insert the new in one transaction).
	newRefreshToken, err := platformAuth.GenerateRefreshToken()
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "failed to generate refresh token")
	}
	newHashArr := sha256.Sum256([]byte(newRefreshToken))
	newHash := hex.EncodeToString(newHashArr[:])

	ctx := c.Request().Context()
	userID, err := s.AuthStore.RotateRefreshToken(ctx, tokenHash, newHash, time.Now().Add(30*24*time.Hour))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRefreshTokenReuse):
			// A rotated token was replayed (likely stolen). The family is now
			// revoked; clear the session so the client must re-authenticate.
			s.clearSessionCookies(c)
			slog.WarnContext(ctx, "refresh token reuse detected; token family revoked", "ip", c.RealIP())
			return apiErr(c, http.StatusUnauthorized, "session revoked, please sign in again")
		case errors.Is(err, auth.ErrRefreshTokenInvalid):
			s.clearSessionCookies(c)
			return apiErr(c, http.StatusUnauthorized, "invalid or expired refresh token")
		default:
			return apiErr(c, http.StatusInternalServerError, "failed to rotate refresh token")
		}
	}

	// Get user info for the new JWT.
	user, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "user not found")
	}

	// Generate new access token.
	accessToken, err := s.Services.Auth.GenerateToken(user, 15*time.Minute)
	if err != nil {
		return apiErr(c, http.StatusInternalServerError, "failed to generate token")
	}

	// Set cookies (for web clients) and return JSON (for CLI/desktop).
	s.setSessionCookies(c, accessToken, newRefreshToken)

	return c.JSON(http.StatusOK, platformAuth.TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    900,
		RefreshToken: newRefreshToken,
	})
}

// HandleAuthMe returns the current authenticated user.
func (s *Server) HandleAuthMe(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}

	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}

	ctx := c.Request().Context()
	user, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return apiErr(c, http.StatusNotFound, "user not found")
	}

	return c.JSON(http.StatusOK, user)
}

// WhoAmIResponse is the trimmed, display-only body returned by
// GET /api/v1/auth/whoami. It intentionally carries ONLY fields safe to expose
// to a cross-origin caller (the marketing landing) — a name, an email, and an
// avatar URL — and never a token, an internal user id, or the OIDC subject.
type WhoAmIResponse struct {
	Authenticated bool   `json:"authenticated"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	AvatarURL     string `json:"avatar_url"`
}

// HandleAuthWhoami is an unauthenticated-tolerant identity probe. It resolves
// the session cookie itself (this route is registered OUTSIDE the auth-required
// group) and ALWAYS returns 200:
//
//   - no / invalid / expired session → {authenticated:false} with empty fields;
//   - valid session → {authenticated:true} with the display name, email, and
//     avatar URL.
//
// The same-site marketing landing calls this cross-origin with credentials to
// decide between a "Sign in" and a "Go to app" CTA. The response is display-only
// by construction (WhoAmIResponse) — it never leaks a token or the full user
// record across origins.
func (s *Server) HandleAuthWhoami(c echo.Context) error {
	claims := validateSessionCookie(c, s.Config.JWTSecret)
	if claims == nil {
		return c.JSON(http.StatusOK, WhoAmIResponse{Authenticated: false})
	}

	// The session JWT already carries a display name + email; use them as the
	// answer (and the fallback) so whoami stays correct even if the user-record
	// lookup below is briefly unavailable.
	resp := WhoAmIResponse{
		Authenticated: true,
		Name:          claims.Name,
		Email:         claims.Email,
	}

	// Enrich with the freshest profile (name/email may have changed) and the
	// avatar URL from the user record. Best-effort: a lookup miss leaves the
	// claim-derived fields in place.
	if s.AuthStore != nil {
		if user, err := s.AuthStore.GetUser(c.Request().Context(), claims.Subject); err == nil && user != nil {
			resp.Name = user.Name
			resp.Email = user.Email
			resp.AvatarURL = user.AvatarURL
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleAuthLogout invalidates the current session by revoking all refresh
// tokens, clearing cookies, and returning the OIDC end_session_url so the
// frontend can terminate the Keycloak SSO session.
func (s *Server) HandleAuthLogout(c echo.Context) error {
	ctx := c.Request().Context()

	var rawIDToken string

	// Revoke all refresh tokens for this user so the session cannot be resumed.
	if userID, ok := c.Get("user_id").(string); ok && userID != "" && s.AuthStore != nil {
		_ = s.AuthStore.RevokeUserRefreshTokens(ctx, userID)

		// Retrieve the stored OIDC ID token for the logout hint.
		if data, err := s.SessionStore.Get(ctx, prefixIDToken+userID); err == nil {
			rawIDToken = string(data)
		}
		_ = s.SessionStore.Delete(ctx, prefixIDToken+userID)

		name, _ := c.Get("name").(string)
		s.emitAuthEvent(c, platev.EventAuthLogout, userID, name, "oidc")
	}

	s.clearSessionCookies(c)

	resp := map[string]string{"status": "logged out"}

	// Fully-formed, provider-appropriate logout URL (terminates upstream SSO,
	// returns to the app origin). The frontend redirects to it verbatim.
	if endSessionURL := s.buildEndSessionURL(ctx, c, rawIDToken); endSessionURL != "" {
		resp["end_session_url"] = endSessionURL
	}

	return c.JSON(http.StatusOK, resp)
}

// buildEndSessionURL returns a complete provider-appropriate logout URL, or ""
// when the provider advertises no end_session_endpoint.
//
// Cognito's /logout is NOT OIDC-RP-initiated-logout compatible: it requires
// client_id + logout_uri (a registered sign-out URL) and rejects the standard
// id_token_hint/post_logout_redirect_uri pair — including any expired
// id_token_hint (its ID tokens live only 15 min) — by bouncing to /login with a
// 400. Keycloak uses the standard pair. Branch on the provider so both work.
func (s *Server) buildEndSessionURL(ctx context.Context, c echo.Context, rawIDToken string) string {
	endSession := s.discoverEndSessionEndpoint(ctx)
	if endSession == "" {
		return ""
	}
	// The app origin, matching the client's registered sign-out URL (trailing /).
	return composeEndSessionURL(endSession, s.Config.AuthProvider, s.Config.OIDCClientID, requestBaseURL(c)+"/", rawIDToken)
}

// composeEndSessionURL adds the provider-appropriate query params to an
// end_session_endpoint. Cognito → client_id + logout_uri (simple logout);
// everything else → the standard OIDC post_logout_redirect_uri (+ id_token_hint,
// + client_id). Pure, so it is unit-testable without OIDC discovery.
func composeEndSessionURL(endpoint, provider, clientID, postLogout, rawIDToken string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	q := u.Query()
	if provider == AuthProviderCognito {
		q.Set("client_id", clientID)
		q.Set("logout_uri", postLogout)
	} else {
		q.Set("post_logout_redirect_uri", postLogout)
		if rawIDToken != "" {
			q.Set("id_token_hint", rawIDToken)
		}
		if clientID != "" {
			q.Set("client_id", clientID)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// discoverEndSessionEndpoint fetches the OIDC provider's end_session_endpoint
// from the discovery document. Returns "" if OIDC is not configured or the
// endpoint is not advertised.
func (s *Server) discoverEndSessionEndpoint(ctx context.Context) string {
	if s.Config.OIDCIssuerURL == "" {
		return ""
	}

	oidcCtx := s.oidcContext(ctx)
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		return ""
	}

	var providerClaims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&providerClaims); err != nil || providerClaims.EndSessionEndpoint == "" {
		return ""
	}

	endSessionURL := providerClaims.EndSessionEndpoint
	// When OIDCPublicURL differs from OIDCIssuerURL (Docker), rewrite
	// the internal URL to the browser-reachable public URL.
	if s.Config.OIDCPublicURL != "" && s.Config.OIDCPublicURL != s.Config.OIDCIssuerURL {
		endSessionURL = strings.Replace(endSessionURL, s.Config.OIDCIssuerURL, s.Config.OIDCPublicURL, 1)
	}

	return endSessionURL
}

// HandleBackChannelLogout handles OIDC Back-Channel Logout requests from
// the identity provider (Keycloak). When Keycloak terminates a session
// (admin action, timeout, logout from another app), it POSTs a logout_token
// to this endpoint. We verify the JWT, look up the user by OIDC subject,
// and revoke all their refresh tokens.
//
// Spec: https://openid.net/specs/openid-connect-backchannel-1_0.html
func (s *Server) HandleBackChannelLogout(c echo.Context) error {
	if s.Config.OIDCIssuerURL == "" || s.AuthStore == nil {
		return c.NoContent(http.StatusBadRequest)
	}

	// The logout_token is delivered as application/x-www-form-urlencoded.
	rawLogoutToken := c.FormValue("logout_token")
	if rawLogoutToken == "" {
		return apiErr(c, http.StatusBadRequest, "logout_token required")
	}

	ctx := c.Request().Context()
	oidcCtx := s.oidcContext(ctx)

	// Create a remote keyset to verify the JWT signature against Keycloak's JWKS.
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		slog.WarnContext(ctx, "back-channel logout: OIDC discovery failed", "error", err)
		return apiErr(c, http.StatusBadRequest, "OIDC discovery failed")
	}

	keySet := provider.Verifier(&oidc.Config{
		ClientID:                   s.Config.OIDCClientID,
		SkipExpiryCheck:            false,
		SkipIssuerCheck:            false,
		InsecureSkipSignatureCheck: false,
		Now:                        time.Now,
	})

	// Verify signature and standard claims (iss, aud, exp).
	idToken, err := keySet.Verify(oidcCtx, rawLogoutToken)
	if err != nil {
		slog.WarnContext(ctx, "back-channel logout: token verification failed", "error", err)
		return apiErr(c, http.StatusBadRequest, "invalid logout_token")
	}

	// Extract and validate back-channel logout specific claims.
	var logoutClaims struct {
		Events json.RawMessage `json:"events"`
		Nonce  *string         `json:"nonce"`
		Sub    string          `json:"sub"`
		Sid    string          `json:"sid"`
	}
	if err := idToken.Claims(&logoutClaims); err != nil {
		slog.WarnContext(ctx, "back-channel logout: failed to extract claims", "error", err)
		return apiErr(c, http.StatusBadRequest, "invalid claims")
	}

	// Spec: logout token MUST contain the back-channel logout event.
	var events map[string]json.RawMessage
	if err := json.Unmarshal(logoutClaims.Events, &events); err != nil {
		return apiErr(c, http.StatusBadRequest, "invalid events claim")
	}
	if _, ok := events["http://schemas.openid.net/event/backchannel-logout"]; !ok {
		return apiErr(c, http.StatusBadRequest, "missing backchannel-logout event")
	}

	// Spec: logout token MUST NOT contain a nonce claim.
	if logoutClaims.Nonce != nil {
		return apiErr(c, http.StatusBadRequest, "logout_token must not contain nonce")
	}

	// Spec: must have sub and/or sid.
	if logoutClaims.Sub == "" && logoutClaims.Sid == "" {
		return apiErr(c, http.StatusBadRequest, "logout_token must contain sub or sid")
	}

	// Look up user by OIDC subject and revoke their tokens.
	if logoutClaims.Sub != "" {
		user, err := s.AuthStore.GetUserByOIDCSub(ctx, logoutClaims.Sub)
		if err != nil {
			// User not found — nothing to revoke. Still return 200 per spec.
			slog.InfoContext(ctx, "back-channel logout: no user found for subject", "oidc_sub", logoutClaims.Sub)
			return c.NoContent(http.StatusOK)
		}

		if err := s.AuthStore.RevokeUserRefreshTokens(ctx, user.ID); err != nil {
			slog.ErrorContext(ctx, "back-channel logout: failed to revoke tokens", "user_id", user.ID, "error", err)
			return apiErr(c, http.StatusInternalServerError, "failed to revoke tokens")
		}

		slog.InfoContext(ctx, "back-channel logout: revoked tokens", "user_id", user.ID, "oidc_sub", logoutClaims.Sub)
	}

	return c.NoContent(http.StatusOK)
}

// sanitizeReturnPath validates that a return path is a safe relative URL.
// It rejects absolute URLs, protocol-relative URLs, and URLs containing
// authority components that could be used for open redirect attacks.
func sanitizeReturnPath(raw string) string {
	if raw == "" {
		return "/"
	}
	// Decode percent-encoded value (cookie may be URL-encoded by the browser).
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return "/"
	}
	// Must start with a single slash (relative to origin).
	if !strings.HasPrefix(decoded, "/") {
		return "/"
	}
	// Reject protocol-relative URLs (//evil.com) and paths with authority (@).
	// Backslashes are rejected too: some browsers normalize "\" to "/", so
	// "/\evil.com" would become "//evil.com" (protocol-relative) after redirect.
	if strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "@\\") {
		return "/"
	}
	// Parse to reject any scheme or host that slipped through.
	u, err := url.Parse(decoded)
	if err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return decoded
}

// requestLocale extracts the preferred locale from the request's
// Accept-Language header as a lowercase BCP-47 primary subtag (e.g.
// "nb-NO;q=0.9, en;q=0.8" → "nb"). Returns "" when the header is absent or
// unparsable. Used to capture a user's locale at sign-in; consumers (e.g.
// the transactional-email send path) always fall back to English.
func requestLocale(c echo.Context) string {
	header := c.Request().Header.Get("Accept-Language")
	if header == "" {
		return ""
	}
	first, _, _ := strings.Cut(header, ",")
	tag, _, _ := strings.Cut(strings.TrimSpace(first), ";")
	base, _, _ := strings.Cut(strings.TrimSpace(tag), "-")
	base = strings.ToLower(base)
	if base == "" || base == "*" {
		return ""
	}
	// Primary subtags are 2–8 alphabetic characters (BCP 47).
	if len(base) < 2 || len(base) > 8 {
		return ""
	}
	for _, r := range base {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	return base
}
