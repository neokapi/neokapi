package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanGuard(t *testing.T) {
	tests := []struct {
		name       string
		plan       string
		feature    Feature
		overrides  map[Feature]bool
		wantStatus int
	}{
		{"pro allowed git connectors", "pro", FeatureConnectorsGit, nil, http.StatusOK},
		{"free blocked git connectors", "free", FeatureConnectorsGit, nil, http.StatusForbidden},
		{"team allowed bravo code exec", "team", FeatureBravoCodeExec, nil, http.StatusOK},
		{"pro blocked bravo code exec", "pro", FeatureBravoCodeExec, nil, http.StatusForbidden},
		{"enterprise allowed sso", "enterprise", FeatureSSOSAML, nil, http.StatusOK},
		{"team blocked sso", "team", FeatureSSOSAML, nil, http.StatusForbidden},
		{"free with override allowed", "free", FeatureConnectorsGit, map[Feature]bool{FeatureConnectorsGit: true}, http.StatusOK},
		{"pro with override revoked", "pro", FeatureConnectorsGit, map[Feature]bool{FeatureConnectorsGit: false}, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyWorkspacePlan, tt.plan)
			if tt.overrides != nil {
				c.Set("feature_overrides", tt.overrides)
			}

			handler := PlanGuard(tt.feature)(func(c echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			err := handler(c)
			if tt.wantStatus == http.StatusOK {
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				require.NoError(t, err) // JSON response, not echo error
				assert.Equal(t, http.StatusForbidden, rec.Code)
			}
		})
	}
}

// mockBillingStore implements enough of BillingStore for QuotaGuard testing.
type mockBillingStore struct {
	remaining int64
	err       error
}

func (m *mockBillingStore) GetSubscription(context.Context, string) (*Subscription, error) {
	return nil, nil
}
func (m *mockBillingStore) UpsertSubscription(context.Context, *Subscription) error { return nil }
func (m *mockBillingStore) ListSubscriptions(context.Context, int, int) ([]*Subscription, error) {
	return nil, nil
}
func (m *mockBillingStore) GetCurrentAllocation(_ context.Context, _ string) (*CreditAllocation, error) {
	if m.err != nil {
		return nil, m.err
	}
	now := time.Now().UTC()
	total := m.remaining + 1000
	used := total - m.remaining
	if m.remaining <= 0 {
		total = 50000
		used = 50000
	}
	return &CreditAllocation{
		CreditsTotal: total,
		CreditsUsed:  used,
		WeekStart:    WeekStart(now),
		WeekEnd:      WeekEnd(now),
	}, nil
}
func (m *mockBillingStore) DeductCredits(context.Context, string, int64, string, string) error {
	return nil
}
func (m *mockBillingStore) CheckCredits(_ context.Context, _ string) (int64, error) {
	return m.remaining, m.err
}
func (m *mockBillingStore) GrantCredits(context.Context, string, int64, string) error { return nil }
func (m *mockBillingStore) GetLedger(context.Context, string, time.Time, time.Time) ([]LedgerEntry, error) {
	return nil, nil
}
func (m *mockBillingStore) GetFeatureOverrides(context.Context, string) ([]FeatureOverride, error) {
	return nil, nil
}
func (m *mockBillingStore) ListAllFeatureOverrides(context.Context) ([]FeatureOverride, error) {
	return nil, nil
}
func (m *mockBillingStore) SetFeatureOverride(context.Context, *FeatureOverride) error { return nil }
func (m *mockBillingStore) DeleteFeatureOverride(context.Context, string, Feature) error {
	return nil
}
func (m *mockBillingStore) ListNotes(context.Context, string) ([]WorkspaceNote, error) {
	return nil, nil
}
func (m *mockBillingStore) AddNote(context.Context, *WorkspaceNote) error { return nil }
func (m *mockBillingStore) GetUpsellOpportunities(context.Context) ([]UpsellOpportunity, error) {
	return nil, nil
}
func (m *mockBillingStore) GetPlatformMetrics(context.Context) (*PlatformMetrics, error) {
	return nil, nil
}
func (m *mockBillingStore) ListBillingEvents(context.Context, int, int, string) ([]BillingEvent, error) {
	return nil, nil
}
func (m *mockBillingStore) RecordBillingEvent(context.Context, *BillingEvent) error { return nil }
func (m *mockBillingStore) MarkStripeEventProcessed(context.Context, string, string) (bool, error) {
	return false, nil
}
func (m *mockBillingStore) UnmarkStripeEvent(context.Context, string) error { return nil }

func TestQuotaGuard(t *testing.T) {
	tests := []struct {
		name       string
		plan       string
		remaining  int64
		storeErr   error
		wantStatus int
	}{
		{"credits remaining allows request", "pro", 10000, nil, http.StatusOK},
		{"credits exhausted blocks request", "pro", 0, nil, http.StatusTooManyRequests},
		{"negative credits blocks request", "free", -100, nil, http.StatusTooManyRequests},
		{"enterprise always allowed", "enterprise", 0, nil, http.StatusOK},
		{"store error allows request", "pro", 0, assert.AnError, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockBillingStore{remaining: tt.remaining, err: tt.storeErr}

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set(contextKeyWorkspacePlan, tt.plan)
			c.Set("workspace_id", "ws-123")

			handler := QuotaGuard(store)(func(c echo.Context) error {
				return c.String(http.StatusOK, "ok")
			})

			err := handler(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestQuotaGuard_RetryAfterHeader(t *testing.T) {
	store := &mockBillingStore{remaining: 0}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyWorkspacePlan, "free")
	c.Set("workspace_id", "ws-123")

	handler := QuotaGuard(store)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestQuotaGuard_OnBlockCallbackFired(t *testing.T) {
	store := &mockBillingStore{remaining: 0}

	var capturedEvent string
	var capturedWsID string
	var capturedProps map[string]any
	onBlock := func(event string, wsID string, props map[string]any) {
		capturedEvent = event
		capturedWsID = wsID
		capturedProps = props
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyWorkspacePlan, "pro")
	c.Set("workspace_id", "ws-456")

	handler := QuotaGuard(store, onBlock)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "billing.credits_exhausted", capturedEvent)
	assert.Equal(t, "ws-456", capturedWsID)
	assert.Equal(t, "pro", capturedProps["plan"])
}

func TestQuotaGuard_OnBlockNotFiredWhenAllowed(t *testing.T) {
	store := &mockBillingStore{remaining: 10000}

	fired := false
	onBlock := func(event string, wsID string, props map[string]any) {
		fired = true
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyWorkspacePlan, "pro")
	c.Set("workspace_id", "ws-456")

	handler := QuotaGuard(store, onBlock)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, fired)
}

func TestPlanGuard_OnBlockCallbackFired(t *testing.T) {
	var capturedEvent string
	var capturedWsID string
	onBlock := func(event string, wsID string, props map[string]any) {
		capturedEvent = event
		capturedWsID = wsID
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyWorkspacePlan, "free")
	c.Set("workspace_id", "ws-789")

	handler := PlanGuard(FeatureConnectorsGit, onBlock)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, "billing.feature_gate_hit", capturedEvent)
	assert.Equal(t, "ws-789", capturedWsID)
}

func TestPlanGuard_OnBlockNotFiredWhenAllowed(t *testing.T) {
	fired := false
	onBlock := func(event string, wsID string, props map[string]any) {
		fired = true
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(contextKeyWorkspacePlan, "pro")
	c.Set("workspace_id", "ws-789")

	handler := PlanGuard(FeatureConnectorsGit, onBlock)(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, fired)
}
