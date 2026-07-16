// Package server — handlers_passkeys.go
//
// Account self-service passkey (WebAuthn) management via the provider-neutral
// CredentialManager port. On hosted prod (Cognito) the server relays the
// WebAuthn ceremony to the browser: it mints a short-lived, user-scoped access
// token on demand from the retained upstream refresh token, calls the IdP's
// WebAuthn APIs, and returns only the ceremony payload — no IdP token ever
// reaches the browser (BFF invariant). On self-host (Keycloak) InApp() is false
// and these endpoints steer the client to the account console instead.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	"golang.org/x/oauth2"
)

// upstreamProviderCognito is the provider key under which the Cognito refresh
// token is retained in UpstreamTokenStore.
const upstreamProviderCognito = "cognito"

// passkeyNonceTTL bounds the start→finish registration handshake.
const passkeyNonceTTL = 5 * time.Minute

// errReauthRequired signals that a user-scoped access token could not be minted
// (no retained refresh token, or it expired/was revoked). The handlers map it
// to 409 {"error":"reauth_required"} so the UI can prompt a fresh login.
var errReauthRequired = errors.New("reauth required")

// retainUpstreamRefreshToken persists the Cognito refresh token from a login
// exchange (encrypted) so later passkey operations can mint a user-scoped
// access token without re-prompting. No-op unless the provider is Cognito, the
// store is configured, and the token carries a refresh_token.
func (s *Server) retainUpstreamRefreshToken(ctx context.Context, userID string, token *oauth2.Token) {
	if s.UpstreamTokens == nil || s.Config.AuthProvider != AuthProviderCognito || token == nil || userID == "" {
		return
	}
	refreshToken, _ := token.Extra("refresh_token").(string)
	if refreshToken == "" {
		return
	}
	// The customer app client's refresh_token_validity is 30 days; store to
	// match so an expired token is treated as gone (→ reauth_required) rather
	// than sent on a doomed refresh.
	if err := s.UpstreamTokens.PutRefreshToken(ctx, userID, upstreamProviderCognito, refreshToken, time.Now().Add(30*24*time.Hour)); err != nil {
		slog.WarnContext(ctx, "retain upstream refresh token", "user_id", userID, "error", err)
	}
}

// freshCognitoAccessToken loads the retained refresh token and exchanges it at
// the Cognito token endpoint (grant_type=refresh_token, HTTP Basic client auth)
// for a short-lived user-scoped access token. The token is used only for the
// duration of the server-side call and never returned to the browser. Returns
// errReauthRequired when no live refresh token is available or the grant is
// rejected.
func (s *Server) freshCognitoAccessToken(ctx context.Context, userID string) (string, error) {
	if s.UpstreamTokens == nil {
		return "", errReauthRequired
	}
	refreshToken, _, err := s.UpstreamTokens.GetRefreshToken(ctx, userID, upstreamProviderCognito)
	if err != nil {
		// Not found or expired — force a fresh login before managing credentials.
		return "", errReauthRequired
	}

	oidcCtx := s.oidcContext(ctx)
	oauth2Cfg, err := auth.NewOAuth2Config(oidcCtx, auth.OIDCConfig{
		IssuerURL:    s.Config.OIDCIssuerURL,
		ClientID:     s.Config.OIDCClientID,
		ClientSecret: s.Config.OIDCClientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("oidc discovery: %w", err)
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(oidcCtx, http.MethodPost, oauth2Cfg.Endpoint.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// Confidential client → HTTP Basic (client_id:client_secret). This avoids
	// InitiateAuth / SECRET_HASH entirely and needs no IAM.
	req.SetBasicAuth(s.Config.OIDCClientID, s.Config.OIDCClientSecret)

	client := http.DefaultClient
	if hc, ok := oidcCtx.Value(oauth2.HTTPClient).(*http.Client); ok && hc != nil {
		client = hc
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token endpoint request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// invalid_grant (expired/revoked refresh token) surfaces as 400/401.
		// Prune the dead token and force re-login.
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			_ = s.UpstreamTokens.DeleteRefreshToken(ctx, userID, upstreamProviderCognito)
			return "", errReauthRequired
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("token endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errReauthRequired
	}
	return tr.AccessToken, nil
}

// passkeyGuard enforces the common preconditions for the in-app passkey
// endpoints: CredentialManager configured (503), authenticated (401), and the
// provider supports in-app management (409 → steer to the account console). On
// failure it writes the response and returns ok=false; the caller returns nil.
func (s *Server) passkeyGuard(c echo.Context) (userID string, ok bool) {
	if s.CredentialManager == nil {
		_ = c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "passkey management is not configured"})
		return "", false
	}
	userID, isStr := c.Get("user_id").(string)
	if !isStr || userID == "" {
		_ = c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
		return "", false
	}
	if !s.CredentialManager.InApp() {
		_ = c.JSON(http.StatusConflict, map[string]any{
			"error":       "account_console_required",
			"account_url": s.CredentialManager.AccountURL(""),
		})
		return "", false
	}
	return userID, true
}

// passkeyTokenError maps a freshCognitoAccessToken failure to an HTTP response.
func (s *Server) passkeyTokenError(c echo.Context, err error) error {
	if errors.Is(err, errReauthRequired) {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "reauth_required"})
	}
	return c.JSON(http.StatusBadGateway, ErrorResponse{Error: "mint access token: " + err.Error()})
}

// passkeyJSON is the wire shape for a listed credential.
type passkeyJSON struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	CreatedAt  string   `json:"created_at,omitempty"`
	Transports []string `json:"transports,omitempty"`
	RPID       string   `json:"rp_id,omitempty"`
}

func passkeysToJSON(pks []auth.Passkey) []passkeyJSON {
	out := make([]passkeyJSON, 0, len(pks))
	for _, p := range pks {
		created := ""
		if !p.CreatedAt.IsZero() {
			created = p.CreatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, passkeyJSON{
			ID:         p.ID,
			Name:       p.Name,
			CreatedAt:  created,
			Transports: p.Transports,
			RPID:       p.RPID,
		})
	}
	return out
}

// HandleAccountSecurity reports how credential management is surfaced for the
// configured provider: in-app (Cognito) or via an external account console
// (Keycloak, with a deep link).
func (s *Server) HandleAccountSecurity(c echo.Context) error {
	if s.CredentialManager == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "account security is not configured"})
	}
	if userID, ok := c.Get("user_id").(string); !ok || userID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
	}
	inApp := s.CredentialManager.InApp()
	accountURL := ""
	if !inApp {
		accountURL = s.CredentialManager.AccountURL("")
	}
	return c.JSON(http.StatusOK, map[string]any{"in_app": inApp, "account_url": accountURL})
}

// HandleListPasskeys returns the signed-in user's registered passkeys.
func (s *Server) HandleListPasskeys(c echo.Context) error {
	userID, ok := s.passkeyGuard(c)
	if !ok {
		return nil
	}
	ctx := c.Request().Context()
	accessToken, err := s.freshCognitoAccessToken(ctx, userID)
	if err != nil {
		return s.passkeyTokenError(c, err)
	}
	passkeys, err := s.CredentialManager.ListPasskeys(ctx, accessToken)
	if err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Error: "list passkeys: " + err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"passkeys": passkeysToJSON(passkeys)})
}

// HandleRegisterStart begins a passkey registration: it fetches the credential
// creation options from the IdP and returns them plus a single-use nonce that
// binds the subsequent finish call.
func (s *Server) HandleRegisterStart(c echo.Context) error {
	userID, ok := s.passkeyGuard(c)
	if !ok {
		return nil
	}
	if s.SessionStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "session store is not configured"})
	}
	ctx := c.Request().Context()
	accessToken, err := s.freshCognitoAccessToken(ctx, userID)
	if err != nil {
		return s.passkeyTokenError(c, err)
	}
	options, err := s.CredentialManager.RegisterStart(ctx, accessToken)
	if err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Error: "start registration: " + err.Error()})
	}
	nonce, err := newPasskeyNonce()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "generate nonce: " + err.Error()})
	}
	if err := s.SessionStore.Set(ctx, prefixPasskeyNonce+userID, []byte(nonce), passkeyNonceTTL); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "persist nonce: " + err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"options": json.RawMessage(options),
		"nonce":   nonce,
	})
}

// passkeyFinishRequest is the body for POST /account/passkeys/register/finish.
type passkeyFinishRequest struct {
	Nonce       string          `json:"nonce"`
	Attestation json.RawMessage `json:"attestation"`
}

// HandleRegisterFinish completes a passkey registration: it validates and
// consumes the nonce, then submits the browser's attestation to the IdP.
func (s *Server) HandleRegisterFinish(c echo.Context) error {
	userID, ok := s.passkeyGuard(c)
	if !ok {
		return nil
	}
	if s.SessionStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "session store is not configured"})
	}
	var req passkeyFinishRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if strings.TrimSpace(req.Nonce) == "" || len(req.Attestation) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "nonce and attestation are required"})
	}

	ctx := c.Request().Context()
	// Validate + consume the single-use registration nonce (constant-time).
	stored, err := s.SessionStore.Get(ctx, prefixPasskeyNonce+userID)
	if err != nil || subtle.ConstantTimeCompare(stored, []byte(req.Nonce)) != 1 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid or expired registration nonce"})
	}
	_ = s.SessionStore.Delete(ctx, prefixPasskeyNonce+userID)

	accessToken, err := s.freshCognitoAccessToken(ctx, userID)
	if err != nil {
		return s.passkeyTokenError(c, err)
	}
	if err := s.CredentialManager.RegisterFinish(ctx, accessToken, req.Attestation); err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Error: "complete registration: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, map[string]any{"status": "registered"})
}

// HandleDeletePasskey removes a registered passkey by its provider credential id.
func (s *Server) HandleDeletePasskey(c echo.Context) error {
	userID, ok := s.passkeyGuard(c)
	if !ok {
		return nil
	}
	credentialID := c.Param("id")
	if credentialID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "credential id is required"})
	}
	ctx := c.Request().Context()
	accessToken, err := s.freshCognitoAccessToken(ctx, userID)
	if err != nil {
		return s.passkeyTokenError(c, err)
	}
	if err := s.CredentialManager.DeletePasskey(ctx, accessToken, credentialID); err != nil {
		return c.JSON(http.StatusBadGateway, ErrorResponse{Error: "delete passkey: " + err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "deleted"})
}

// newPasskeyNonce returns a 256-bit random hex nonce.
func newPasskeyNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
