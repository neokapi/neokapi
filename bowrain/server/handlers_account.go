// Package server — handlers_account.go
//
// Account-management handlers for the authenticated user: onboarding (handle
// + personal workspace), handle availability check, and Bowrain-managed
// email change with write-through to the upstream IdP (via the provider-neutral
// IdentityAdmin port — Keycloak Admin API or Cognito, per Config.AuthProvider).
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/mailer"
)

// onboardingResponse describes whether the calling user still needs to
// complete onboarding (pick a handle and create a personal workspace).
type onboardingResponse struct {
	NeedsOnboarding bool   `json:"needs_onboarding"`
	SuggestedSlug   string `json:"suggested_slug,omitempty"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name,omitempty"`
}

// HandleGetOnboarding returns whether the user still needs onboarding plus
// a suggested slug derived from their email.
func (s *Server) HandleGetOnboarding(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	ctx := c.Request().Context()
	u, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return apiErr(c, http.StatusNotFound, "user not found")
	}
	needs, err := s.Services.Auth.NeedsOnboarding(ctx, userID)
	if err != nil {
		return serverErr(c, err)
	}
	resp := onboardingResponse{
		NeedsOnboarding: needs,
		Email:           u.Email,
		DisplayName:     u.Name,
	}
	if needs {
		base := platauth.SuggestSlug(u.Email)
		if base != "" {
			suggested, err := s.Services.Auth.FindAvailableSlug(ctx, base)
			if err == nil {
				resp.SuggestedSlug = suggested
			} else {
				resp.SuggestedSlug = base
			}
		}
	}
	return c.JSON(http.StatusOK, resp)
}

// onboardingRequest is the body for POST /auth/me/onboarding.
type onboardingRequest struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

// HandleCompleteOnboarding finalizes onboarding: creates the personal
// workspace with the chosen slug and marks the user onboarded.
func (s *Server) HandleCompleteOnboarding(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}
	// Signups gate (ctrl-managed): closing signups blocks new users from
	// finalizing onboarding. Users who are already onboarded pass through
	// (CompleteOnboarding is idempotent) so a freeze never locks out existing
	// accounts.
	if !s.PlatformConfig.SignupsOpen() {
		if needs, err := s.Services.Auth.NeedsOnboarding(c.Request().Context(), userID); err != nil || needs {
			return apiErr(c, http.StatusForbidden, "signups are currently closed")
		}
	}

	var req onboardingRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	req.Slug = strings.TrimSpace(strings.ToLower(req.Slug))
	if req.Slug == "" {
		return apiErr(c, http.StatusBadRequest, "slug is required")
	}
	w, firstWorkspace, err := s.Services.Auth.CompleteOnboarding(c.Request().Context(), userID, req.Slug, strings.TrimSpace(req.DisplayName))
	if err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	// The personal workspace receives its one-time trial credit grant here —
	// the onboarding path does not go through HandleCreateWorkspace. Idempotent
	// (once per workspace, ever), so the idempotent re-onboarding case cannot
	// double-grant.
	billing.EnsureTrialGrant(c.Request().Context(), s.BillingStore, w.ID)
	s.greetNewAccount(c, userID, w, firstWorkspace)
	w.Role = platauth.RoleOwner
	return c.JSON(http.StatusOK, w)
}

// slugCheckResponse is the body for GET /auth/check-slug.
type slugCheckResponse struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// HandleCheckSlug reports whether a slug is available. Public — used by the
// onboarding form for live validation. Slug rules and reservations are
// enforced server-side; the client also runs format checks.
func (s *Server) HandleCheckSlug(c echo.Context) error {
	if s.AuthStore == nil || s.Services == nil || s.Services.Auth == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	slug := strings.TrimSpace(strings.ToLower(c.QueryParam("slug")))
	if slug == "" {
		return apiErr(c, http.StatusBadRequest, "slug is required")
	}
	avail, reason, err := s.Services.Auth.IsSlugAvailable(c.Request().Context(), slug)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, slugCheckResponse{Available: avail, Reason: reason})
}

// emailChangeRequest is the body for POST /auth/me/email.
type emailChangeRequest struct {
	NewEmail string `json:"new_email"`
}

// HandleRequestEmailChange initiates a Bowrain-managed email change.
//
// A verification token is generated and persisted as a hash; the plaintext
// token is mailed to the *new* address inside a confirmation link. Receiving
// and clicking that link is what proves the user controls the new mailbox.
func (s *Server) HandleRequestEmailChange(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	if s.Mailer == nil {
		return apiErr(c, http.StatusServiceUnavailable, "email is not configured on this server")
	}
	if s.IdentityAdmin == nil {
		return apiErr(c, http.StatusServiceUnavailable, "email change is unavailable (identity admin not configured)")
	}
	userID, ok := c.Get("user_id").(string)
	if !ok || userID == "" {
		return apiErr(c, http.StatusUnauthorized, "not authenticated")
	}

	var req emailChangeRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	newEmail := strings.TrimSpace(strings.ToLower(req.NewEmail))
	if !looksLikeEmail(newEmail) {
		return apiErr(c, http.StatusBadRequest, "new_email is not a valid email address")
	}

	ctx := c.Request().Context()

	// Block if the address is already in use by another account.
	if existing, err := s.AuthStore.GetUserByEmail(ctx, newEmail); err == nil && existing.ID != userID {
		return apiErr(c, http.StatusConflict, "that email is already in use")
	}

	// Same address? Nothing to do.
	current, err := s.AuthStore.GetUser(ctx, userID)
	if err != nil {
		return apiErr(c, http.StatusNotFound, "user not found")
	}
	if strings.EqualFold(strings.TrimSpace(current.Email), newEmail) {
		return apiErr(c, http.StatusBadRequest, "new email matches the current email")
	}

	// Generate token and persist hash.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return serverErr(c, fmt.Errorf("generate token: %w", err))
	}
	plaintext := "ec_" + hex.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	chReq := &platauth.EmailChangeRequest{UserID: userID, NewEmail: newEmail}
	if err := s.AuthStore.CreateEmailChangeRequest(ctx, chReq, tokenHash); err != nil {
		return serverErr(c, fmt.Errorf("persist request: %w", err))
	}

	confirmURL := s.confirmEmailURL(c, plaintext)
	// Render in the requesting user's locale (the recipient is that same
	// user's new address); English when unset.
	locale := ""
	if s.AuthStore != nil {
		if u, err := s.AuthStore.GetUser(ctx, userID); err == nil && u != nil {
			locale = u.Locale
		}
	}
	if err := s.Mailer.SendEmailChangeVerify(ctx, newEmail, locale, mailer.EmailChangeVerifyData{
		NewEmail:   newEmail,
		ConfirmURL: confirmURL,
		ExpiresIn:  "24 hours",
	}); err != nil {
		// Roll back persisted request so the user can retry without colliding.
		_ = s.AuthStore.DeleteEmailChangeRequestsForUser(ctx, userID)
		return serverErr(c, fmt.Errorf("send verification email: %w", err))
	}

	return c.JSON(http.StatusAccepted, map[string]any{
		"status":     "verification sent",
		"new_email":  newEmail,
		"expires_at": chReq.ExpiresAt,
	})
}

// emailConfirmRequest is the body for POST /auth/email/confirm.
type emailConfirmRequest struct {
	Token string `json:"token"`
}

// HandleConfirmEmailChange validates the verification token, writes the new
// email through to the upstream IdP via the IdentityAdmin port, updates the
// local users row, deletes any other pending requests for this user, and revokes
// refresh tokens so the user must sign in again with their new email.
//
// This endpoint is unauthenticated: the token alone authorizes the change.
func (s *Server) HandleConfirmEmailChange(c echo.Context) error {
	if s.AuthStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "auth not configured")
	}
	if s.IdentityAdmin == nil {
		return apiErr(c, http.StatusServiceUnavailable, "email change is unavailable (identity admin not configured)")
	}

	var req emailConfirmRequest
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Token) == "" {
		return apiErr(c, http.StatusBadRequest, "token is required")
	}

	hash := sha256.Sum256([]byte(req.Token))
	tokenHash := hex.EncodeToString(hash[:])

	ctx := c.Request().Context()
	pending, err := s.AuthStore.GetEmailChangeRequestByToken(ctx, tokenHash)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, "invalid or expired token")
	}
	if time.Now().After(pending.ExpiresAt) {
		_ = s.AuthStore.DeleteEmailChangeRequestsForUser(ctx, pending.UserID)
		return apiErr(c, http.StatusBadRequest, "token has expired")
	}

	user, err := s.AuthStore.GetUser(ctx, pending.UserID)
	if err != nil {
		return apiErr(c, http.StatusNotFound, "user not found")
	}

	// Recheck conflicts at confirmation time — a competing change may have
	// landed during the verification window.
	if existing, err := s.AuthStore.GetUserByEmail(ctx, pending.NewEmail); err == nil && existing.ID != user.ID {
		_ = s.AuthStore.DeleteEmailChangeRequestsForUser(ctx, user.ID)
		return apiErr(c, http.StatusConflict, "that email is already in use")
	}

	// Write through to the upstream IdP first; if that fails the local DB stays
	// authoritative-but-old, which is preferable to a divergence where the IdP
	// says one thing and Bowrain says another.
	if user.OIDCSub == "" {
		_ = s.AuthStore.DeleteEmailChangeRequestsForUser(ctx, user.ID)
		return apiErr(c, http.StatusConflict, "user has no identity-provider subject; cannot update upstream")
	}
	if err := s.IdentityAdmin.UpdateUserEmail(ctx, user.OIDCSub, pending.NewEmail); err != nil {
		slog.ErrorContext(ctx, "upstream identity email update failed", "user_id", user.ID, "error", err)
		return serverErrStatus(c, http.StatusBadGateway, fmt.Errorf("update upstream identity: %w", err))
	}

	user.Email = pending.NewEmail
	if err := s.AuthStore.UpdateUser(ctx, user); err != nil {
		// The IdP already changed; surface this as a server error so the
		// operator can reconcile. The next OIDC login will re-sync.
		slog.ErrorContext(ctx, "local email update failed after upstream identity update", "user_id", user.ID, "error", err)
		return serverErr(c, fmt.Errorf("local update failed; sign in again to refresh: %w", err))
	}

	_ = s.AuthStore.DeleteEmailChangeRequestsForUser(ctx, user.ID)
	if err := s.AuthStore.RevokeUserRefreshTokens(ctx, user.ID); err != nil {
		slog.WarnContext(ctx, "revoke refresh tokens after email change", "user_id", user.ID, "error", err)
	}
	// Drop any elevated credential-management token too: the identity just
	// changed and all sessions are being invalidated, so re-elevation is required.
	s.clearElevatedToken(ctx, user.ID)
	s.clearSessionCookies(c)

	return c.JSON(http.StatusOK, map[string]any{
		"status":    "email updated",
		"new_email": pending.NewEmail,
	})
}

// confirmEmailURL builds the URL the user clicks in the verification mail.
// Defaults to the request scheme+host; overridable via PUBLIC_BASE_URL when
// the server lives behind a different ingress hostname.
func (s *Server) confirmEmailURL(c echo.Context, token string) string {
	base := strings.TrimRight(s.Config.OIDCPublicURL, "/")
	if base == "" {
		req := c.Request()
		scheme := "https"
		if req.TLS == nil {
			if forwarded := req.Header.Get("X-Forwarded-Proto"); forwarded != "" {
				scheme = forwarded
			} else {
				scheme = "http"
			}
		}
		base = scheme + "://" + req.Host
	}
	// Drop any /realms/... path that OIDCPublicURL might carry.
	if i := strings.Index(base, "/realms/"); i >= 0 {
		base = base[:i]
	}
	return base + "/account/confirm-email?token=" + token
}

// looksLikeEmail is a permissive check (RFC-5321/5322 are not worth
// implementing here; the upstream IdP rejects malformed addresses on update).
func looksLikeEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at >= len(s)-1 {
		return false
	}
	if strings.IndexByte(s[at+1:], '.') < 0 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n,;<>") {
		return false
	}
	return true
}
