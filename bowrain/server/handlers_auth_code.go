package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"

	platformAuth "github.com/neokapi/neokapi/bowrain/core/auth"
	venueauth "github.com/neokapi/neokapi/host/venue/auth"
)

// The one-time code sign-in for native clients (the Bowrain desktop and mobile apps).
//
// The server brokers the OIDC flow with the identity provider, because the API
// accepts only the tokens the server issues. With response_mode=code the
// desktop sign-in route ends in a redirect that carries a one-time code and the
// client's state, never a token: a custom-scheme redirect can be delivered to
// any app that registered the same scheme. The client redeems the code by POST
// with its PKCE verifier, and the tokens travel only in that response body.
//
//	GET  /api/v1/auth/desktop/login?redirect_uri&code_challenge&code_challenge_method=S256&state&response_mode=code
//	→ identity provider → GET /api/v1/auth/desktop/callback
//	→ 302 <redirect_uri>?code=<one-time code>&state=<client state>
//	POST /api/v1/auth/desktop/token {code, code_verifier, redirect_uri}
//	→ 200 {access_token, refresh_token, token_type, expires_in}

// desktopResponseModeCode asks the sign-in redirect for a one-time code.
const desktopResponseModeCode = "code"

// desktopCodeTTL is how long a one-time sign-in code can be redeemed.
const desktopCodeTTL = 60 * time.Second

// desktopCodeEntry is what a one-time code stands for. It holds no token: the
// tokens are minted when the code is redeemed.
type desktopCodeEntry struct {
	UserID        string `json:"user_id"`
	CodeChallenge string `json:"code_challenge"`
	RedirectURI   string `json:"redirect_uri"`
}

// checkDesktopRedirect accepts a loopback HTTP redirect (the desktop app) or a
// bowrain:// redirect (the native apps), and nothing else.
func checkDesktopRedirect(redirectURI string) error {
	parsed, err := url.Parse(redirectURI)
	if err != nil || redirectURI == "" {
		return errors.New("invalid redirect_uri")
	}
	isLocalhost := parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost"
	isCustomScheme := parsed.Scheme == "bowrain"
	if !isLocalhost && !isCustomScheme {
		return errors.New("redirect_uri must be a loopback http URL or a bowrain:// URL")
	}
	return nil
}

// validS256Challenge reports whether s has the shape of an S256 challenge:
// the unpadded base64url encoding of a SHA-256 digest.
func validS256Challenge(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

// redirectWithLoginCode ends a code-mode sign-in: it stores a one-time code for
// the signed-in user, bound to the client's PKCE challenge and redirect, and
// sends the browser to the client with the code and the client's state.
func (s *Server) redirectWithLoginCode(c echo.Context, entry *desktopAuthEntry, userID string) error {
	code := randomHex(32)
	rec := &desktopCodeEntry{UserID: userID, CodeChallenge: entry.CodeChallenge, RedirectURI: entry.RedirectURI}
	if err := sessionSet(c.Request().Context(), s.SessionStore, prefixDesktopCode, code, rec, desktopCodeTTL); err != nil {
		return serverErr(c, fmt.Errorf("store sign-in code: %w", err))
	}
	target, err := url.Parse(entry.RedirectURI)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "invalid redirect_uri")
	}
	q := target.Query()
	q.Set("code", code)
	q.Set("state", entry.ClientState)
	target.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, target.String())
}

// desktopTokenRequest is the body of POST /api/v1/auth/desktop/token.
type desktopTokenRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

// HandleDesktopTokenExchange redeems a one-time sign-in code. The code is
// consumed whether or not the rest of the request holds, so a code that was
// seen by anyone else is spent by the first attempt.
func (s *Server) HandleDesktopTokenExchange(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil || s.SessionStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	var req desktopTokenRequest
	if err := c.Bind(&req); err != nil || req.Code == "" || req.CodeVerifier == "" {
		return apiErr(c, http.StatusBadRequest, "code, code_verifier and redirect_uri required")
	}
	if err := checkDesktopRedirect(req.RedirectURI); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	invalid := func(why string) error {
		// The code and the verifier are secrets; only the reason is logged.
		slog.WarnContext(ctx, "sign-in code refused", "reason", why, "ip", c.RealIP())
		return apiErr(c, http.StatusBadRequest, "invalid_grant")
	}

	entry, err := sessionTake[desktopCodeEntry](ctx, s.SessionStore, prefixDesktopCode, req.Code)
	if errors.Is(err, ErrSessionNotFound) {
		return invalid("unknown, used or expired code")
	}
	if err != nil {
		return serverErr(c, fmt.Errorf("read sign-in code: %w", err))
	}
	if subtle.ConstantTimeCompare([]byte(entry.RedirectURI), []byte(req.RedirectURI)) != 1 {
		return invalid("redirect_uri does not match")
	}
	challenge := venueauth.ComputeCodeChallenge(req.CodeVerifier)
	if subtle.ConstantTimeCompare([]byte(challenge), []byte(entry.CodeChallenge)) != 1 {
		return invalid("code_verifier does not match")
	}

	user, err := s.AuthStore.GetUser(ctx, entry.UserID)
	if err != nil {
		return invalid("user not found")
	}
	token, refreshToken, err := s.issueSessionTokens(c, user)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, venueauth.TokenResponse{
		AccessToken:  token,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int((15 * time.Minute).Seconds()),
	})
}

// issueSessionTokens mints a 15-minute access token and a persisted 30-day
// refresh token for a user, as the desktop sign-in does.
func (s *Server) issueSessionTokens(c echo.Context, user *platformAuth.User) (string, string, error) {
	token, err := s.Services.Auth.GenerateToken(user, 15*time.Minute)
	if err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	refreshToken, err := platformAuth.GenerateRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	sum := sha256.Sum256([]byte(refreshToken))
	if _, err := s.AuthStore.StoreRefreshToken(c.Request().Context(), user.ID, hex.EncodeToString(sum[:]), time.Now().Add(30*24*time.Hour)); err != nil {
		// A refresh token that was not persisted could never be redeemed.
		return "", "", fmt.Errorf("store refresh token: %w", err)
	}
	return token, refreshToken, nil
}
