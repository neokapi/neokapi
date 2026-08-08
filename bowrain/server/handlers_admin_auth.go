package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// adminSessionCookieName is the HttpOnly cookie holding the admin control-plane
// session JWT (audience bowrain-admin). It is deliberately distinct from the
// user session cookie (bowrain_session) so admin and user identities never mix,
// and it is path-scoped to /api/admin so it is never sent to user routes.
const adminSessionCookieName = "bowrain_admin_session"

// adminSessionTTL is the lifetime of an admin session cookie. There is no admin
// refresh token: when it expires the ctrl SPA re-runs the admin OIDC flow, which
// is silent while the Keycloak SSO session is still alive. Admin sessions are
// deliberately short because the admin API includes impersonation and
// credit-grant endpoints.
const adminSessionTTL = 8 * time.Hour

// AdminExchangeRequest is the POST body for /api/admin/auth/exchange.
type AdminExchangeRequest struct {
	IDToken string `json:"id_token"`
}

// AdminIdentityResponse is returned by the exchange and /auth/me endpoints.
type AdminIdentityResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// HandleAdminAuthExchange verifies an admin-realm OIDC id_token (obtained by the
// ctrl SPA via browser PKCE) and, on success, mints a bowrain-admin session JWT
// and sets it as an HttpOnly cookie. This is the BFF boundary: the admin realm
// token is verified once server-side and never persisted in the browser;
// subsequent admin API calls authenticate with the HttpOnly cookie + CSRF
// header instead of a browser-held Bearer token.
//
// Authorization is unchanged from the Bearer model: holding a valid
// bowrain-admin realm token is the admin credential. Without an admin OIDC
// verifier the admin API is not mounted at all, so there is nothing to exchange.
func (s *Server) HandleAdminAuthExchange(c echo.Context) error {
	if s.AdminVerifier == nil || s.Config.JWTSecret == "" {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "admin auth not configured"})
	}

	var req AdminExchangeRequest
	if err := c.Bind(&req); err != nil || req.IDToken == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id_token required"})
	}

	ctx := c.Request().Context()
	idToken, err := s.AdminVerifier.Verify(ctx, req.IDToken)
	if err != nil {
		// The verifier's own words (issuer mismatch, key ids, clock skew) go to
		// the log, never to an unauthenticated caller.
		slog.WarnContext(ctx, "admin id_token verification failed", "error", err)
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid id_token"})
	}

	// Extract identity, mirroring AdminGuard's Bearer path (email falls back to
	// preferred_username) so the admin identity is identical however the request
	// authenticated.
	var claims struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return serverErr(c, fmt.Errorf("extract admin claims: %w", err))
	}
	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	adminJWT, err := platauth.GenerateAdminToken(idToken.Subject, email, claims.Name, s.Config.JWTSecret, adminSessionTTL)
	if err != nil {
		return serverErr(c, fmt.Errorf("mint admin session: %w", err))
	}

	s.setAdminSessionCookie(c, adminJWT)
	s.emitAuthEvent(c, platev.EventAuthLogin, idToken.Subject, claims.Name, "admin-oidc")

	return c.JSON(http.StatusOK, AdminIdentityResponse{Email: email, Name: claims.Name})
}

// HandleAdminAuthLogout clears the admin session cookie. The ctrl SPA then
// redirects the browser to the admin realm's end-session endpoint to terminate
// the Keycloak SSO session.
func (s *Server) HandleAdminAuthLogout(c echo.Context) error {
	s.clearAdminSessionCookie(c)
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// HandleAdminAuthMe returns the identity of the current admin session. It runs
// behind the admin auth middleware, so a valid admin cookie (or Bearer admin
// token) is required; the identity is read from the context the middleware set.
func (s *Server) HandleAdminAuthMe(c echo.Context) error {
	email, _ := c.Get("admin_email").(string)
	name, _ := c.Get("admin_name").(string)
	return c.JSON(http.StatusOK, AdminIdentityResponse{Email: email, Name: name})
}

// setAdminSessionCookie writes the admin session cookie, path-scoped to
// /api/admin so it is never sent to user (/api/v1) routes.
func (s *Server) setAdminSessionCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/api/admin",
		MaxAge:   int(adminSessionTTL / time.Second),
		HttpOnly: true,
		Secure:   s.cookieSecure(c),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearAdminSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/api/admin",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure(c),
		SameSite: http.SameSiteLaxMode,
	})
}

// validateAdminSessionCookie returns the claims from a valid admin session
// cookie, or nil when absent/invalid.
func (s *Server) validateAdminSessionCookie(c echo.Context) *platauth.Claims {
	cookie, err := c.Cookie(adminSessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	claims, err := platauth.ValidateAdminToken(cookie.Value, s.Config.JWTSecret)
	if err != nil {
		return nil
	}
	return claims
}

// adminAuthMiddleware accepts EITHER the admin session cookie (BFF path, with
// CSRF enforcement on state-changing requests) OR an admin-realm Bearer token
// (validated by the supplied AdminGuard). Keeping the Bearer fallback makes the
// cookie migration backward-compatible with any Bearer admin API client. The
// cookie path sets admin_email/admin_name on the context, matching AdminGuard.
func (s *Server) adminAuthMiddleware(guard echo.MiddlewareFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if claims := s.validateAdminSessionCookie(c); claims != nil {
				if err := enforceCSRFForCookie(c); err != nil {
					return err
				}
				c.Set("admin_email", claims.Email)
				c.Set("admin_name", claims.Name)
				return next(c)
			}
			return guard(next)(c)
		}
	}
}
