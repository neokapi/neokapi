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
func PlanGuard(feature Feature) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			plan := Plan(c.Get(contextKeyWorkspacePlan).(string))

			// Check overrides if available on context.
			var overrides map[Feature]bool
			if o, ok := c.Get("feature_overrides").(map[Feature]bool); ok {
				overrides = o
			}

			if HasFeature(plan, feature, overrides) {
				return next(c)
			}

			return c.JSON(http.StatusForbidden, map[string]any{
				"error":        "upgrade_required",
				"feature":      feature,
				"minimum_plan": MinimumPlanFor(feature),
			})
		}
	}
}

// QuotaGuard returns Echo middleware that rejects requests when weekly credits
// are exhausted. Returns 429 with Retry-After header set to next Monday 00:00 UTC.
func QuotaGuard(store BillingStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			plan := Plan(c.Get(contextKeyWorkspacePlan).(string))

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
// admin Keycloak realm. It sets "admin_email" and "admin_name" on the context.
func AdminGuard(adminVerifier *oidc.IDTokenVerifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractBearerToken(c)
			if token == "" {
				return echo.ErrUnauthorized
			}

			idToken, err := adminVerifier.Verify(c.Request().Context(), token)
			if err != nil {
				return echo.ErrUnauthorized
			}

			var claims struct {
				Email string `json:"email"`
				Name  string `json:"name"`
			}
			if err := idToken.Claims(&claims); err != nil {
				return echo.ErrUnauthorized
			}

			c.Set("admin_email", claims.Email)
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
