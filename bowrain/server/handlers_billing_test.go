package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBillingStore implements billing.BillingStore for handler tests.
type mockBillingStore struct {
	sub          *billing.Subscription
	alloc        *billing.CreditAllocation
	ledger       []billing.LedgerEntry
	overrides    []billing.FeatureOverride
	notes        []billing.WorkspaceNote
	upsells      []billing.UpsellOpportunity
	metrics      *billing.PlatformMetrics
	events       []billing.BillingEvent
	subs         []*billing.Subscription
	allOverrides []billing.FeatureOverride

	grantedAmount int64
	grantedSource string
	upsertedSub   *billing.Subscription
	addedNote     *billing.WorkspaceNote
	setOverride   *billing.FeatureOverride
	recordedEvts  []*billing.BillingEvent

	// spendable is what CheckCredits reports (the full multi-bucket balance).
	spendable int64
	// grantSetsAlloc makes a plan GrantCredits publish the allocation, like
	// the real store does.
	grantSetsAlloc bool
}

func (m *mockBillingStore) GetSubscription(_ context.Context, _ string) (*billing.Subscription, error) {
	if m.sub == nil {
		return nil, assert.AnError
	}
	return m.sub, nil
}
func (m *mockBillingStore) UpsertSubscription(_ context.Context, sub *billing.Subscription) error {
	m.upsertedSub = sub
	return nil
}
func (m *mockBillingStore) MarkStripeEventProcessed(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *mockBillingStore) UnmarkStripeEvent(_ context.Context, _ string) error {
	return nil
}
func (m *mockBillingStore) ListSubscriptions(_ context.Context, _, _ int) ([]*billing.Subscription, error) {
	return m.subs, nil
}
func (m *mockBillingStore) GetCurrentAllocation(_ context.Context, _ string) (*billing.CreditAllocation, error) {
	if m.alloc == nil {
		return nil, assert.AnError
	}
	return m.alloc, nil
}
func (m *mockBillingStore) DeductCredits(context.Context, string, int64, string, string) error {
	return nil
}
func (m *mockBillingStore) CheckCredits(context.Context, string) (int64, error) {
	return m.spendable, nil
}
func (m *mockBillingStore) GrantCredits(_ context.Context, _ string, amount int64, source string) error {
	m.grantedAmount = amount
	m.grantedSource = source
	if m.grantSetsAlloc {
		// Mirror the real store: after a plan grant, the current allocation
		// exists (so EnsureMonthlyAllocation's re-read finds it).
		m.alloc = &billing.CreditAllocation{
			CreditsTotal: amount,
			PeriodStart:  billing.MonthStart(time.Now().UTC()),
			PeriodEnd:    billing.MonthEnd(time.Now().UTC()),
			Source:       billing.SourcePlan,
		}
	}
	return nil
}
func (m *mockBillingStore) GrantPurchasedCredits(_ context.Context, _ string, amount int64, _ string) (bool, error) {
	m.grantedAmount = amount
	m.grantedSource = billing.SourcePurchased
	return true, nil
}
func (m *mockBillingStore) GrantTrialCredits(_ context.Context, _ string, amount int64) (bool, error) {
	m.grantedAmount = amount
	m.grantedSource = billing.SourceTrial
	return true, nil
}
func (m *mockBillingStore) GetLedger(_ context.Context, _ string, _, _ time.Time) ([]billing.LedgerEntry, error) {
	return m.ledger, nil
}
func (m *mockBillingStore) GetFeatureOverrides(_ context.Context, _ string) ([]billing.FeatureOverride, error) {
	return m.overrides, nil
}
func (m *mockBillingStore) ListAllFeatureOverrides(_ context.Context) ([]billing.FeatureOverride, error) {
	return m.allOverrides, nil
}
func (m *mockBillingStore) SetFeatureOverride(_ context.Context, o *billing.FeatureOverride) error {
	m.setOverride = o
	return nil
}
func (m *mockBillingStore) DeleteFeatureOverride(context.Context, string, billing.Feature) error {
	return nil
}
func (m *mockBillingStore) ListNotes(_ context.Context, _ string) ([]billing.WorkspaceNote, error) {
	return m.notes, nil
}
func (m *mockBillingStore) AddNote(_ context.Context, n *billing.WorkspaceNote) error {
	m.addedNote = n
	return nil
}
func (m *mockBillingStore) GetUpsellOpportunities(_ context.Context) ([]billing.UpsellOpportunity, error) {
	return m.upsells, nil
}
func (m *mockBillingStore) GetPlatformMetrics(_ context.Context) (*billing.PlatformMetrics, error) {
	if m.metrics == nil {
		return nil, assert.AnError
	}
	return m.metrics, nil
}
func (m *mockBillingStore) ListBillingEvents(_ context.Context, _, _ int, _ string) ([]billing.BillingEvent, error) {
	return m.events, nil
}
func (m *mockBillingStore) RecordBillingEvent(_ context.Context, evt *billing.BillingEvent) error {
	m.recordedEvts = append(m.recordedEvts, evt)
	return nil
}

func newBillingTestServer(store billing.BillingStore) *Server {
	return &Server{
		BillingStore: store,
	}
}

// MonthlyAllocationMiddleware routes each workspace to the right credit
// machinery: paid plans get the month's plan allocation; Free and
// still-trialing workspaces get the one-time trial-grant backfill instead
// (bounding per-signup exposure to the grant).
func TestMonthlyAllocationMiddleware_Dispatch(t *testing.T) {
	run := func(store *mockBillingStore, plan string) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("workspace_id", "ws-1")
		c.Set("workspace_plan", plan)
		handler := MonthlyAllocationMiddleware(store)(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		require.NoError(t, handler(c))
	}

	t.Run("free plan ensures the trial grant, never a plan allocation", func(t *testing.T) {
		store := &mockBillingStore{}
		run(store, "free")
		assert.Equal(t, billing.SourceTrial, store.grantedSource)
		assert.Equal(t, int64(billing.FreeTrialGrantCredits), store.grantedAmount)
	})

	t.Run("active paid plan gets the monthly plan allocation", func(t *testing.T) {
		store := &mockBillingStore{
			sub:            &billing.Subscription{WorkspaceID: "ws-1", Plan: billing.PlanPro, Status: "active"},
			grantSetsAlloc: true,
		}
		run(store, "pro")
		assert.Equal(t, billing.SourcePlan, store.grantedSource)
		assert.Equal(t, int64(billing.ProMonthlyCredits), store.grantedAmount)
	})

	t.Run("trialing paid plan gets the trial grant, not the paid allowance", func(t *testing.T) {
		store := &mockBillingStore{
			sub: &billing.Subscription{WorkspaceID: "ws-1", Plan: billing.PlanPro, Status: "trialing"},
		}
		run(store, "pro")
		assert.Equal(t, billing.SourceTrial, store.grantedSource,
			"a trialing subscription must not mint the paid monthly allocation")
	})

	t.Run("existing allocation grants nothing", func(t *testing.T) {
		now := time.Now().UTC()
		store := &mockBillingStore{
			alloc: &billing.CreditAllocation{
				CreditsTotal: billing.ProMonthlyCredits,
				PeriodStart:  billing.MonthStart(now),
				PeriodEnd:    billing.MonthEnd(now),
				Source:       billing.SourcePlan,
			},
		}
		run(store, "pro")
		assert.Empty(t, store.grantedSource, "an existing month allocation must not re-grant")
	})
}

func TestHandleGetBilling_NilStore(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleGetBilling(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleGetBilling_FreePlan(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")

	err := s.HandleGetBilling(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "free", resp["plan"])
	assert.Equal(t, "active", resp["status"])
}

func TestHandleGetBilling_WithSubscription(t *testing.T) {
	now := time.Now().UTC()
	store := &mockBillingStore{
		sub: &billing.Subscription{
			WorkspaceID: "ws-1",
			Plan:        billing.PlanPro,
			Status:      "active",
		},
		alloc: &billing.CreditAllocation{
			CreditsTotal: 500000,
			CreditsUsed:  123000,
			PeriodStart:  billing.MonthStart(now),
			PeriodEnd:    billing.MonthEnd(now),
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")

	err := s.HandleGetBilling(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "pro", resp["plan"])
	assert.Equal(t, float64(500000), resp["credits_total"])
	assert.Equal(t, float64(123000), resp["credits_used"])
	assert.Equal(t, float64(377000), resp["credits_remaining"])
	// The payload names the monthly reset, not the weekly one.
	require.Contains(t, resp, "month_resets_at")
	assert.NotContains(t, resp, "week_resets_at")
	resetsAt, err := time.Parse(time.RFC3339, resp["month_resets_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, billing.MonthEnd(now), resetsAt.UTC())
}

// A free workspace has no plan allocation (Free's recurring allowance is 0),
// but the payload must still expose the full spendable balance — the remaining
// one-time trial grant — so the billing page has a real number to show.
func TestHandleGetBilling_FreeShowsSpendableTrialBalance(t *testing.T) {
	store := &mockBillingStore{spendable: 200_000}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")

	err := s.HandleGetBilling(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "free", resp["plan"])
	assert.Equal(t, float64(0), resp["credits_total"], "no recurring plan bucket on Free")
	assert.Equal(t, float64(200_000), resp["spendable_credits"], "the trial grant is the spendable balance")
}

func TestHandleGetBillingUsage_NilStore(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing/usage", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleGetBillingUsage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleGetBillingUsage_WithEntries(t *testing.T) {
	store := &mockBillingStore{
		ledger: []billing.LedgerEntry{
			{Amount: -1000, Operation: "ai_translation"},
			{Amount: -500, Operation: "ai_translation"},
			{Amount: -200, Operation: "bravo_message"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing/usage", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")

	err := s.HandleGetBillingUsage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	usage := resp["usage_by_operation"].(map[string]any)
	assert.Equal(t, float64(1500), usage["ai_translation"])
	assert.Equal(t, float64(200), usage["bravo_message"])
}

func TestHandleCreateCheckout_NilStripe(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/billing/checkout", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleCreateCheckout(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleCreatePortal_NilStripe(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/billing/portal", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleCreatePortal(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleGetInvoices_NilStripe(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing/invoices", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleGetInvoices(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleStripeWebhook_NilHandler(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleStripeWebhook(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleStripeWebhook_MissingSignature(t *testing.T) {
	store := &mockBillingStore{}
	s := &Server{
		WebhookHandler: billing.NewWebhookHandler(store, "whsec_test"),
	}

	e := echo.New()
	body := `{"type": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/stripe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleStripeWebhook(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
