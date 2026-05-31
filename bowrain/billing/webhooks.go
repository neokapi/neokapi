package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// WorkspacePlanSyncer updates the cached plan and Stripe customer ID
// on the workspace record. Implemented by AuthStore.
type WorkspacePlanSyncer interface {
	SyncWorkspacePlan(ctx context.Context, workspaceID, plan, stripeCustomerID string) error
}

// EventTracker captures product analytics events (e.g. PostHog).
type EventTracker interface {
	CaptureEvent(distinctID, event string, properties map[string]any)
}

// WebhookHandler processes Stripe webhook events.
type WebhookHandler struct {
	store         BillingStore
	planSyncer    WorkspacePlanSyncer
	notifier      *BillingNotifier
	tracker       EventTracker
	webhookSecret string
}

// NewWebhookHandler creates a WebhookHandler.
func NewWebhookHandler(store BillingStore, webhookSecret string) *WebhookHandler {
	return &WebhookHandler{
		store:         store,
		webhookSecret: webhookSecret,
	}
}

// SetPlanSyncer configures the workspace plan syncer. When set, webhook
// handlers will update the workspace plan cache after subscription changes.
func (h *WebhookHandler) SetPlanSyncer(syncer WorkspacePlanSyncer) {
	h.planSyncer = syncer
}

// SetNotifier configures the billing email notifier.
func (h *WebhookHandler) SetNotifier(notifier *BillingNotifier) {
	h.notifier = notifier
}

// SetEventTracker configures the event tracker for conversion tracking.
func (h *WebhookHandler) SetEventTracker(tracker EventTracker) {
	h.tracker = tracker
}

// syncPlan updates the cached workspace plan. Errors are logged, not returned,
// because failing to sync the cache should not reject the webhook.
func (h *WebhookHandler) syncPlan(ctx context.Context, workspaceID string, plan Plan, customerID string) {
	if h.planSyncer == nil {
		return
	}
	if err := h.planSyncer.SyncWorkspacePlan(ctx, workspaceID, string(plan), customerID); err != nil {
		slog.Info("failed to sync workspace plan for", "id", workspaceID, "error", err)
	}
}

// HandleWebhook verifies the Stripe signature and processes the event.
//
// Stripe delivers webhooks at-least-once and retries on any non-2xx response,
// so the handler must be idempotent: a duplicate delivery of the same event.ID
// must not re-apply side effects (e.g. double-granting credits). Before
// dispatching, the event ID is recorded in a processed-events table; a
// duplicate ID short-circuits and returns nil (HTTP 200) so Stripe stops
// retrying. If dispatch fails, the marker is rolled back so a subsequent retry
// reprocesses the event.
//
// A nil return maps to HTTP 200 at the server. We return nil for handled
// events, duplicate deliveries, and intentionally-ignored event types; we
// return a non-nil error only for genuine processing failures that should be
// retried.
func (h *WebhookHandler) HandleWebhook(payload []byte, signature string) error {
	event, err := webhook.ConstructEvent(payload, signature, h.webhookSecret)
	if err != nil {
		return fmt.Errorf("verify webhook signature: %w", err)
	}

	ctx := context.Background()

	// Idempotency: claim the event ID before applying any side effects. If it was
	// already processed, this is a duplicate delivery — short-circuit with 200.
	if event.ID != "" {
		alreadyProcessed, err := h.store.MarkStripeEventProcessed(ctx, event.ID, string(event.Type))
		if err != nil {
			return fmt.Errorf("record stripe event: %w", err)
		}
		if alreadyProcessed {
			slog.Info("skipping duplicate stripe event", "id", event.ID, "type", event.Type)
			return nil
		}
	}

	if err := h.dispatch(ctx, event); err != nil {
		// Processing failed: release the idempotency claim so Stripe's retry is
		// reprocessed instead of being silently skipped as a duplicate.
		if event.ID != "" {
			if uerr := h.store.UnmarkStripeEvent(ctx, event.ID); uerr != nil {
				slog.Info("failed to roll back stripe event marker", "id", event.ID, "error", uerr)
			}
		}
		return err
	}
	return nil
}

// dispatch routes a verified event to its type-specific handler. Returning nil
// for unhandled types yields HTTP 200 so Stripe does not retry them forever.
func (h *WebhookHandler) dispatch(ctx context.Context, event stripe.Event) error {
	switch event.Type {
	case "checkout.session.completed":
		return h.handleCheckoutCompleted(ctx, event)
	case "customer.subscription.updated":
		return h.handleSubscriptionUpdated(ctx, event)
	case "customer.subscription.deleted":
		return h.handleSubscriptionDeleted(ctx, event)
	case "invoice.paid":
		return h.handleInvoicePaid(ctx, event)
	case "invoice.payment_failed":
		return h.handlePaymentFailed(ctx, event)
	default:
		slog.Info("unhandled stripe event type:", "value", event.Type)
	}
	return nil
}

func (h *WebhookHandler) handleCheckoutCompleted(ctx context.Context, event stripe.Event) error {
	var sess stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
		return fmt.Errorf("unmarshal checkout session: %w", err)
	}

	workspaceID := sess.Metadata["workspace_id"]
	if workspaceID == "" {
		slog.Info("checkout.session.completed: no workspace_id in metadata")
		return nil
	}

	// Handle one-time credit pack purchase.
	if sess.Metadata["type"] == "credit_pack" {
		creditPackAmount := int64(500_000) // 500K credits per pack
		if err := h.store.GrantCredits(ctx, workspaceID, creditPackAmount, "purchased"); err != nil {
			return fmt.Errorf("grant credits: %w", err)
		}
		return h.store.RecordBillingEvent(ctx, &BillingEvent{
			WorkspaceID: workspaceID,
			EventType:   "credits_purchased",
			Detail:      fmt.Sprintf("Credit pack purchased, +%d credits", creditPackAmount),
		})
	}

	sub := &Subscription{
		WorkspaceID:          workspaceID,
		StripeCustomerID:     sess.Customer.ID,
		StripeSubscriptionID: sess.Subscription.ID,
		Plan:                 PlanPro, // default; updated by subscription.updated
		Status:               "active",
		SeatCount:            1,
	}

	if err := h.store.UpsertSubscription(ctx, sub); err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}

	h.syncPlan(ctx, workspaceID, sub.Plan, sess.Customer.ID)

	// Track checkout completed conversion event.
	if h.tracker != nil {
		h.tracker.CaptureEvent(workspaceID, "billing.checkout_completed", map[string]any{
			"workspace_id": workspaceID,
			"customer_id":  sess.Customer.ID,
		})
	}

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "subscription_created",
		Detail:      "Checkout completed, customer=" + sess.Customer.ID,
	})
}

func (h *WebhookHandler) handleSubscriptionUpdated(ctx context.Context, event stripe.Event) error {
	var stripeSub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &stripeSub); err != nil {
		return fmt.Errorf("unmarshal subscription: %w", err)
	}

	// Look up existing subscription by stripe customer ID.
	workspaceID := stripeSub.Metadata["workspace_id"]
	if workspaceID == "" {
		slog.Info("subscription.updated: no workspace_id in metadata for sub", "value", stripeSub.ID)
		return nil
	}

	// Determine plan from price metadata or price ID.
	plan := planFromSubscription(&stripeSub)
	seatCount := 1
	if len(stripeSub.Items.Data) > 0 {
		seatCount = int(stripeSub.Items.Data[0].Quantity)
		if seatCount < 1 {
			seatCount = 1
		}
	}

	sub := &Subscription{
		WorkspaceID:          workspaceID,
		StripeCustomerID:     stripeSub.Customer.ID,
		StripeSubscriptionID: stripeSub.ID,
		Plan:                 plan,
		Status:               string(stripeSub.Status),
		SeatCount:            seatCount,
	}

	// Extract period from the first subscription item.
	if len(stripeSub.Items.Data) > 0 {
		item := stripeSub.Items.Data[0]
		if item.CurrentPeriodEnd > 0 {
			sub.CurrentPeriodEnd = time.Unix(item.CurrentPeriodEnd, 0)
		}
		if item.CurrentPeriodStart > 0 {
			sub.CurrentPeriodStart = time.Unix(item.CurrentPeriodStart, 0)
		}
	}

	if stripeSub.CancelAt > 0 {
		t := time.Unix(stripeSub.CancelAt, 0)
		sub.CancelAt = &t
	}

	if err := h.store.UpsertSubscription(ctx, sub); err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}

	h.syncPlan(ctx, workspaceID, plan, stripeSub.Customer.ID)

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "subscription_updated",
		Detail:      fmt.Sprintf("Plan=%s, status=%s, seats=%d", plan, stripeSub.Status, seatCount),
	})
}

func (h *WebhookHandler) handleSubscriptionDeleted(ctx context.Context, event stripe.Event) error {
	var stripeSub stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &stripeSub); err != nil {
		return fmt.Errorf("unmarshal subscription: %w", err)
	}

	workspaceID := stripeSub.Metadata["workspace_id"]
	if workspaceID == "" {
		slog.Info("subscription.deleted: no workspace_id in metadata for sub", "value", stripeSub.ID)
		return nil
	}

	// Downgrade to Free.
	sub := &Subscription{
		WorkspaceID:          workspaceID,
		StripeCustomerID:     stripeSub.Customer.ID,
		StripeSubscriptionID: "",
		Plan:                 PlanFree,
		Status:               "canceled",
		SeatCount:            1,
	}

	if err := h.store.UpsertSubscription(ctx, sub); err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}

	h.syncPlan(ctx, workspaceID, PlanFree, stripeSub.Customer.ID)

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "subscription_deleted",
		Detail:      "Subscription canceled, downgraded to free",
	})
}

func (h *WebhookHandler) handleInvoicePaid(ctx context.Context, event stripe.Event) error {
	var inv stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
		return fmt.Errorf("unmarshal invoice: %w", err)
	}

	// Skip non-subscription invoices.
	if inv.Parent == nil || inv.Parent.SubscriptionDetails == nil {
		return nil
	}

	workspaceID := inv.Metadata["workspace_id"]
	if workspaceID == "" {
		// Try subscription metadata.
		if inv.Parent.SubscriptionDetails.Metadata != nil {
			workspaceID = inv.Parent.SubscriptionDetails.Metadata["workspace_id"]
		}
	}
	if workspaceID == "" {
		slog.Info("invoice.paid: no workspace_id for invoice", "value", inv.ID)
		return nil
	}

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "invoice_paid",
		Detail:      fmt.Sprintf("Amount=%d %s", inv.AmountPaid, inv.Currency),
	})
}

func (h *WebhookHandler) handlePaymentFailed(ctx context.Context, event stripe.Event) error {
	var inv stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
		return fmt.Errorf("unmarshal invoice: %w", err)
	}

	workspaceID := inv.Metadata["workspace_id"]
	if workspaceID == "" && inv.Parent != nil && inv.Parent.SubscriptionDetails != nil {
		workspaceID = inv.Parent.SubscriptionDetails.Metadata["workspace_id"]
	}
	if workspaceID == "" {
		slog.Info("invoice.payment_failed: no workspace_id for invoice", "value", inv.ID)
		return nil
	}

	// Update subscription status to past_due.
	existing, err := h.store.GetSubscription(ctx, workspaceID)
	if err == nil && existing != nil {
		existing.Status = "past_due"
		if err := h.store.UpsertSubscription(ctx, existing); err != nil {
			slog.Info("failed to set past_due for workspace", "id", workspaceID, "error", err)
		}
	}

	// Send payment failed email notification.
	if h.notifier != nil && inv.CustomerEmail != "" {
		h.notifier.NotifyPaymentFailed(ctx, inv.CustomerEmail, workspaceID)
	}

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "payment_failed",
		Detail:      fmt.Sprintf("Invoice %s, amount=%d %s", inv.ID, inv.AmountDue, inv.Currency),
	})
}

// planFromSubscription determines the Plan from a Stripe subscription's price metadata.
func planFromSubscription(sub *stripe.Subscription) Plan {
	if len(sub.Items.Data) == 0 {
		return PlanFree
	}

	item := sub.Items.Data[0]
	if item.Price != nil && item.Price.Metadata != nil {
		if p, ok := item.Price.Metadata["bowrain_plan"]; ok {
			plan := Plan(p)
			if ValidPlans[plan] {
				return plan
			}
		}
	}

	// Fallback: if quantity > 1, likely Team.
	if item.Quantity > 1 {
		return PlanTeam
	}
	return PlanPro
}
