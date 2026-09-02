package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
)

// HandleGetBilling returns the current plan, subscription status, credit balance,
// and monthly reset countdown for a workspace.
//
// credits_total/used/remaining describe the MONTHLY PLAN bucket (what resets
// and what an upgrade changes); they are zero for workspaces without a plan
// allocation (Free, or a still-trialing subscription). spendable_credits is the
// full balance across every bucket (plan + trial grant + purchased packs) —
// the number QuotaGuard actually enforces — so the client can show a Free
// workspace its remaining trial credits.
// GET /api/v1/:ws/billing
func (s *Server) HandleGetBilling(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)

	ctx := c.Request().Context()
	sub, err := s.BillingStore.GetSubscription(ctx, wsID)
	if err != nil {
		// No subscription means free plan.
		sub = &billing.Subscription{
			WorkspaceID: wsID,
			Plan:        billing.PlanFree,
			Status:      "active",
		}
	}

	// Ensure this month's plan allocation exists (paid plans only; nil for
	// Free and still-trialing workspaces).
	alloc, _ := billing.EnsureMonthlyAllocation(ctx, s.BillingStore, wsID, sub.Plan)
	var creditsTotal, creditsUsed, creditsRemaining int64
	monthEnd := billing.MonthEnd(time.Now().UTC())
	if alloc != nil {
		creditsTotal = alloc.CreditsTotal
		creditsUsed = alloc.CreditsUsed
		creditsRemaining = max(creditsTotal-creditsUsed, 0)
		monthEnd = alloc.PeriodEnd
	}

	// Full spendable balance across all buckets. Degrades to the plan remainder
	// when unreadable (e.g. a workspace with no credit rows yet).
	spendable := creditsRemaining
	if bal, err := s.BillingStore.CheckCredits(ctx, wsID); err == nil {
		spendable = max(bal, 0)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"plan":              sub.Plan,
		"status":            sub.Status,
		"credits_total":     creditsTotal,
		"credits_used":      creditsUsed,
		"credits_remaining": creditsRemaining,
		"spendable_credits": spendable,
		"month_resets_at":   monthEnd,
		"subscription":      sub,
	})
}

// BillingUsageResponse is the credit-usage answer: the per-operation spend
// summed over the whole window, and one page of the ledger behind it.
//
// The two halves are narrowed differently on purpose — UsageByOperation covers
// the window whatever ?operation and ?limit say, Entries is the page those
// narrow — so they are named rather than left to an anonymous map a reader has
// to infer the contract of.
//
// UsageByOperation and NetByOperation answer different questions and a client
// needs both. Usage is spend: debits only, summed positive, which is what a
// "where did the credits go" breakdown means. Net is every movement, signed —
// including the purchases, grants, expiries and plan resets that spend
// excludes. A client deriving the ledger's operation filter from the spend
// breakdown could not offer the operations the ledger table beside it was
// already showing.
type BillingUsageResponse struct {
	UsageByOperation map[string]int64      `json:"usage_by_operation"`
	NetByOperation   map[string]int64      `json:"net_by_operation"`
	From             time.Time             `json:"from"`
	To               time.Time             `json:"to"`
	Entries          []billing.LedgerEntry `json:"entries"`
	Total            int                   `json:"total"`
	Limit            int                   `json:"limit"`
	Offset           int                   `json:"offset"`
}

// HandleGetBillingUsage returns the credit usage breakdown by operation type
// plus one page of the ledger behind it.
//
// usage_by_operation is summed in SQL over the whole window, so it stays
// correct however small the page is; entries is the ?limit/?offset page,
// narrowed by ?operation. total counts the entries the same filter matches.
// GET /api/v1/:ws/billing/usage
func (s *Server) HandleGetBillingUsage(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)

	// Default to the current month (the allocation period).
	now := time.Now().UTC()
	from := billing.MonthStart(now)
	to := now

	if v := c.QueryParam("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := c.QueryParam("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	ctx := c.Request().Context()
	byOp, err := s.BillingStore.GetUsageByOperation(ctx, wsID, from, to)
	if err != nil {
		return serverErr(c, err)
	}
	netByOp, err := s.BillingStore.GetNetByOperation(ctx, wsID, from, to)
	if err != nil {
		return serverErr(c, err)
	}

	limit, offset := pageParams(c, 50, billing.MaxLedgerPageSize)
	page, err := s.BillingStore.GetLedgerPage(ctx, wsID, billing.LedgerQuery{
		From:      from,
		To:        to,
		Operation: c.QueryParam("operation"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return serverErr(c, err)
	}
	if page.Entries == nil {
		page.Entries = []billing.LedgerEntry{}
	}

	return c.JSON(http.StatusOK, BillingUsageResponse{
		UsageByOperation: byOp,
		NetByOperation:   netByOp,
		From:             from,
		To:               to,
		Entries:          page.Entries,
		Total:            page.Total,
		Limit:            page.Limit,
		Offset:           page.Offset,
	})
}

// HandleGetBillingModelUsage returns token usage grouped by model and operation.
// GET /api/v1/workspaces/:ws/billing/model-usage
func (s *Server) HandleGetBillingModelUsage(c echo.Context) error {
	if s.QuotaStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "usage tracking not configured"})
	}

	ws := c.Param("ws")
	now := time.Now().UTC()
	from := billing.MonthStart(now)
	to := now

	if v := c.QueryParam("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := c.QueryParam("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	pgStore, ok := s.QuotaStore.(*jobs.QuotaStoreDB)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "model usage not available"})
	}

	ctx := c.Request().Context()
	usage, err := pgStore.GetUsageByModel(ctx, ws, from, to)
	if err != nil {
		return serverErr(c, err)
	}

	// Also fetch runner/container time usage.
	// Use workspace_id from context (more reliable than slug for runner_usage table).
	wsID, _ := c.Get("workspace_id").(string)
	runnerKey := wsID
	if runnerKey == "" {
		runnerKey = ws
	}
	runnerUsage, _ := pgStore.GetRunnerUsage(ctx, runnerKey, from, to)

	return c.JSON(http.StatusOK, map[string]any{
		"model_usage":  usage,
		"runner_usage": runnerUsage,
		"from":         from,
		"to":           to,
	})
}

// HandleListPlans returns the plans this deployment can sell, so the client can
// render exactly the upgrade paths that are actually purchasable here.
//
// It carries no dollar amounts: prices live in Stripe (DECISIONS L4). A plan is
// purchasable only when its Stripe price is configured — on a self-hosted or
// not-yet-provisioned deployment every plan comes back purchasable=false and the
// UI shows no upgrade buttons rather than buttons that 503.
// GET /api/v1/:ws/billing/plans
func (s *Server) HandleListPlans(c echo.Context) error {
	wsID, _ := c.Get("workspace_id").(string)

	current := billing.PlanFree
	if s.BillingStore != nil {
		if sub, err := s.BillingStore.GetSubscription(c.Request().Context(), wsID); err == nil && sub != nil {
			current = sub.Plan
		}
	}

	plans := make([]billing.PlanInfo, 0, len(billing.ValidPlans))
	for _, p := range []billing.Plan{billing.PlanFree, billing.PlanPro, billing.PlanTeam, billing.PlanEnterprise} {
		purchasable := s.StripeClient != nil && s.priceForPlan(p) != ""
		plans = append(plans, billing.DescribePlan(p, purchasable, p == current))
	}

	return c.JSON(http.StatusOK, map[string]any{
		"plans": plans,
		"credit_pack": map[string]any{
			"credits":     billing.CreditPackCredits,
			"purchasable": s.StripeClient != nil && s.Config.StripeCreditPriceID != "",
		},
	})
}

// priceForPlan resolves a plan to its configured Stripe price ID. Only the
// self-serve plans have one: Free is the downgrade target rather than a purchase,
// and Enterprise is sold by hand. An unconfigured price yields "" — the caller
// turns that into "not purchasable", never into a call to Stripe.
func (s *Server) priceForPlan(plan billing.Plan) string {
	switch plan {
	case billing.PlanPro:
		return s.Config.StripeProPriceID
	case billing.PlanTeam:
		return s.Config.StripeTeamPriceID
	default:
		return ""
	}
}

// HandleCreateCheckout creates a Stripe Checkout session and returns the URL.
//
// The client asks for a *plan*, never a price. The price is resolved here from
// the deployment's configuration — the same server-side pattern HandleBuyCredits
// already used for the credit pack. (Accepting a client-supplied price_id let any
// authenticated owner check out against any price in the Stripe account,
// including a $0 one.)
// POST /api/v1/:ws/billing/checkout
func (s *Server) HandleCreateCheckout(c echo.Context) error {
	if s.StripeClient == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "stripe not configured"})
	}

	if err := s.requireRole(c, platauth.RoleOwner); err != nil {
		return err
	}

	wsID, _ := c.Get("workspace_id").(string)

	var req struct {
		Plan       string `json:"plan"`
		Seats      int    `json:"seats"`
		SuccessURL string `json:"success_url"`
		CancelURL  string `json:"cancel_url"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.SuccessURL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "success_url is required"})
	}

	plan := billing.Plan(req.Plan)
	if !slices.Contains(billing.SelfServePlans, plan) {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "unknown plan: must be one of pro, team"})
	}
	priceID := s.priceForPlan(plan)
	if priceID == "" {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "plan " + req.Plan + " is not purchasable on this deployment"})
	}

	ctx := c.Request().Context()

	// Seats are the Stripe subscription quantity for a per-seat plan, so they are
	// what the customer is charged for. Default to the workspace's current member
	// count — the honest number — and reject anything below it, because a
	// subscription with fewer seats than members would under-bill a workspace that
	// is already over the limit.
	seats := 1
	if billing.PerSeatPlans[plan] {
		var err error
		seats, err = s.checkoutSeats(ctx, wsID, req.Seats)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		}
	}

	// Look up existing Stripe customer for this workspace.
	sub, _ := s.BillingStore.GetSubscription(ctx, wsID)
	var customerID string
	if sub != nil && sub.StripeCustomerID != "" {
		customerID = sub.StripeCustomerID
	}

	// Create a Stripe customer if one doesn't exist yet.
	if customerID == "" {
		email, _ := c.Get("email").(string)
		wsSlug := c.Param("ws")
		var err error
		customerID, err = s.StripeClient.CreateCustomer(ctx, wsID, email, wsSlug)
		if err != nil {
			return serverErr(c, fmt.Errorf("failed to create stripe customer: %w", err))
		}
	}

	// plan and seats ride along on the session (and, via SubscriptionData, on the
	// subscription) so the checkout.session.completed webhook can apply the right
	// plan immediately instead of defaulting to Pro and waiting for a subscription
	// event that may never come.
	url, err := s.StripeClient.CreateCheckoutSessionWithOptions(customerID, priceID, req.SuccessURL, req.CancelURL, billing.CheckoutOptions{
		Quantity: int64(seats),
		Metadata: map[string]string{
			"workspace_id": wsID,
			"plan":         string(plan),
			"seats":        strconv.Itoa(seats),
		},
	})
	if err != nil {
		return serverErr(c, err)
	}

	// Track checkout started event.
	if s.PostHogClient != nil {
		userID, _ := c.Get("user_id").(string)
		s.PostHogClient.CaptureEvent(userID, analytics.EventCheckoutStarted, map[string]any{
			"workspace_id": wsID,
			"plan":         string(plan),
			"seats":        seats,
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"checkout_url": url,
	})
}

// checkoutSeats resolves the seat count to bill for a per-seat plan: the
// requested number, floored at the workspace's current member count (you cannot
// buy fewer seats than you are using) and at 1.
func (s *Server) checkoutSeats(ctx context.Context, wsID string, requested int) (int, error) {
	members := 1
	if s.AuthStore != nil {
		if m, err := s.AuthStore.ListMembers(ctx, wsID); err == nil {
			members = max(len(m), 1)
		}
	}
	if requested == 0 {
		return members, nil
	}
	if requested < members {
		return 0, fmt.Errorf("seats (%d) is fewer than the workspace's current members (%d)", requested, members)
	}
	if requested > maxCheckoutSeats {
		return 0, fmt.Errorf("seats (%d) exceeds the self-serve maximum (%d). Contact sales", requested, maxCheckoutSeats)
	}
	return requested, nil
}

// maxCheckoutSeats bounds a self-serve per-seat purchase. Beyond it the buyer is
// an Enterprise conversation, and the cap keeps a fat-fingered (or hostile)
// quantity from creating a five-figure subscription.
const maxCheckoutSeats = 50

// HandleCreatePortal creates a Stripe Customer Portal session and returns the URL.
// POST /api/v1/:ws/billing/portal
func (s *Server) HandleCreatePortal(c echo.Context) error {
	if s.StripeClient == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "stripe not configured"})
	}

	if err := s.requireRole(c, platauth.RoleOwner); err != nil {
		return err
	}

	wsID, _ := c.Get("workspace_id").(string)

	ctx := c.Request().Context()
	sub, err := s.BillingStore.GetSubscription(ctx, wsID)
	if err != nil || sub == nil || sub.StripeCustomerID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no active subscription"})
	}

	var req struct {
		ReturnURL string `json:"return_url"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	url, err := s.StripeClient.CreatePortalSession(ctx, sub.StripeCustomerID, req.ReturnURL)
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"portal_url": url,
	})
}

// HandleGetInvoices returns invoice history from Stripe.
// GET /api/v1/:ws/billing/invoices
func (s *Server) HandleGetInvoices(c echo.Context) error {
	if s.StripeClient == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "stripe not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)

	ctx := c.Request().Context()
	sub, err := s.BillingStore.GetSubscription(ctx, wsID)
	if err != nil || sub == nil || sub.StripeCustomerID == "" {
		return c.JSON(http.StatusOK, map[string]any{"invoices": []any{}})
	}

	invoices, err := s.StripeClient.GetInvoices(ctx, sub.StripeCustomerID, 25)
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"invoices": invoices,
	})
}

// HandleBuyCredits creates a one-time Stripe Checkout session for purchasing credit packs.
// POST /api/v1/:ws/billing/buy-credits
func (s *Server) HandleBuyCredits(c echo.Context) error {
	if s.StripeClient == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "stripe not configured"})
	}
	if s.Config.StripeCreditPriceID == "" {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "credit pack purchases not configured"})
	}

	if err := s.requireRole(c, platauth.RoleOwner); err != nil {
		return err
	}

	wsID, _ := c.Get("workspace_id").(string)
	ctx := c.Request().Context()

	var req struct {
		SuccessURL string `json:"success_url"`
		CancelURL  string `json:"cancel_url"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.SuccessURL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "success_url is required"})
	}

	// Get or create Stripe customer.
	sub, _ := s.BillingStore.GetSubscription(ctx, wsID)
	var customerID string
	if sub != nil && sub.StripeCustomerID != "" {
		customerID = sub.StripeCustomerID
	}
	if customerID == "" {
		email, _ := c.Get("email").(string)
		wsSlug := c.Param("ws")
		var err error
		customerID, err = s.StripeClient.CreateCustomer(ctx, wsID, email, wsSlug)
		if err != nil {
			return serverErr(c, fmt.Errorf("failed to create stripe customer: %w", err))
		}
	}

	url, err := s.StripeClient.CreatePaymentCheckout(ctx, customerID, s.Config.StripeCreditPriceID, req.SuccessURL, req.CancelURL, map[string]string{
		"workspace_id": wsID,
		"type":         "credit_pack",
	})
	if err != nil {
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"checkout_url": url,
	})
}

// HandleStripeWebhook processes incoming Stripe webhook events.
// POST /api/webhooks/stripe
func (s *Server) HandleStripeWebhook(c echo.Context) error {
	if s.WebhookHandler == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "stripe webhooks not configured"})
	}

	// Cap the webhook body before the signature check: a Stripe event is small,
	// and an unauthenticated caller must not be able to make the server buffer an
	// arbitrarily large body ahead of verification.
	body, err := io.ReadAll(io.LimitReader(c.Request().Body, 1<<20))
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to read body"})
	}

	sig := c.Request().Header.Get("Stripe-Signature")
	if sig == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "missing stripe signature"})
	}

	if err := s.WebhookHandler.HandleWebhook(body, sig); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
