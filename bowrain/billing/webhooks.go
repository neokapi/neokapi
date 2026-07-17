package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/neokapi/neokapi/bowrain/analytics"
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

// track fires a fire-and-forget analytics event when a tracker is configured.
func (h *WebhookHandler) track(distinctID, event string, props map[string]any) {
	if h.tracker == nil {
		return
	}
	h.tracker.CaptureEvent(distinctID, event, props)
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
	case "customer.subscription.created", "customer.subscription.updated":
		// Stripe emits `created` for a new subscription and only emits `updated`
		// when something subsequently changes — so a fresh Team purchase can be
		// followed by no `updated` at all. Both carry the same payload shape, and
		// the subscription's price metadata is the authoritative plan signal
		// (planFromSubscription), so both route to the same handler.
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

	// Handle one-time credit pack purchase. Purchased packs are non-expiring
	// (SourcePurchased) — they persist across weekly rollovers until spent, and
	// spend draws from them only after the weekly plan allowance (Epic 004).
	//
	// Granted idempotently on the checkout session id: Stripe delivers webhooks
	// at-least-once and this handler's marker is rolled back on any error, so a
	// naive grant would double-credit a $5 pack on retry. A duplicate delivery
	// returns granted=false and is a no-op success.
	if sess.Metadata["type"] == "credit_pack" {
		if _, err := h.store.GrantPurchasedCredits(ctx, workspaceID, CreditPackCredits, sess.ID); err != nil {
			return fmt.Errorf("grant credits: %w", err)
		}
		h.track(workspaceID, analytics.EventCheckoutCompleted, map[string]any{
			"workspace_id": workspaceID,
			"type":         "credit_pack",
		})
		return nil
	}

	// The plan and seat count come from the session metadata the checkout handler
	// stamped, because the checkout handler is the only place that chose the
	// price. Reading them here is what makes a Team purchase land on Team without
	// waiting for a subsequent subscription event; when that event does arrive it
	// re-derives the same plan from the price's `bowrain_plan` metadata.
	plan := planFromMetadata(sess.Metadata)

	// A checkout that lands while the workspace is on the local card-free
	// trial is a trial conversion (epic 007/018 funnel join).
	wasTrialing := false
	if existing, err := h.store.GetSubscription(ctx, workspaceID); err == nil &&
		existing != nil && existing.Status == "trialing" {
		wasTrialing = true
	}

	sub := &Subscription{
		WorkspaceID:          workspaceID,
		StripeCustomerID:     sess.Customer.ID,
		StripeSubscriptionID: sess.Subscription.ID,
		Plan:                 plan,
		Status:               "active",
		SeatCount:            seatsFromMetadata(sess.Metadata),
	}

	if err := h.store.UpsertSubscription(ctx, sub); err != nil {
		return fmt.Errorf("upsert subscription: %w", err)
	}

	h.syncPlan(ctx, workspaceID, sub.Plan, sess.Customer.ID)

	// Track checkout completed conversion (and trial conversion when the
	// workspace was on the local card-free trial).
	h.track(workspaceID, analytics.EventCheckoutCompleted, map[string]any{
		"workspace_id": workspaceID,
		"customer_id":  sess.Customer.ID,
		"plan":         string(sub.Plan),
		"seats":        sub.SeatCount,
	})
	if wasTrialing {
		h.track(workspaceID, analytics.EventTrialConverted, map[string]any{
			"workspace_id": workspaceID,
			"plan":         string(sub.Plan),
		})
	}

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "subscription_created",
		Detail:      fmt.Sprintf("Checkout completed, plan=%s, seats=%d, customer=%s", sub.Plan, sub.SeatCount, sess.Customer.ID),
	})
}

// planFromMetadata reads the plan the checkout handler stamped on the session.
// An absent or unrecognized value falls back to Pro — the cheapest paid plan —
// so a malformed event can never silently hand out a richer plan than was paid
// for; the subscription event that follows corrects it from the price metadata.
func planFromMetadata(md map[string]string) Plan {
	plan := Plan(md["plan"])
	if ValidPlans[plan] && plan != PlanFree {
		return plan
	}
	return PlanPro
}

// seatsFromMetadata reads the seat count the checkout handler stamped (the
// Stripe subscription quantity it requested). Seats are re-derived from the
// subscription item quantity on every subsequent subscription event.
func seatsFromMetadata(md map[string]string) int {
	n, err := strconv.Atoi(md["seats"])
	if err != nil || n < 1 {
		return 1
	}
	return n
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

	// A canceled subscription is terminal. Stripe does not guarantee event
	// ordering, so a `subscription.updated` (status active) can be delivered
	// AFTER the `subscription.deleted` that canceled it — and a blind upsert would
	// resurrect the workspace to its paid plan with no live Stripe subscription
	// behind it (Team service for free). Reactivation always arrives as a fresh
	// checkout with a NEW subscription id, never as a stale update to the
	// canceled one, so ignoring updates for a locally-terminal subscription is
	// safe. `deleted` clears stripe_subscription_id, so that empty id is the
	// tombstone we key on.
	if existing, err := h.store.GetSubscription(ctx, workspaceID); err == nil &&
		existing != nil && existing.Status == "canceled" && existing.StripeSubscriptionID == "" {
		slog.Info("ignoring subscription.updated for a canceled subscription",
			"workspace", workspaceID, "event_sub", stripeSub.ID)
		return nil
	}

	// Determine plan from price metadata or price ID.
	plan := planFromSubscription(&stripeSub)
	seatCount := 1
	if len(stripeSub.Items.Data) > 0 {
		seatCount = max(int(stripeSub.Items.Data[0].Quantity), 1)
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

	// The plan being cancelled: deleted-subscription payloads may omit items,
	// so guard before deriving from price metadata.
	cancelledPlan := PlanFree
	if stripeSub.Items != nil {
		cancelledPlan = planFromSubscription(&stripeSub)
	}
	h.track(workspaceID, analytics.EventSubscriptionCancelled, map[string]any{
		"workspace_id": workspaceID,
		"plan":         string(cancelledPlan),
	})

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

	h.track(workspaceID, analytics.EventPaymentFailed, map[string]any{
		"workspace_id": workspaceID,
	})

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
