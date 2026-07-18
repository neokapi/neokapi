package billing

import (
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/apierror"
)

// contextKeyWorkspacePlan is the echo context key for the workspace plan.
const contextKeyWorkspacePlan = "workspace_plan"

// ContextKeyFeatureOverrides is the echo context key under which the resolved
// per-workspace feature overrides (map[Feature]bool) are stored. It is populated
// by the server's feature-overrides middleware after workspace access resolves,
// and read by PlanGuard and by handlers that surface entitlements to clients.
// Exported so the server package sets the same key PlanGuard reads.
const ContextKeyFeatureOverrides = "feature_overrides"

// OverridesFromContext returns the per-workspace feature overrides stored on the
// echo context by the feature-overrides middleware, or nil if none are set.
func OverridesFromContext(c echo.Context) map[Feature]bool {
	o, _ := c.Get(ContextKeyFeatureOverrides).(map[Feature]bool)
	return o
}

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
			overrides := OverridesFromContext(c)

			if HasFeature(plan, feature, overrides) {
				return next(c)
			}

			if len(onBlock) > 0 && onBlock[0] != nil {
				wsID, _ := c.Get("workspace_id").(string)
				onBlock[0](analytics.EventFeatureGateHit, wsID, map[string]any{
					"feature":      string(feature),
					"plan":         planStr,
					"minimum_plan": string(MinimumPlanFor(feature)),
				})
			}

			return apierror.Write(c, http.StatusForbidden, "upgrade_required", map[string]any{
				"feature":      feature,
				"minimum_plan": MinimumPlanFor(feature),
			})
		}
	}
}

// GuardEventFunc is an optional callback fired when PlanGuard or QuotaGuard
// blocks a request. Used for analytics (e.g. PostHog).
type GuardEventFunc func(event string, workspaceID string, props map[string]any)

// RequireFeature is PlanGuard for a decision a handler can only make after
// reading the request body — a connector's type, say, which decides whether the
// request needs a paid feature at all. It returns nil when the workspace is
// entitled, and otherwise writes the same 403 `upgrade_required` payload the
// middleware writes, so the client's upgrade prompt does not have to know which
// of the two blocked it.
//
// Like PlanGuard, it allows everything when no plan is on the context (billing
// not configured — a self-hosted deployment gates nothing).
func RequireFeature(c echo.Context, feature Feature, onBlock ...GuardEventFunc) error {
	planStr, _ := c.Get(contextKeyWorkspacePlan).(string)
	if planStr == "" {
		return nil
	}
	if HasFeature(Plan(planStr), feature, OverridesFromContext(c)) {
		return nil
	}

	if len(onBlock) > 0 && onBlock[0] != nil {
		wsID, _ := c.Get("workspace_id").(string)
		onBlock[0](analytics.EventFeatureGateHit, wsID, map[string]any{
			"feature":      string(feature),
			"plan":         planStr,
			"minimum_plan": string(MinimumPlanFor(feature)),
		})
	}

	return apierror.Write(c, http.StatusForbidden, "upgrade_required", map[string]any{
		"feature":      feature,
		"minimum_plan": MinimumPlanFor(feature),
	})
}

// QuotaGuard returns Echo middleware that rejects requests when the workspace's
// spendable credits (monthly plan allowance + trial grant + purchased packs)
// are exhausted. Returns 429 with Retry-After set to the next month start
// (00:00 UTC on the 1st) — when a paid plan's allowance refreshes.
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
				return creditsExhaustedResponse(c, plan, onBlock...)
			}

			return next(c)
		}
	}
}

// creditsExhaustedResponse writes the standard 429 credits-exhausted payload
// (with a Retry-After of the next monthly reset — meaningful for paid plans;
// a Free workspace's way forward is an upgrade or a credit pack, which the
// client copy explains) and fires the optional analytics callback. Shared by
// QuotaGuard (middleware) and GuardSyncCredits (handler).
func creditsExhaustedResponse(c echo.Context, plan Plan, onBlock ...GuardEventFunc) error {
	retryAfter := MonthEnd(time.Now().UTC())
	c.Response().Header().Set("Retry-After", retryAfter.Format(time.RFC1123))
	if len(onBlock) > 0 && onBlock[0] != nil {
		workspaceID, _ := c.Get("workspace_id").(string)
		onBlock[0](analytics.EventCreditsExhausted, workspaceID, map[string]any{
			"plan": string(plan),
			"path": c.Path(),
		})
	}
	return apierror.Write(c, http.StatusTooManyRequests, "credits_exhausted", map[string]any{
		"resets_at":   retryAfter.Format(time.RFC3339),
		"retry_after": int(time.Until(retryAfter).Seconds()),
	})
}

// GuardSyncCredits enforces the credit gate from inside a handler, where
// BYO is known from the parsed request body. It returns a non-nil echo 429 error
// when the workspace is out of platform credits and the request is NOT BYO; nil
// (allow) otherwise. It is the body-aware counterpart to the QuotaGuard
// middleware: a synchronous AI route that accepts a bring-your-own key
// (provider_config_id / api_key) cannot be wrapped by QuotaGuard, because the
// middleware cannot see the body to know the request burns no credits.
//
// Like QuotaGuard, it degrades to allow when billing is not configured, the plan
// is empty or enterprise, there is no workspace context, or the balance is
// unreadable — so a self-hosted or misconfigured deployment is never blocked.
func GuardSyncCredits(c echo.Context, store BillingStore, byo bool, onBlock ...GuardEventFunc) error {
	if byo || store == nil {
		return nil
	}
	planStr, _ := c.Get(contextKeyWorkspacePlan).(string)
	if planStr == "" {
		return nil
	}
	plan := Plan(planStr)
	if plan == PlanEnterprise {
		return nil
	}
	workspaceID, _ := c.Get("workspace_id").(string)
	if workspaceID == "" {
		return nil
	}
	remaining, err := store.CheckCredits(c.Request().Context(), workspaceID)
	if err != nil {
		return nil
	}
	if remaining <= 0 {
		return creditsExhaustedResponse(c, plan, onBlock...)
	}
	return nil
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
	scheme, token, ok := strings.Cut(auth, " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return token
}
