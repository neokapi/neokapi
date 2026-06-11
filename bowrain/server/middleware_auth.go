package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

const sessionCookieName = "bowrain_session"

// errAccessDenied is returned by the permission/role gate helpers AFTER they
// write the 403 response. Returning a non-nil error is essential: echo's
// c.JSON returns nil on success, so a helper that only did `return c.JSON(403)`
// would let the caller's `if err != nil { return err }` fall through and the
// handler would proceed with the mutation despite the 403 (a fail-open). The
// caller returns this error to stop; echo's error handler no-ops because the
// response is already committed.
var errAccessDenied = errors.New("access denied")

// deny writes a 403 with the given message and returns errAccessDenied so the
// calling handler aborts.
func deny(c echo.Context, msg string) error {
	_ = c.JSON(http.StatusForbidden, ErrorResponse{Error: msg})
	return errAccessDenied
}

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
	// Propagate actor + request metadata into the request context so the
	// EventEmittingStore (and handler-emitted audit events) can attribute
	// events to the authenticated user and record where they came from.
	meta := requestMeta(c)
	ctx := platev.WithActor(c.Request().Context(), claims.Subject, claims.Name)
	ctx = platev.WithRequestMeta(ctx, meta)
	// Attribution for content writes (block_history / change_log). The
	// request id is the default correlation id; push/merge handlers override it.
	ctx = bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: claims.Subject, CorrelationID: meta.RequestID})
	c.SetRequest(c.Request().WithContext(ctx))
}

// requestMeta extracts audit-relevant request metadata from the echo context.
func requestMeta(c echo.Context) platev.RequestMeta {
	reqID, _ := c.Get("request_id").(string)
	if reqID == "" {
		reqID = c.Response().Header().Get(echo.HeaderXRequestID)
	}
	return platev.RequestMeta{
		RequestID: reqID,
		IP:        c.RealIP(),
		UserAgent: c.Request().UserAgent(),
	}
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
	ctx = platev.WithRequestMeta(ctx, requestMeta(c))
	ctx = bstore.WithChangeContext(ctx, bstore.ChangeContext{Actor: user.ID, CorrelationID: requestMeta(c).RequestID})
	c.SetRequest(c.Request().WithContext(ctx))

	// Fire-and-forget last-used update.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in API token last-used update", "panic", r)
			}
		}()
		if err := authStore.UpdateAPITokenLastUsed(context.WithoutCancel(ctx), apiToken.ID); err != nil {
			slog.Debug("update API token last-used", "token_id", apiToken.ID, "error", err)
		}
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

			// Reject malformed or reserved slugs up front. This catches stale
			// client calls to removed top-level routes (e.g. /api/v1/config)
			// that would otherwise be swallowed by the /:ws catch-all and
			// surface as a confusing "workspace not found".
			if err := ValidateWorkspaceSlug(wsSlug); err != nil {
				return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no such endpoint"})
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

// WeeklyAllocationMiddleware lazily ensures a weekly credit allocation exists
// for the current workspace. It reads the plan from context (set by
// WorkspaceAccessMiddleware) and calls EnsureWeeklyAllocation once per week.
// When billing is not configured (store is nil or plan is empty), this is a no-op.
func WeeklyAllocationMiddleware(store billing.BillingStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if store == nil {
				return next(c)
			}
			wsID, _ := c.Get("workspace_id").(string)
			planStr, _ := c.Get("workspace_plan").(string)
			if wsID == "" || planStr == "" {
				return next(c)
			}
			// Fire and forget — allocation is idempotent and fast.
			_, _ = billing.EnsureWeeklyAllocation(c.Request().Context(), store, wsID, billing.Plan(planStr))
			return next(c)
		}
	}
}

// requireRole verifies that the authenticated user has one of the allowed roles
// in the current workspace. Returns an echo error response on failure, nil on success.
func (s *Server) requireRole(c echo.Context, allowed ...platauth.Role) error {
	role, ok := c.Get("workspace_role").(platauth.Role)
	if !ok {
		return deny(c, "workspace role not available")
	}
	for _, r := range allowed {
		if role == r {
			return nil
		}
	}
	if actor, _ := c.Get("user_id").(string); actor != "" {
		s.emitAudit(c, auditEvent{
			Type:   platev.EventAuthzDenied,
			Effect: "deny",
			Data:   map[string]string{"reason": "insufficient_role", "role": string(role), "path": c.Path()},
		})
	}
	return deny(c, "insufficient permissions")
}

// ProjectAccessMiddleware resolves the caller's effective permissions for the
// current request and sets them on the context as "project_permissions" (and
// "project_languages"). It resolves, in priority order:
//
//  1. Claim-token sync (pre-membership bootstrap): the claim token authorizes
//     full access to exactly the claimed project.
//  2. Project membership: an explicit project_members role template.
//  3. Workspace-role fallback: the user's workspace role default permissions —
//     used both for workspace-level resources (no project in the path) and for
//     project routes where the user has no explicit project membership.
//
// It always sets a permission context for an authenticated request, so the
// fail-closed requirePermission helper can enforce uniformly. Routes that need
// the workspace-role fallback but run outside WorkspaceAccessMiddleware (e.g.
// the flat sync routes) are resolved via the project's workspace.
func (s *Server) ProjectAccessMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1. Claim-token sync: full access to the single claimed project.
			if claimPID, _ := c.Get("claim_project_id").(string); claimPID != "" {
				c.Set("project_permissions", platauth.PermAll)
				return next(c)
			}

			if s.AuthStore == nil {
				return next(c)
			}

			userID, _ := c.Get("user_id").(string)
			if userID == "" {
				return next(c) // not authenticated; AuthMiddleware handles rejection
			}

			ctx := c.Request().Context()

			projectID := projectParam(c)
			if projectID == "" {
				projectID = c.Param("id")
			}

			var resolved *platauth.ResolvedPermission
			if projectID == "" {
				// Workspace-level resource (no project in path): resolve from the
				// user's workspace role.
				resolved = s.workspaceRoleFallback(c, "", userID)
			} else {
				// 2. Explicit project membership (or group binding).
				var err error
				resolved, err = s.AuthStore.ResolveProjectPermissions(ctx, projectID, userID)
				if err != nil {
					// 3. Fall back to the user's workspace role for this project.
					resolved = s.workspaceRoleFallback(c, projectID, userID)
				}
			}

			perms := resolved.Permissions

			// 4. Subtract deny rules (negative permissions always win). Applied
			// when the workspace context is known (the workspace route group).
			if wsID, _ := c.Get("workspace_id").(string); wsID != "" {
				wsRole, _ := c.Get("workspace_role").(platauth.Role)
				if denied, derr := s.AuthStore.ResolveDenies(ctx, wsID, projectID, userID, wsRole); derr == nil {
					perms &^= denied
				}
			}

			c.Set("project_permissions", perms)
			c.Set("project_languages", resolved.Languages)

			// Enrich the change context with the actor's workspace role so
			// block_history records who-with-what-role made each edit.
			if wsRole, _ := c.Get("workspace_role").(platauth.Role); wsRole != "" {
				cctx := bstore.WithChangeContext(c.Request().Context(), bstore.ChangeContext{ActorRole: string(wsRole)})
				c.SetRequest(c.Request().WithContext(cctx))
			}
			return next(c)
		}
	}
}

// workspaceRoleFallback resolves a user's permissions from their workspace role,
// honoring any per-workspace role override. It prefers a workspace_role already
// on the context (set by WorkspaceAccessMiddleware); otherwise — for routes that
// run outside the workspace group, such as the flat sync routes — it looks up
// the project's workspace and the user's membership there. Returns zero
// permissions when the user has no resolvable workspace membership (deny).
func (s *Server) workspaceRoleFallback(c echo.Context, projectID, userID string) *platauth.ResolvedPermission {
	ctx := c.Request().Context()

	wsRole, _ := c.Get("workspace_role").(platauth.Role)
	wsID, _ := c.Get("workspace_id").(string)

	if wsRole == "" {
		// Resolve via the project's workspace (flat sync routes have no
		// workspace context on the request).
		if projectID != "" && s.ContentStore != nil {
			if proj, err := s.ContentStore.GetProject(ctx, projectID); err == nil && proj != nil && proj.WorkspaceID != "" {
				wsID = proj.WorkspaceID
				if m, err := s.AuthStore.GetMembership(ctx, proj.WorkspaceID, userID); err == nil && m != nil {
					wsRole = m.Role
				}
			}
		}
	}

	if wsRole == "" {
		// No resolvable workspace membership — deny by default.
		return &platauth.ResolvedPermission{}
	}

	// Honor a per-workspace override of the role's default permissions.
	if wsID != "" {
		if perms, ok, err := s.AuthStore.GetWorkspaceRoleOverride(ctx, wsID, wsRole); err == nil && ok {
			return &platauth.ResolvedPermission{Permissions: perms}
		}
	}
	return platauth.DefaultPermissionsForRole(wsRole)
}

// requirePermission verifies that the user has the required permission in the
// current project (or workspace-resource) context. Returns an echo error
// response on failure, nil on success.
//
// Fail-closed: if no permission context was resolved on the request, access is
// denied. Every authenticated route that calls this passes through
// ProjectAccessMiddleware, which always resolves a permission set (project
// membership, workspace-role fallback, or claim-token grant). A missing context
// therefore means the caller is unauthenticated or the route is misconfigured —
// either way, deny rather than silently allow.
func (s *Server) requirePermission(c echo.Context, perm platauth.Permission) error {
	perms, ok := c.Get("project_permissions").(platauth.Permission)
	if !ok {
		s.emitAuthzDenied(c, perm, "no_permission_context")
		return deny(c, "permission context not resolved")
	}
	if !perms.Has(perm) {
		s.emitAuthzDenied(c, perm, "insufficient_permissions")
		return deny(c, "insufficient project permissions")
	}
	return nil
}

// emitAuthzDenied records an authorization denial for an authenticated caller.
// Anonymous/unauthenticated probes are not recorded (no actor to attribute).
func (s *Server) emitAuthzDenied(c echo.Context, perm platauth.Permission, reason string) {
	if actor, _ := c.Get("user_id").(string); actor == "" {
		return
	}
	projectID := projectParam(c)
	if projectID == "" {
		projectID = c.Param("id")
	}
	s.emitAudit(c, auditEvent{
		Type:      platev.EventAuthzDenied,
		ProjectID: projectID,
		Effect:    "deny",
		Data: map[string]string{
			"required_permission": perm.String(),
			"reason":              reason,
			"path":                c.Path(),
		},
	})
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
	return deny(c, "no access to language: "+locale)
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

			// Enforce the grant's project constraint: if the grant is scoped to
			// specific projects and this request targets a project outside that
			// set, drop all permissions so requirePermission denies.
			if len(grant.ProjectIDs) > 0 {
				reqProject := projectParam(c)
				if reqProject == "" {
					reqProject = c.Param("id")
				}
				if reqProject != "" && !containsString(grant.ProjectIDs, reqProject) {
					c.Set("project_permissions", platauth.Permission(0))
					c.Set("bravo_mode", string(grant.Mode))
					return next(c)
				}
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

// containsString reports whether s is present in the slice.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
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
