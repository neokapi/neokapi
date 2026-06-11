package server

import (
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
)

// HandleGetBilling returns the current plan, subscription status, credit balance,
// and weekly reset countdown for a workspace.
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

	// Ensure weekly credit allocation exists for the current week.
	alloc, _ := billing.EnsureWeeklyAllocation(ctx, s.BillingStore, wsID, sub.Plan)
	var creditsTotal, creditsUsed, creditsRemaining int64
	var weekEnd time.Time
	if alloc != nil {
		creditsTotal = alloc.CreditsTotal
		creditsUsed = alloc.CreditsUsed
		creditsRemaining = max(creditsTotal-creditsUsed, 0)
		weekEnd = alloc.WeekEnd
	} else {
		creditsTotal = billing.CreditsForPlan(sub.Plan)
		creditsRemaining = creditsTotal
	}

	return c.JSON(http.StatusOK, map[string]any{
		"plan":              sub.Plan,
		"status":            sub.Status,
		"credits_total":     creditsTotal,
		"credits_used":      creditsUsed,
		"credits_remaining": creditsRemaining,
		"week_resets_at":    weekEnd,
		"subscription":      sub,
	})
}

// HandleGetBillingUsage returns the credit usage breakdown by operation type.
// GET /api/v1/:ws/billing/usage
func (s *Server) HandleGetBillingUsage(c echo.Context) error {
	if s.BillingStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "billing not configured"})
	}

	wsID, _ := c.Get("workspace_id").(string)

	// Default to current week.
	now := time.Now().UTC()
	from := billing.WeekStart(now)
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

	entries, err := s.BillingStore.GetLedger(c.Request().Context(), wsID, from, to)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Aggregate by operation type.
	byOp := make(map[string]int64)
	for _, e := range entries {
		if e.Amount < 0 {
			byOp[e.Operation] += -e.Amount
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"usage_by_operation": byOp,
		"from":               from,
		"to":                 to,
		"entries":            entries,
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
	from := billing.WeekStart(now)
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

// HandleCreateCheckout creates a Stripe Checkout session and returns the URL.
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
		PriceID    string `json:"price_id"`
		SuccessURL string `json:"success_url"`
		CancelURL  string `json:"cancel_url"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if req.PriceID == "" || req.SuccessURL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "price_id and success_url are required"})
	}

	// Look up existing Stripe customer for this workspace.
	ctx := c.Request().Context()
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
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create stripe customer: " + err.Error()})
		}
	}

	url, err := s.StripeClient.CreateCheckoutSession(ctx, customerID, req.PriceID, req.SuccessURL, req.CancelURL, map[string]string{
		"workspace_id": wsID,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Track checkout started event.
	if s.PostHogClient != nil {
		userID, _ := c.Get("user_id").(string)
		s.PostHogClient.CaptureEvent(userID, "billing.checkout_started", map[string]any{
			"workspace_id": wsID,
			"price_id":     req.PriceID,
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"checkout_url": url,
	})
}

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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to create stripe customer: " + err.Error()})
		}
	}

	url, err := s.StripeClient.CreatePaymentCheckout(ctx, customerID, s.Config.StripeCreditPriceID, req.SuccessURL, req.CancelURL, map[string]string{
		"workspace_id": wsID,
		"type":         "credit_pack",
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

	body, err := io.ReadAll(c.Request().Body)
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
