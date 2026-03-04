package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gokapi/gokapi/bowrain/auth"
	platauth "github.com/gokapi/gokapi/platform/auth"
	"github.com/labstack/echo/v4"
)

const sessionCookieName = "bowrain_session"

// validateSessionCookie extracts and validates a JWT from the bowrain_session cookie.
func validateSessionCookie(c echo.Context, jwtSecret string) *platauth.Claims {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	claims, err := platauth.ValidateToken(cookie.Value, jwtSecret)
	if err != nil {
		return nil
	}
	return claims
}

// setClaimsOnContext sets user claims on the Echo context for downstream handlers.
func setClaimsOnContext(c echo.Context, claims *platauth.Claims) {
	c.Set("user_id", claims.Subject)
	c.Set("email", claims.Email)
	c.Set("name", claims.Name)
}

// AuthMiddleware validates JWT tokens from the Authorization header (Bearer),
// API tokens (Bearer bwt_*), or the bowrain_session cookie and sets user
// claims on the Echo context.
func AuthMiddleware(jwtSecret string, authStore auth.AuthStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try Bearer header first.
			header := c.Request().Header.Get("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				token := strings.TrimPrefix(header, "Bearer ")

				// API token (bwt_ prefix).
				if strings.HasPrefix(token, "bwt_") && authStore != nil {
					return handleAPIToken(c, next, token, authStore)
				}

				// JWT token.
				claims, err := platauth.ValidateToken(token, jwtSecret)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid token: " + err.Error()})
				}
				setClaimsOnContext(c, claims)
				return next(c)
			}

			// Fall back to session cookie.
			claims := validateSessionCookie(c, jwtSecret)
			if claims != nil {
				setClaimsOnContext(c, claims)
				return next(c)
			}

			return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing authorization"})
		}
	}
}

// handleAPIToken validates a bwt_ API token, looks up the user, and sets context.
func handleAPIToken(c echo.Context, next echo.HandlerFunc, token string, authStore auth.AuthStore) error {
	ctx := c.Request().Context()

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	apiToken, err := authStore.GetAPITokenByHash(ctx, tokenHash)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid api token"})
	}

	if apiToken.ExpiresAt != nil && time.Now().After(*apiToken.ExpiresAt) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "api token expired"})
	}

	user, err := authStore.GetUser(ctx, apiToken.UserID)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "token user not found"})
	}

	c.Set("user_id", user.ID)
	c.Set("email", user.Email)
	c.Set("name", user.Name)
	c.Set("api_token_id", apiToken.ID)

	// Fire-and-forget last-used update.
	go func() {
		_ = authStore.UpdateAPITokenLastUsed(context.Background(), apiToken.ID)
	}()

	return next(c)
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
			if strings.HasPrefix(header, "Bearer ") {
				token := strings.TrimPrefix(header, "Bearer ")

				// API token (bwt_ prefix).
				if strings.HasPrefix(token, "bwt_") && authStore != nil {
					return handleAPIToken(c, next, token, authStore)
				}

				// JWT token.
				claims, err := platauth.ValidateToken(token, jwtSecret)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid token: " + err.Error()})
				}
				setClaimsOnContext(c, claims)
				return next(c)
			}

			// Fall back to session cookie.
			claims := validateSessionCookie(c, jwtSecret)
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
