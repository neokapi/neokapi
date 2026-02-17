package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/labstack/echo/v4"
)

// AuthMiddleware validates JWT tokens from the Authorization header
// and sets user claims on the Echo context.
func AuthMiddleware(jwtSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing authorization header"})
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid authorization header format"})
			}

			claims, err := auth.ValidateToken(token, jwtSecret)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid token: " + err.Error()})
			}

			c.Set("user_id", claims.Subject)
			c.Set("email", claims.Email)
			c.Set("name", claims.Name)
			return next(c)
		}
	}
}

// ClaimOrAuthMiddleware accepts either a JWT (Bearer) or a ClaimToken for sync routes.
// With a ClaimToken, access is restricted to only the matching project.
func ClaimOrAuthMiddleware(jwtSecret string, authStore auth.AuthStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if header == "" {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing authorization header"})
			}

			// Try ClaimToken first.
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

			// Fall back to JWT.
			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid authorization header format"})
			}

			claims, err := auth.ValidateToken(token, jwtSecret)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid token: " + err.Error()})
			}

			c.Set("user_id", claims.Subject)
			c.Set("email", claims.Email)
			c.Set("name", claims.Name)
			return next(c)
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
