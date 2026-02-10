package server

import (
	"net/http"
	"strings"

	"github.com/gokapi/gokapi/core/auth"
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
