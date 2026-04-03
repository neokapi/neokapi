package billing

import (
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"
)

// contextKeyWorkspacePlan is the echo context key for the workspace plan.
const contextKeyWorkspacePlan = "workspace_plan"

// PlanGuard returns Echo middleware that rejects requests when the workspace
// plan does not include the required feature. It reads the plan from
// the echo context (set by workspace access middleware) and checks overrides.
// When billing is not configured (plan is empty), all features are allowed.
func PlanGuard(feature Feature, onBlock ...GuardEventFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			planStr, _ := c.Get(contextKeyWorkspacePlan).(string)
			// When billing is not configured or plan is not set, allow all.
			if planStr == "" {
				return next(c)
			}

			plan := Plan(planStr)

			// Check overrides if available on context.
			var overrides map[Feature]bool
			if o, ok := c.Get("feature_overrides").(map[Feature]bool); ok {
				overrides = o
			}

			if HasFeature(plan, feature, overrides) {
				return next(c)
			}

			if len(onBlock) > 0 && onBlock[0] != nil {
				wsID, _ := c.Get("workspace_id").(string)
				onBlock[0]("billing.feature_gate_hit", wsID, map[string]any{
					"feature":      string(feature),
					"plan":         planStr,
					"minimum_plan": string(MinimumPlanFor(feature)),
				})
			}

			return c.JSON(http.StatusForbidden, map[string]any{
				"error":        "upgrade_required",
				"feature":      feature,
				"minimum_plan": MinimumPlanFor(feature),
			})
		}
	}
}

// GuardEventFunc is an optional callback fired when PlanGuard or QuotaGuard
// blocks a request. Used for analytics (e.g. PostHog).
type GuardEventFunc func(event string, workspaceID string, props map[string]any)

// QuotaGuard returns Echo middleware that rejects requests when weekly credits
// are exhausted. Returns 429 with Retry-After header set to next Monday 00:00 UTC.
// When billing is not configured (store is nil or plan is empty), all requests pass.
func QuotaGuard(store BillingStore, onBlock ...GuardEventFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// When billing is not configured, allow all.
			if store == nil {
				return next(c)
			}

			planStr, _ := c.Get(contextKeyWorkspacePlan).(string)
			if planStr == "" {
				return next(c)
			}

			plan := Plan(planStr)

			// Enterprise plans have unlimited credits.
			if plan == PlanEnterprise {
				return next(c)
			}

			workspaceID, _ := c.Get("workspace_id").(string)
			if workspaceID == "" {
				return next(c) // no workspace context, skip check
			}

			remaining, err := store.CheckCredits(c.Request().Context(), workspaceID)
			if err != nil {
				// If we can't check credits (e.g., no allocation yet), allow the request.
				return next(c)
			}

			if remaining <= 0 {
				retryAfter := WeekEnd(time.Now().UTC())
				c.Response().Header().Set("Retry-After", retryAfter.Format(time.RFC1123))
				if len(onBlock) > 0 && onBlock[0] != nil {
					onBlock[0]("billing.credits_exhausted", workspaceID, map[string]any{
						"plan": string(plan),
						"path": c.Path(),
					})
				}
				return c.JSON(http.StatusTooManyRequests, map[string]any{
					"error":       "credits_exhausted",
					"resets_at":   retryAfter.Format(time.RFC3339),
					"retry_after": int(time.Until(retryAfter).Seconds()),
				})
			}

			return next(c)
		}
	}
}

// AdminGuard returns Echo middleware that verifies the JWT was issued by the
// admin Keycloak realm. It accepts both access tokens and ID tokens.
// Two verifiers are needed because Keycloak access tokens use aud="account"
// while ID tokens use aud=<client_id>.
// It sets "admin_email" and "admin_name" on the context.
func AdminGuard(idTokenVerifier, accessTokenVerifier *oidc.IDTokenVerifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractBearerToken(c)
			if token == "" {
				return echo.ErrUnauthorized
			}

			ctx := c.Request().Context()

			// Try ID token verifier first (audience = client ID).
			idToken, err := idTokenVerifier.Verify(ctx, token)
			if err != nil {
				// Fall back to access token verifier (skips audience check).
				idToken, err = accessTokenVerifier.Verify(ctx, token)
				if err != nil {
					return echo.ErrUnauthorized
				}
			}

			var claims struct {
				Email             string `json:"email"`
				Name              string `json:"name"`
				PreferredUsername string `json:"preferred_username"`
			}
			if err := idToken.Claims(&claims); err != nil {
				return echo.ErrUnauthorized
			}

			email := claims.Email
			if email == "" {
				email = claims.PreferredUsername
			}

			c.Set("admin_email", email)
			c.Set("admin_name", claims.Name)
			return next(c)
		}
	}
}

// extractBearerToken extracts a bearer token from the Authorization header.
func extractBearerToken(c echo.Context) string {
	auth := c.Request().Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}
