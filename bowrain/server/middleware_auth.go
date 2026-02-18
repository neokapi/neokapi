package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/labstack/echo/v4"
)

const sessionCookieName = "bowrain_session"

// validateBearerToken extracts and validates a JWT from a "Bearer <token>" Authorization header.
// Returns the validated claims or nil if the header is absent or not a Bearer token.
// If the header is present but the token is invalid, it returns an error string.
func validateBearerToken(c echo.Context, jwtSecret string) (*auth.Claims, string) {
	header := c.Request().Header.Get("Authorization")
	if header == "" || !strings.HasPrefix(header, "Bearer ") {
		return nil, ""
	}
	token := strings.TrimPrefix(header, "Bearer ")
	claims, err := auth.ValidateToken(token, jwtSecret)
	if err != nil {
		return nil, "invalid token: " + err.Error()
	}
	return claims, ""
}

// validateSessionCookie extracts and validates a JWT from the bowrain_session cookie.
func validateSessionCookie(c echo.Context, jwtSecret string) *auth.Claims {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	claims, err := auth.ValidateToken(cookie.Value, jwtSecret)
	if err != nil {
		return nil
	}
	return claims
}

// setClaimsOnContext sets user claims on the Echo context for downstream handlers.
func setClaimsOnContext(c echo.Context, claims *auth.Claims) {
	c.Set("user_id", claims.Subject)
	c.Set("email", claims.Email)
	c.Set("name", claims.Name)
}

// AuthMiddleware validates JWT tokens from the Authorization header (Bearer)
// or the bowrain_session cookie and sets user claims on the Echo context.
func AuthMiddleware(jwtSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try Bearer header first.
			claims, errMsg := validateBearerToken(c, jwtSecret)
			if errMsg != "" {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errMsg})
			}
			if claims != nil {
				setClaimsOnContext(c, claims)
				return next(c)
			}

			// Fall back to session cookie.
			claims = validateSessionCookie(c, jwtSecret)
			if claims != nil {
				setClaimsOnContext(c, claims)
				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing authorization"})
		}
	}
}

// ClaimOrAuthMiddleware accepts either a JWT (Bearer), a ClaimToken for sync routes,
// or a session cookie. With a ClaimToken, access is restricted to only the matching project.
func ClaimOrAuthMiddleware(jwtSecret string, authStore auth.AuthStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")

			// Try ClaimToken first (only from header).
			if strings.HasPrefix(header, "ClaimToken ") {
				token := strings.TrimPrefix(header, "ClaimToken ")
				hash := sha256.Sum256([]byte(token))
				tokenHash := hex.EncodeToString(hash[:])

				unclaimed, err := authStore.GetUnclaimedByToken(c.Request().Context(), tokenHash)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid claim token"})
				}

				// Restrict to matching project.
				projectID := c.Param("id")
				if projectID != "" && projectID != unclaimed.ProjectID {
					return c.JSON(http.StatusForbidden, ErrorResponse{Error: "claim token does not match project"})
				}

				c.Set("claim_project_id", unclaimed.ProjectID)
				return next(c)
			}

			// Try Bearer header.
			claims, errMsg := validateBearerToken(c, jwtSecret)
			if errMsg != "" {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errMsg})
			}
			if claims != nil {
				setClaimsOnContext(c, claims)
				return next(c)
			}

			// Fall back to session cookie.
			claims = validateSessionCookie(c, jwtSecret)
			if claims != nil {
				setClaimsOnContext(c, claims)
				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing authorization"})
		}
	}
}

// WorkspaceAccessMiddleware checks that the authenticated user is a member
// of the workspace identified by the :ws path parameter.
func WorkspaceAccessMiddleware(authStore auth.AuthStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userID, ok := c.Get("user_id").(string)
			if !ok || userID == "" {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "not authenticated"})
			}

			wsSlug := c.Param("ws")
			if wsSlug == "" {
				return next(c)
			}

			ctx := c.Request().Context()
			w, err := authStore.GetWorkspaceBySlug(ctx, wsSlug)
			if err != nil {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "workspace not found"})
			}

			_, err = authStore.GetMembership(ctx, w.ID, userID)
			if err != nil {
				return c.JSON(http.StatusForbidden, ErrorResponse{Error: "not a member of this workspace"})
			}

			// Store workspace ID on context for downstream handlers.
			c.Set("workspace_id", w.ID)
			return next(c)
		}
	}
}
