package server

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
)

func pulseWorkspaceVisibility(ws *platauth.Workspace) platauth.DashboardVisibility {
	if ws.Type != platauth.WorkspaceTypeTeam || ws.DashboardVisibility == "" {
		return platauth.DashboardPrivate
	}
	return ws.DashboardVisibility
}

func pulseProjectVisible(p *store.Project, workspaceID string) bool {
	return p != nil && p.WorkspaceID == workspaceID && !p.Archived &&
		(p.DashboardVisibility == "public" || p.DashboardVisibility == "unlisted")
}

// PulseAccessMiddleware resolves the workspace by slug from the :workspace
// URL parameter and enforces its dashboard visibility.
//
// On success the middleware stores "pulse_workspace" on the echo context.
func PulseAccessMiddleware(jwtSecret string, authStore auth.AuthStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			param := c.Param("workspace")
			if param == "" {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
			}

			ctx := c.Request().Context()
			// Try slug first, then fall back to access key (for unlisted dashboards).
			ws, err := authStore.GetWorkspaceBySlug(ctx, param)
			if err != nil {
				ws, err = authStore.GetWorkspaceByAccessKey(ctx, param)
			}
			if err != nil || ws == nil {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
			}

			switch pulseWorkspaceVisibility(ws) {
			case platauth.DashboardPublic:
				// Fully accessible, indexed.
			case platauth.DashboardUnlisted:
				// Accessible but not indexed.
				c.Response().Header().Set("X-Robots-Tag", "noindex")
			case platauth.DashboardPrivate:
				// Only workspace members with valid auth.
				if !pulseAuthCheck(c, jwtSecret, authStore, ws.ID) {
					return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
				}
			default:
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
			}

			c.Set("pulse_workspace", ws)
			c.Set("pulse_workspace_id", ws.ID)
			return next(c)
		}
	}
}

// PulseProjectAccessMiddleware checks the project's own dashboard_visibility.
// Must run after PulseAccessMiddleware so "pulse_workspace" is on the context.
// Returns 404 for private projects (prevents enumeration).
func PulseProjectAccessMiddleware(cs store.ContentStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			pid := projectParam(c)
			if pid == "" {
				return next(c)
			}

			ws := pulseWorkspace(c)
			if ws == nil {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
			}

			ctx := c.Request().Context()
			p, err := cs.GetProject(ctx, pid)
			if err != nil || !pulseProjectVisible(p, ws.ID) {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "not found"})
			}

			c.Set("pulse_project", p)
			return next(c)
		}
	}
}

// pulseAuthCheck attempts to validate JWT from Authorization header or session
// cookie and verify workspace membership. Returns true if the user is a member.
func pulseAuthCheck(c *echo.Context, jwtSecret string, authStore auth.AuthStore, workspaceID string) bool {
	var claims *platauth.Claims

	// Try Bearer token.
	header := c.Request().Header.Get("Authorization")
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		token := after
		if parsed, err := platauth.ValidateToken(token, jwtSecret); err == nil {
			claims = parsed
		}
	}

	// Try session cookie.
	if claims == nil {
		claims = validateSessionCookie(c, jwtSecret)
	}

	if claims == nil {
		return false
	}

	// Check workspace membership.
	ctx := c.Request().Context()
	membership, err := authStore.GetMembership(ctx, workspaceID, claims.Subject)
	return err == nil && membership != nil
}
