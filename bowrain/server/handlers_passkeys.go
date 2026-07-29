// Package server — handlers_passkeys.go
//
// Account self-service passkey (WebAuthn) management via the provider-neutral
// CredentialManager port. On hosted prod (Cognito) the server relays the
// WebAuthn ceremony to the browser and never exposes an IdP token (BFF
// invariant). On self-host (Keycloak) InApp() is false and these endpoints
// steer the client to the account console instead.
//
// Security model — step-up elevation (not always-on scope). Cognito's WebAuthn
// APIs are user-self-service and require an access token carrying the broad
// aws.cognito.signin.user.admin scope (it can also change email, drop MFA,
// delete the account). Rather than bake that scope into every login — which
// would make each retained token account-takeover-grade — the server requests
// it ONLY during an explicit step-up ("elevation"): a fresh authorization-code
// round-trip (prompt=login) that yields a short-lived access token. That token
// is stashed encrypted in the ephemeral session store for a brief window
// (elevationWindow) and used server-side for the ceremony; no long-lived
// admin-capable secret is ever persisted, and an ordinary session can never
// manage credentials.
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	"golang.org/x/oauth2"
)

// cognitoSelfServiceScope is the OAuth scope Cognito requires on the access
// token for its user-self-service APIs (WebAuthn included). Requested only at
// step-up, never at ordinary login.
const cognitoSelfServiceScope = "aws.cognito.signin.user.admin"

// elevationWindow bounds how long a single step-up authorizes credential
// management before the user must confirm their identity again. The stored
// token never outlives this, nor its own (shorter) IdP expiry.
const elevationWindow = 10 * time.Minute

// passkeyNonceTTL bounds the start→finish registration handshake.
const passkeyNonceTTL = 5 * time.Minute

// elevatePath is the step-up entry point the UI is steered to on 409.
const elevatePath = "/api/v1/account/security/elevate"

// errElevationRequired signals that no live elevated access token is available
// (never elevated, or the window expired). Handlers map it to
// 409 {"error":"elevation_required","elevate_url":…} so the UI can start a
// step-up re-auth.
var errElevationRequired = errors.New("elevation required")

// elevateEntry is the pending step-up state, keyed by the OIDC `state` in the
// session store. UserID binds the callback to the session that started it;
// Return is the sanitized same-origin path to send the browser back to.
type elevateEntry struct {
	CodeVerifier string `json:"code_verifier"`
	Nonce        string `json:"nonce"`
	UserID       string `json:"user_id"`
	Return       string `json:"return"`
}

// storeElevatedToken seals the access token and stores it in the ephemeral
// session store under the user, bounded by elevationWindow (and never past the
// token's own expiry).
func (s *Server) storeElevatedToken(ctx context.Context, userID, accessToken string, exp time.Time) error {
	if s.SessionStore == nil {
		return errors.New("session store is not configured")
	}
	ttl := elevationWindow
	if !exp.IsZero() {
		if until := time.Until(exp); until < ttl {
			ttl = until
		}
	}
	if ttl <= 0 {
		return errors.New("elevated token already expired")
	}
	sealed, err := s.secretsCipher.Seal(accessToken)
	if err != nil {
		return err
	}
	return s.SessionStore.Set(ctx, prefixElevatedToken+userID, []byte(sealed), ttl)
}

// elevatedCognitoAccessToken returns the user's current elevated access token,
// or errElevationRequired if none is live.
func (s *Server) elevatedCognitoAccessToken(ctx context.Context, userID string) (string, error) {
	if s.SessionStore == nil {
		return "", errElevationRequired
	}
	sealed, err := s.SessionStore.Get(ctx, prefixElevatedToken+userID)
	if err != nil {
		return "", errElevationRequired
	}
	token, err := s.secretsCipher.Open(string(sealed))
	if err != nil || token == "" {
		return "", errElevationRequired
	}
	return token, nil
}

// clearElevatedToken drops the stored elevated token (best effort). Used when
// the IdP reports the token no longer carries the required scope, so the next
// attempt re-elevates rather than replays a stale token.
func (s *Server) clearElevatedToken(ctx context.Context, userID string) {
	if s.SessionStore != nil {
		_ = s.SessionStore.Delete(ctx, prefixElevatedToken+userID)
	}
}

// passkeyGuard enforces the common preconditions for the in-app passkey
// endpoints: CredentialManager configured (503), authenticated (401), and the
// provider supports in-app management (409 → steer to the account console). On
// failure it writes the response and returns ok=false; the caller returns nil.
func (s *Server) passkeyGuard(c echo.Context) (userID string, ok bool) {
	if s.CredentialManager == nil {
		_ = apiErr(c, http.StatusServiceUnavailable, "passkey management is not configured")
		return "", false
	}
	userID, isStr := c.Get("user_id").(string)
	if !isStr || userID == "" {
		_ = apiErr(c, http.StatusUnauthorized, "not authenticated")
		return "", false
	}
	if !s.CredentialManager.InApp() {
		_ = apiErr(c, http.StatusConflict, "account_console_required", map[string]any{
			"account_url": s.CredentialManager.AccountURL(""),
		})
		return "", false
	}
	return userID, true
}

// elevationRequiredJSON writes the 409 the UI treats as "start a step-up".
func elevationRequiredJSON(c echo.Context) error {
	return apiErr(c, http.StatusConflict, "elevation_required", map[string]any{
		"elevate_url": elevatePath,
	})
}

// isScopeError reports whether an IdP operation failed because the access token
// lacks the self-service scope. It can happen if a token was minted before the
// scope was allowed; treat it as "re-elevate" rather than a hard 502.
func isScopeError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NotAuthorizedException") && strings.Contains(msg, "required scopes")
}

// passkeyOpError maps a CredentialManager operation error to a response: a
// missing-scope failure becomes 409 elevation_required (re-elevate); anything
// else is a 502 with the given prefix.
func (s *Server) passkeyOpError(c echo.Context, userID, prefix string, err error) error {
	if isScopeError(err) {
		s.clearElevatedToken(c.Request().Context(), userID)
		return elevationRequiredJSON(c)
	}
	return serverErrStatus(c, http.StatusBadGateway, fmt.Errorf("%s: %w", prefix, err))
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
// (Keycloak, with a deep link). It needs no elevated token — it only reports
// capability.
func (s *Server) HandleAccountSecurity(c echo.Context) error {
	if s.CredentialManager == nil {
		return apiErr(c, http.StatusServiceUnavailable, "account security is not configured")
	}
	if userID, ok := c.Get("user_id").(string); !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	inApp := s.CredentialManager.InApp()
	accountURL := ""
	if !inApp {
		accountURL = s.CredentialManager.AccountURL("")
	}
	return c.JSON(http.StatusOK, map[string]any{"in_app": inApp, "account_url": accountURL})
}

// HandleSecurityElevate begins a step-up re-authentication that yields a
// short-lived, self-service-scoped Cognito access token. It redirects the
// browser to the IdP authorize endpoint requesting cognitoSelfServiceScope with
// prompt=login (fresh auth). The callback stashes only the resulting access
// token (encrypted, short TTL). This is a top-level GET navigation, so it rides
// the SameSite=Lax session cookie and is CSRF-exempt.
func (s *Server) HandleSecurityElevate(c echo.Context) error {
	if s.CredentialManager == nil {
		return apiErr(c, http.StatusServiceUnavailable, "account security is not configured")
	}
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	if !s.CredentialManager.InApp() {
		// Keycloak et al. manage credentials in their own console — no step-up.
		return apiErr(c, http.StatusConflict, "account_console_required", map[string]any{
			"account_url": s.CredentialManager.AccountURL(""),
		})
	}
	if s.SessionStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "session store is not configured")
	}
	if s.Config.OIDCIssuerURL == "" || s.Config.OIDCClientID == "" {
		return apiErr(c, http.StatusServiceUnavailable, "OIDC not configured")
	}

	oidcCtx := s.oidcContext(c.Request().Context())
	provider, err := oidc.NewProvider(oidcCtx, s.Config.OIDCIssuerURL)
	if err != nil {
		return serverErr(c, fmt.Errorf("OIDC discovery failed: %w", err))
	}
	authURL := provider.Endpoint().AuthURL
	if s.Config.OIDCPublicURL != "" && s.Config.OIDCPublicURL != s.Config.OIDCIssuerURL {
		authURL = strings.Replace(authURL, s.Config.OIDCIssuerURL, s.Config.OIDCPublicURL, 1)
	}

	ap, err := newOIDCAuthParams()
	if err != nil {
		return serverErr(c, err)
	}

	ctx := c.Request().Context()
	entry := &elevateEntry{
		CodeVerifier: ap.CodeVerifier,
		Nonce:        ap.Nonce,
		UserID:       userID,
		Return:       sanitizeReturnPath(c.QueryParam("return")),
	}
	if err := sessionSet(ctx, s.SessionStore, prefixElevateState, ap.State, entry, authStateTTL); err != nil {
		return serverErr(c, fmt.Errorf("store elevate state: %w", err))
	}

	redirectURI := requestBaseURL(c) + "/api/v1/account/security/elevate/callback"
	params := url.Values{
		"client_id":             {s.Config.OIDCClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid " + cognitoSelfServiceScope},
		"state":                 {ap.State},
		"code_challenge":        {ap.Challenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {ap.Nonce},
		// Force a fresh authentication for this sensitive step-up rather than
		// silently reusing the IdP SSO session.
		"prompt": {"login"},
	}
	return c.Redirect(http.StatusFound, authURL+"?"+params.Encode())
}

// HandleSecurityElevateCallback completes the step-up: it validates the bound
// state, exchanges the code for a self-service-scoped access token, stashes it
// (encrypted, short TTL), and redirects the browser back to the Security page
// with ?elevated=1 (or ?elevated=0 on any failure — never a hard error page,
// since this is a UX gate the user can retry).
func (s *Server) HandleSecurityElevateCallback(c echo.Context) error {
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()

	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if state == "" {
		return c.Redirect(http.StatusFound, "/?elevated=0")
	}
	entry, err := sessionGet[elevateEntry](ctx, s.SessionStore, prefixElevateState, state)
	if err != nil {
		return c.Redirect(http.StatusFound, "/?elevated=0")
	}
	_ = sessionDelete(ctx, s.SessionStore, prefixElevateState, state)

	returnTo := sanitizeReturnPath(entry.Return)
	fail := func() error { return c.Redirect(http.StatusFound, returnTo+"?elevated=0") }

	// Bind the callback to the session that initiated the step-up.
	if entry.UserID != userID {
		return fail()
	}
	if code == "" {
		return fail()
	}

	redirectURI := requestBaseURL(c) + "/api/v1/account/security/elevate/callback"
	oidcCtx := s.oidcContext(ctx)
	oauth2Cfg, err := auth.NewOAuth2Config(oidcCtx, auth.OIDCConfig{
		IssuerURL:    s.Config.OIDCIssuerURL,
		ClientID:     s.Config.OIDCClientID,
		ClientSecret: s.Config.OIDCClientSecret,
		RedirectURL:  redirectURI,
	})
	if err != nil {
		return fail()
	}
	token, err := oauth2Cfg.Exchange(oidcCtx, code, oauth2.VerifierOption(entry.CodeVerifier))
	if err != nil {
		return fail()
	}
	// Verify the id_token nonce (replay protection) when present.
	if raw, hasID := token.Extra("id_token").(string); hasID && raw != "" {
		verifier, verr := auth.NewOIDCVerifier(oidcCtx, s.Config.OIDCIssuerURL, s.Config.OIDCClientID)
		if verr != nil {
			return fail()
		}
		idToken, ierr := verifier.Verify(oidcCtx, raw)
		if ierr != nil || idToken.Nonce != entry.Nonce {
			return fail()
		}
	}
	if token.AccessToken == "" {
		return fail()
	}
	if err := s.storeElevatedToken(ctx, userID, token.AccessToken, token.Expiry); err != nil {
		return fail()
	}
	return c.Redirect(http.StatusFound, returnTo+"?elevated=1")
}

// HandleListPasskeys returns the signed-in user's registered passkeys.
func (s *Server) HandleListPasskeys(c echo.Context) error {
	userID, ok := s.passkeyGuard(c)
	if !ok {
		return nil
	}
	ctx := c.Request().Context()
	accessToken, err := s.elevatedCognitoAccessToken(ctx, userID)
	if err != nil {
		return elevationRequiredJSON(c)
	}
	passkeys, err := s.CredentialManager.ListPasskeys(ctx, accessToken)
	if err != nil {
		return s.passkeyOpError(c, userID, "list passkeys", err)
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
		return apiErr(c, http.StatusServiceUnavailable, "session store is not configured")
	}
	ctx := c.Request().Context()
	accessToken, err := s.elevatedCognitoAccessToken(ctx, userID)
	if err != nil {
		return elevationRequiredJSON(c)
	}
	options, err := s.CredentialManager.RegisterStart(ctx, accessToken)
	if err != nil {
		return s.passkeyOpError(c, userID, "start registration", err)
	}
	nonce, err := newPasskeyNonce()
	if err != nil {
		return serverErr(c, fmt.Errorf("generate nonce: %w", err))
	}
	if err := s.SessionStore.Set(ctx, prefixPasskeyNonce+userID, []byte(nonce), passkeyNonceTTL); err != nil {
		return serverErr(c, fmt.Errorf("persist nonce: %w", err))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"options": options,
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
		return apiErr(c, http.StatusServiceUnavailable, "session store is not configured")
	}
	var req passkeyFinishRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Nonce) == "" || len(req.Attestation) == 0 {
		return apiErr(c, http.StatusBadRequest, "nonce and attestation are required")
	}

	ctx := c.Request().Context()
	// Validate + consume the single-use registration nonce (constant-time).
	stored, err := s.SessionStore.Get(ctx, prefixPasskeyNonce+userID)
	if err != nil || subtle.ConstantTimeCompare(stored, []byte(req.Nonce)) != 1 {
		return apiErr(c, http.StatusBadRequest, "invalid or expired registration nonce")
	}
	_ = s.SessionStore.Delete(ctx, prefixPasskeyNonce+userID)

	accessToken, err := s.elevatedCognitoAccessToken(ctx, userID)
	if err != nil {
		return elevationRequiredJSON(c)
	}
	if err := s.CredentialManager.RegisterFinish(ctx, accessToken, req.Attestation); err != nil {
		return s.passkeyOpError(c, userID, "complete registration", err)
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
		return apiErr(c, http.StatusBadRequest, "credential id is required")
	}
	ctx := c.Request().Context()
	accessToken, err := s.elevatedCognitoAccessToken(ctx, userID)
	if err != nil {
		return elevationRequiredJSON(c)
	}
	if err := s.CredentialManager.DeletePasskey(ctx, accessToken, credentialID); err != nil {
		return s.passkeyOpError(c, userID, "delete passkey", err)
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
