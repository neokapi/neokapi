package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/billing"
)

func TestFeatureOverridesMiddleware(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	tests := []struct {
		name  string
		store billing.BillingStore
		wsID  string
		want  map[billing.Feature]bool // nil = overrides key never set
	}{
		{"nil store is a no-op", nil, "ws1", nil},
		{
			"no workspace on context skips load",
			&mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: true}}},
			"",
			nil,
		},
		{
			"loads an active grant",
			&mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: true}}},
			"ws1",
			map[billing.Feature]bool{billing.FeatureBravo: true},
		},
		{
			"honors a revocation",
			&mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: false}}},
			"ws1",
			map[billing.Feature]bool{billing.FeatureBravo: false},
		},
		{
			"skips an expired override (falls back to plan matrix)",
			&mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: true, ExpiresAt: &past}}},
			"ws1",
			nil,
		},
		{
			"keeps an unexpired override",
			&mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: true, ExpiresAt: &future}}},
			"ws1",
			map[billing.Feature]bool{billing.FeatureBravo: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tt.wsID != "" {
				c.Set("workspace_id", tt.wsID)
			}

			var got map[billing.Feature]bool
			h := FeatureOverridesMiddleware(tt.store, nil)(func(c echo.Context) error {
				got = billing.OverridesFromContext(c)
				return c.String(http.StatusOK, "ok")
			})
			require.NoError(t, h(c))
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFeatureOverridesMiddleware_EnablesPlanGuard proves the founder-dogfood path
// end-to-end: FeatureBravo is dark on every plan, but a per-workspace override
// loaded by this middleware lets it through PlanGuard. Without the middleware the
// override would be ignored (the bug this epic also fixes).
func TestFeatureOverridesMiddleware_EnablesPlanGuard(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws1")
	c.Set("workspace_plan", "team") // team does NOT grant FeatureBravo

	store := &mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: true}}}

	final := FeatureOverridesMiddleware(store, nil)(
		billing.PlanGuard(billing.FeatureBravo)(func(c echo.Context) error {
			return c.String(http.StatusOK, "reached")
		}),
	)
	require.NoError(t, final(c))
	assert.Equal(t, http.StatusOK, rec.Code, "override must let the dark feature through PlanGuard")

	// Without the override, the same plan is blocked.
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec2)
	c2.Set("workspace_id", "ws1")
	c2.Set("workspace_plan", "team")
	blocked := FeatureOverridesMiddleware(&mockBillingStore{}, nil)(
		billing.PlanGuard(billing.FeatureBravo)(func(c echo.Context) error {
			return c.String(http.StatusOK, "reached")
		}),
	)
	require.NoError(t, blocked(c2))
	assert.Equal(t, http.StatusForbidden, rec2.Code, "dark feature is blocked without an override")
}

// TestFeatureOverridesMiddleware_GlobalFlagsLayer proves the precedence order:
// instance-wide global flags override the plan matrix, and per-workspace
// overrides in turn win over the global flags.
func TestFeatureOverridesMiddleware_GlobalFlagsLayer(t *testing.T) {
	e := echo.New()

	globals := func() map[string]bool {
		return map[string]bool{string(billing.FeatureBravo): true}
	}

	// Global flag alone enables FeatureBravo (no per-workspace override, no store).
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.Set("workspace_id", "ws1")
	var got map[billing.Feature]bool
	h := FeatureOverridesMiddleware(nil, globals)(func(c echo.Context) error {
		got = billing.OverridesFromContext(c)
		return c.String(http.StatusOK, "ok")
	})
	require.NoError(t, h(c))
	assert.True(t, got[billing.FeatureBravo], "global flag must enable the feature")

	// A per-workspace override of false wins over the global true.
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec2)
	c2.Set("workspace_id", "ws1")
	store := &mockBillingStore{overrides: []billing.FeatureOverride{{Feature: billing.FeatureBravo, Enabled: false}}}
	var got2 map[billing.Feature]bool
	h2 := FeatureOverridesMiddleware(store, globals)(func(c echo.Context) error {
		got2 = billing.OverridesFromContext(c)
		return c.String(http.StatusOK, "ok")
	})
	require.NoError(t, h2(c2))
	assert.False(t, got2[billing.FeatureBravo], "per-workspace override must win over the global flag")
}
