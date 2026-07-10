package server

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
)

// FeatureOverridesMiddleware loads the per-workspace feature overrides from the
// billing store and places them on the echo context under
// billing.ContextKeyFeatureOverrides, so PlanGuard (and any handler surfacing
// entitlements) honors admin-granted overrides.
//
// This is what makes per-workspace overrides actually take effect in production:
// PlanGuard already reads the overrides key, but nothing populated it before —
// so an override set via the control plane was silently ignored. With this
// middleware, an admin can grant a dark feature (e.g. FeatureBravo) to a single
// workspace for dogfooding without changing the plan matrix.
//
// It must run after WorkspaceAccessMiddleware (which sets workspace_id) and
// before any PlanGuard. It is a no-op when billing is not configured, when there
// is no workspace on the context, or on store error (fail-open to the plan
// matrix, matching PlanGuard's "billing not configured → allow" posture).
// Expired overrides are ignored.
func FeatureOverridesMiddleware(store billing.BillingStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if store == nil {
				return next(c)
			}
			wsID, _ := c.Get("workspace_id").(string)
			if wsID == "" {
				return next(c)
			}
			overrides, err := store.GetFeatureOverrides(c.Request().Context(), wsID)
			if err != nil || len(overrides) == 0 {
				return next(c)
			}
			now := time.Now()
			m := make(map[billing.Feature]bool, len(overrides))
			for _, o := range overrides {
				if o.ExpiresAt != nil && o.ExpiresAt.Before(now) {
					continue // expired grant/revocation — fall back to the plan matrix
				}
				m[o.Feature] = o.Enabled
			}
			if len(m) > 0 {
				c.Set(billing.ContextKeyFeatureOverrides, m)
			}
			return next(c)
		}
	}
}
