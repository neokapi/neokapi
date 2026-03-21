package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/platform/auth"
	platev "github.com/neokapi/neokapi/platform/event"
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

// setClaimsOnContext sets user claims on both the Echo context and the request
// context (for actor attribution in events emitted by the ContentStore).
func setClaimsOnContext(c echo.Context, claims *platauth.Claims) {
	c.Set("user_id", claims.Subject)
	c.Set("email", claims.Email)
	c.Set("name", claims.Name)
	// Propagate actor into the request context so the EventEmittingStore can
	// attribute events to the authenticated user.
	ctx := platev.WithActor(c.Request().Context(), claims.Subject, claims.Name)
	c.SetRequest(c.Request().WithContext(ctx))
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
				// Auto-provision user if not in DB (supports pre-generated agent tokens).
				if authStore != nil {
					ensureUserExists(c.Request().Context(), authStore, claims)
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

// ensureUserExists creates a user record if it doesn't already exist.
// This supports agent tokens where the user ID is embedded in the JWT but
// the user hasn't gone through the OIDC registration flow.
func ensureUserExists(ctx context.Context, authStore auth.AuthStore, claims *platauth.Claims) {
	_, err := authStore.GetUser(ctx, claims.Subject)
	if err == nil {
		return // already exists
	}
	u := &platauth.User{
		ID:    claims.Subject,
		Email: claims.Email,
		Name:  claims.Name,
	}
	_ = authStore.CreateUser(ctx, u) // best-effort; ignore duplicate race
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
	c.Set("api_token_scopes", apiToken.Scopes)
	// Propagate actor into the request context for event attribution.
	ctx = platev.WithActor(ctx, user.ID, user.Name)
	c.SetRequest(c.Request().WithContext(ctx))

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

			m, err := authStore.GetMembership(ctx, w.ID, userID)
			if err != nil {
				return c.JSON(http.StatusForbidden, ErrorResponse{Error: "not a member of this workspace"})
			}

			// Store workspace ID, plan, and user role on context for downstream handlers.
			c.Set("workspace_id", w.ID)
			c.Set("workspace_role", m.Role)
			c.Set("workspace_plan", w.Plan)
			c.Set("workspace_stripe_customer_id", w.StripeCustomerID)
			return next(c)
		}
	}
}

// requireRole verifies that the authenticated user has one of the allowed roles
// in the current workspace. Returns an echo error response on failure, nil on success.
func (s *Server) requireRole(c echo.Context, allowed ...platauth.Role) error {
	role, ok := c.Get("workspace_role").(platauth.Role)
	if !ok {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "workspace role not available"})
	}
	for _, r := range allowed {
		if role == r {
			return nil
		}
	}
	return c.JSON(http.StatusForbidden, ErrorResponse{Error: "insufficient permissions"})
}

// ProjectAccessMiddleware resolves project-level permissions for the authenticated user.
// It checks the project_members table for an explicit membership and falls back to
// default permissions based on the user's workspace role.
func ProjectAccessMiddleware(authStore auth.AuthStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract project ID from either :pid or :id parameter.
			projectID := c.Param("pid")
			if projectID == "" {
				projectID = c.Param("id")
			}
			if projectID == "" {
				return next(c) // no project context, skip
			}

			userID, _ := c.Get("user_id").(string)
			if userID == "" {
				return next(c) // not authenticated, let auth middleware handle it
			}

			ctx := c.Request().Context()
			resolved, err := authStore.ResolveProjectPermissions(ctx, projectID, userID)
			if err != nil {
				// No explicit project membership — fall back to workspace role defaults.
				wsRole, _ := c.Get("workspace_role").(platauth.Role)
				resolved = platauth.DefaultPermissionsForRole(wsRole)
			}

			c.Set("project_permissions", resolved.Permissions)
			c.Set("project_languages", resolved.Languages)
			return next(c)
		}
	}
}

// requirePermission verifies that the user has the required permission in the
// current project context. Returns an echo error response on failure, nil on success.
func (s *Server) requirePermission(c echo.Context, perm platauth.Permission) error {
	perms, ok := c.Get("project_permissions").(platauth.Permission)
	if !ok {
		// No project permissions on context — the request came through a route
		// without ProjectAccessMiddleware (e.g., legacy sync routes with
		// ClaimOrAuthMiddleware). Skip the check; those routes have their own auth.
		return nil
	}
	if !perms.Has(perm) {
		return c.JSON(http.StatusForbidden, ErrorResponse{Error: "insufficient project permissions"})
	}
	return nil
}

// requireLanguagePermission verifies both the permission and language access.
// Use for language-scoped operations like translation and review.
func (s *Server) requireLanguagePermission(c echo.Context, perm platauth.Permission, locale string) error {
	if err := s.requirePermission(c, perm); err != nil {
		return err
	}
	languages, _ := c.Get("project_languages").([]string)
	if len(languages) == 0 {
		return nil // all languages allowed
	}
	for _, l := range languages {
		if l == locale {
			return nil
		}
	}
	return c.JSON(http.StatusForbidden, ErrorResponse{Error: "no access to language: " + locale})
}

// ScopeRestrictionMiddleware narrows project_permissions based on API token scopes.
// Only applies when the request is authenticated via an API token (api_token_id on context).
// Parses the token's scopes and intersects them with the already-resolved project permissions.
func ScopeRestrictionMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scopesJSON, _ := c.Get("api_token_scopes").(string)
			if scopesJSON == "" {
				return next(c) // not an API token request or no scopes set
			}

			perms, ok := c.Get("project_permissions").(platauth.Permission)
			if !ok {
				return next(c) // no project permissions to restrict
			}

			resolved, err := platauth.ParseScopes(scopesJSON)
			if err != nil || resolved.IsFullAccess {
				return next(c) // "*" scope or parse error — no restriction
			}

			// Intersect permissions.
			c.Set("project_permissions", perms&resolved.Permissions)

			// Intersect languages if scopes restrict them.
			if len(resolved.Languages) > 0 {
				existing, _ := c.Get("project_languages").([]string)
				if len(existing) == 0 {
					c.Set("project_languages", resolved.Languages)
				} else {
					c.Set("project_languages", intersectStrings(existing, resolved.Languages))
				}
			}

			return next(c)
		}
	}
}

// SessionGrantMiddleware narrows project_permissions based on an active session grant
// (used by @bravo conversations and MCP tool sessions). Only applies when
// bravo_session_id is present on the echo context.
func SessionGrantMiddleware(stateStore SessionStateStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionID, _ := c.Get("bravo_session_id").(string)
			if sessionID == "" {
				return next(c)
			}

			grant, err := GetSessionGrant(c.Request().Context(), stateStore, sessionID)
			if err != nil || grant == nil {
				return next(c) // no grant found — no restriction
			}

			// Intersect permissions with grant ceiling.
			perms, ok := c.Get("project_permissions").(platauth.Permission)
			if ok {
				c.Set("project_permissions", perms&grant.Permissions)
			}

			// Intersect languages.
			if len(grant.Languages) > 0 {
				existing, _ := c.Get("project_languages").([]string)
				if len(existing) == 0 {
					c.Set("project_languages", grant.Languages)
				} else {
					c.Set("project_languages", intersectStrings(existing, grant.Languages))
				}
			}

			// Set mode on context for downstream use.
			c.Set("bravo_mode", string(grant.Mode))

			return next(c)
		}
	}
}

// intersectStrings returns elements present in both slices.
func intersectStrings(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	var result []string
	for _, s := range a {
		if set[s] {
			result = append(result, s)
		}
	}
	return result
}
