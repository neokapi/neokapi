package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// WebhookHandler processes Stripe webhook events.
type WebhookHandler struct {
	store         BillingStore
	webhookSecret string
}

// NewWebhookHandler creates a WebhookHandler.
func NewWebhookHandler(store BillingStore, webhookSecret string) *WebhookHandler {
	return &WebhookHandler{
		store:         store,
		webhookSecret: webhookSecret,
	}
}

// HandleWebhook verifies the Stripe signature and processes the event.
func (h *WebhookHandler) HandleWebhook(payload []byte, signature string) error {
	event, err := webhook.ConstructEvent(payload, signature, h.webhookSecret)
	if err != nil {
		return fmt.Errorf("verify webhook signature: %w", err)
	}

	ctx := context.Background()

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
		log.Printf("unhandled stripe event type: %s", event.Type)
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
		log.Printf("checkout.session.completed: no workspace_id in metadata")
		return nil
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

	return h.store.RecordBillingEvent(ctx, &BillingEvent{
		WorkspaceID: workspaceID,
		EventType:   "subscription_created",
		Detail:      fmt.Sprintf("Checkout completed, customer=%s", sess.Customer.ID),
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
		log.Printf("subscription.updated: no workspace_id in metadata for sub %s", stripeSub.ID)
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
		log.Printf("subscription.deleted: no workspace_id in metadata for sub %s", stripeSub.ID)
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
		log.Printf("invoice.paid: no workspace_id for invoice %s", inv.ID)
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
		log.Printf("invoice.payment_failed: no workspace_id for invoice %s", inv.ID)
		return nil
	}

	// Update subscription status to past_due.
	existing, err := h.store.GetSubscription(ctx, workspaceID)
	if err == nil && existing != nil {
		existing.Status = "past_due"
		if err := h.store.UpsertSubscription(ctx, existing); err != nil {
			log.Printf("failed to set past_due for workspace %s: %v", workspaceID, err)
		}
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
