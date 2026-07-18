package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkoutServer builds a Server whose Stripe client exists but whose price
// configuration is under the test's control. The Stripe key is deliberately a
// fake: every assertion below must be reached *before* the handler would talk to
// Stripe, which is the point — validation happens server-side, not at Stripe.
func checkoutServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	authStore := newMockAuthStoreForBilling()
	authStore.workspaces["ws-1"] = &platauth.Workspace{ID: "ws-1", Plan: "free"}
	return &Server{
		Config:       cfg,
		BillingStore: &mockBillingStore{},
		StripeClient: billing.NewStripeClient("sk_test_fake"),
		AuthStore:    authStore,
	}
}

func checkoutRequest(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/billing/checkout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")
	c.Set("workspace_role", platauth.RoleOwner)
	require.NoError(t, s.HandleCreateCheckout(c))
	return rec
}

// The checkout endpoint used to forward any client-supplied price_id straight to
// Stripe: an authenticated owner could check out against *any* price in the
// Stripe account, including a $0 one, and get the plan attached to it. The field
// no longer exists — a body carrying only price_id names no plan and is rejected
// without Stripe ever being called.
func TestCreateCheckout_ClientSuppliedPriceIDIsRejected(t *testing.T) {
	s := checkoutServer(t, Config{
		StripeProPriceID:  "price_pro",
		StripeTeamPriceID: "price_team",
	})

	rec := checkoutRequest(t, s, `{"price_id":"price_attacker_chose_this","success_url":"https://x/ok"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unknown plan")
}

func TestCreateCheckout_UnknownPlanIsRejected(t *testing.T) {
	s := checkoutServer(t, Config{StripeProPriceID: "price_pro"})

	for _, plan := range []string{"", "free", "enterprise", "pro-plus", "TEAM"} {
		rec := checkoutRequest(t, s, `{"plan":"`+plan+`","success_url":"https://x/ok"}`)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "plan %q must not be self-serve purchasable", plan)
	}
}

// A plan whose price this deployment has not configured is not purchasable. It
// must say so rather than send an empty price to Stripe.
func TestCreateCheckout_UnconfiguredPlanIsNotPurchasable(t *testing.T) {
	s := checkoutServer(t, Config{StripeProPriceID: "price_pro"}) // no team price

	rec := checkoutRequest(t, s, `{"plan":"team","success_url":"https://x/ok"}`)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "not purchasable")
}

func TestCreateCheckout_SuccessURLRequired(t *testing.T) {
	s := checkoutServer(t, Config{StripeProPriceID: "price_pro"})

	rec := checkoutRequest(t, s, `{"plan":"pro"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "success_url")
}

// Seats are what a per-seat plan is billed on, so a workspace cannot buy fewer
// seats than it already has members — that would under-bill it from day one.
func TestCreateCheckout_SeatsBelowMemberCountRejected(t *testing.T) {
	s := checkoutServer(t, Config{StripeTeamPriceID: "price_team"})
	authStore := s.AuthStore.(*mockAuthStoreForBilling)
	authStore.members["ws-1"] = []*platauth.Membership{
		{UserID: "u1"}, {UserID: "u2"}, {UserID: "u3"},
	}

	rec := checkoutRequest(t, s, `{"plan":"team","seats":1,"success_url":"https://x/ok"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "fewer than the workspace's current members")
}

// A self-serve purchase is bounded: beyond the cap the buyer is a sales
// conversation, and the cap stops a fat-fingered quantity creating a five-figure
// subscription.
func TestCreateCheckout_SeatsAboveCapRejected(t *testing.T) {
	s := checkoutServer(t, Config{StripeTeamPriceID: "price_team"})

	rec := checkoutRequest(t, s, `{"plan":"team","seats":5000,"success_url":"https://x/ok"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "self-serve maximum")
}

// Only an owner may spend the workspace's money.
func TestCreateCheckout_RequiresOwner(t *testing.T) {
	s := checkoutServer(t, Config{StripeProPriceID: "price_pro"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/billing/checkout",
		strings.NewReader(`{"plan":"pro","success_url":"https://x/ok"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")
	c.Set("workspace_role", platauth.RoleViewer)

	_ = s.HandleCreateCheckout(c)
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func decodePlans(t *testing.T, rec *httptest.ResponseRecorder) map[billing.Plan]billing.PlanInfo {
	t.Helper()
	var resp struct {
		Plans      []billing.PlanInfo `json:"plans"`
		CreditPack struct {
			Credits     int64 `json:"credits"`
			Purchasable bool  `json:"purchasable"`
		} `json:"credit_pack"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	byID := map[billing.Plan]billing.PlanInfo{}
	for _, p := range resp.Plans {
		byID[p.ID] = p
	}
	return byID
}

func listPlans(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/billing/plans", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("workspace_id", "ws-1")
	require.NoError(t, s.HandleListPlans(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	return rec
}

// The client renders upgrade buttons from this endpoint, so "purchasable" must
// mean the price actually exists in this deployment's configuration.
func TestListPlans_PurchasableFollowsConfiguredPrices(t *testing.T) {
	s := checkoutServer(t, Config{StripeProPriceID: "price_pro"}) // pro only

	plans := decodePlans(t, listPlans(t, s))

	assert.True(t, plans[billing.PlanPro].Purchasable)
	assert.False(t, plans[billing.PlanTeam].Purchasable, "no team price configured")
	assert.False(t, plans[billing.PlanFree].Purchasable, "free is the downgrade target, not a purchase")
	assert.False(t, plans[billing.PlanEnterprise].Purchasable, "enterprise is sold by hand")

	assert.Equal(t, int64(billing.ProMonthlyCredits), plans[billing.PlanPro].MonthlyCredits)
	assert.Equal(t, int64(0), plans[billing.PlanFree].MonthlyCredits, "free has no recurring allowance")
	assert.True(t, plans[billing.PlanTeam].PerSeat)
}

// With no Stripe at all (self-hosted, or prod before provisioning) nothing is
// purchasable — the UI shows no upgrade buttons rather than buttons that 503.
func TestListPlans_NoStripeMeansNothingPurchasable(t *testing.T) {
	s := &Server{
		Config:       Config{StripeProPriceID: "price_pro", StripeTeamPriceID: "price_team"},
		BillingStore: &mockBillingStore{},
	}

	plans := decodePlans(t, listPlans(t, s))

	for _, p := range plans {
		assert.False(t, p.Purchasable, "plan %s must not be purchasable without a Stripe client", p.ID)
	}
}

func TestListPlans_MarksCurrentPlan(t *testing.T) {
	s := checkoutServer(t, Config{StripeProPriceID: "price_pro", StripeTeamPriceID: "price_team"})
	s.BillingStore = &mockBillingStore{
		sub: &billing.Subscription{WorkspaceID: "ws-1", Plan: billing.PlanTeam, Status: "active"},
	}

	plans := decodePlans(t, listPlans(t, s))

	assert.True(t, plans[billing.PlanTeam].Current)
	assert.False(t, plans[billing.PlanPro].Current)
}
