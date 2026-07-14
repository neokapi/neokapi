package server

import (
	"time"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
)

// FeatureOverridesMiddleware resolves the effective feature overrides for a
// request and places them on the echo context under
// billing.ContextKeyFeatureOverrides, so PlanGuard (and any handler surfacing
// entitlements) honors them. Two layers are merged, highest precedence last:
//
//  1. Instance-wide global feature flags (ctrl-managed platform_config): an
//     admin toggle that turns a feature on or off across every workspace,
//     layered OVER the plan matrix but UNDER per-workspace grants.
//  2. Per-workspace feature overrides (billing store): admin-granted or -revoked
//     per workspace, which win over the instance-wide flags — so the founder can
//     dogfood a dark feature (e.g. FeatureBravo) on one workspace regardless of
//     the global default.
//
// The resulting map is what makes both layers take effect: PlanGuard reads the
// overrides key, and HasFeature treats a present key as authoritative over the
// plan matrix.
//
// It must run after WorkspaceAccessMiddleware (which sets workspace_id) and
// before any PlanGuard. Per-workspace lookup is skipped when billing is not
// configured, when there is no workspace on the context, or on store error
// (fail-open, matching PlanGuard's "billing not configured → allow" posture);
// the instance-wide flags still apply. Expired per-workspace overrides are
// ignored.
//
// globalFeatures returns the current instance-wide flags (nil is fine); it is
// read per request so a ctrl change is picked up live via the platform_config
// service cache.
func FeatureOverridesMiddleware(store billing.BillingStore, globalFeatures func() map[string]bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			m := map[billing.Feature]bool{}

			// Layer 1: instance-wide global flags (under per-workspace overrides).
			if globalFeatures != nil {
				for name, enabled := range globalFeatures() {
					if name != "" {
						m[billing.Feature(name)] = enabled
					}
				}
			}

			// Layer 2: per-workspace overrides (win over global flags).
			if store != nil {
				if wsID, _ := c.Get("workspace_id").(string); wsID != "" {
					if overrides, err := store.GetFeatureOverrides(c.Request().Context(), wsID); err == nil {
						now := time.Now()
						for _, o := range overrides {
							if o.ExpiresAt != nil && o.ExpiresAt.Before(now) {
								continue // expired grant/revocation — fall back
							}
							m[o.Feature] = o.Enabled
						}
					}
				}
			}

			if len(m) > 0 {
				c.Set(billing.ContextKeyFeatureOverrides, m)
			}
			return next(c)
		}
	}
}
