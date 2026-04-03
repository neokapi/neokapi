package billing

import (
	"context"
	"log"
	"time"
)

// UsageHooks provides billing integration points for AI operations.
// All methods are safe to call on a nil receiver (no-op).
type UsageHooks struct {
	Store    BillingStore
	Stripe   *StripeClient
	Notifier *BillingNotifier
	// getOwnerEmail resolves workspace owner email for notifications.
	// Set by the server when wiring up hooks.
	GetOwnerEmail func(ctx context.Context, workspaceID string) string
}

// DeductTokens deducts token credits and reports to Stripe Meters.
// workspaceID is the billing workspace, tokens is the raw token count,
// op describes the operation (e.g. "ai_translation", "bravo_message"),
// and refID is a correlation ID (job ID, message ID, etc.).
func (h *UsageHooks) DeductTokens(ctx context.Context, workspaceID string, tokens int, op, refID string) {
	if h == nil || h.Store == nil {
		return
	}

	credits := TokensToCredits(tokens)
	if err := h.Store.DeductCredits(ctx, workspaceID, credits, op, refID); err != nil {
		log.Printf("billing: deduct credits for %s: %v", workspaceID, err)
	}

	// Check credit thresholds for notifications.
	h.checkCreditThresholds(ctx, workspaceID)

	h.reportMeter(workspaceID, "ai_token_usage", int64(tokens), op)
}

// DeductContainerTime deducts container-time credits and reports to Stripe.
func (h *UsageHooks) DeductContainerTime(ctx context.Context, workspaceID string, duration time.Duration, refID string) {
	if h == nil || h.Store == nil {
		return
	}

	credits := ContainerTimeCredits(duration)
	if err := h.Store.DeductCredits(ctx, workspaceID, credits, "bravo_container", refID); err != nil {
		log.Printf("billing: deduct container credits for %s: %v", workspaceID, err)
	}

	h.reportMeter(workspaceID, "container_time_usage", int64(duration.Seconds()), "bravo_container")
}

// checkCreditThresholds sends notifications when credits cross warning/exhaustion thresholds.
func (h *UsageHooks) checkCreditThresholds(ctx context.Context, workspaceID string) {
	if h.Notifier == nil || h.GetOwnerEmail == nil {
		return
	}

	alloc, err := h.Store.GetCurrentAllocation(ctx, workspaceID)
	if err != nil || alloc == nil || alloc.CreditsTotal == 0 {
		return
	}

	email := h.GetOwnerEmail(ctx, workspaceID)
	if email == "" {
		return
	}

	remaining := alloc.CreditsTotal - alloc.CreditsUsed
	usagePct := float64(alloc.CreditsUsed) / float64(alloc.CreditsTotal)

	if remaining <= 0 {
		h.Notifier.NotifyCreditsExhausted(ctx, email, workspaceID)
	} else if usagePct >= 0.8 {
		h.Notifier.NotifyCreditsWarning(ctx, email, workspaceID, alloc.CreditsUsed, alloc.CreditsTotal)
	}
}

// reportMeter fires a Stripe meter event asynchronously.
func (h *UsageHooks) reportMeter(workspaceID, eventName string, value int64, op string) {
	if h.Stripe == nil {
		return
	}

	// Look up Stripe customer ID from subscription.
	sub, err := h.Store.GetSubscription(context.Background(), workspaceID)
	if err != nil || sub == nil || sub.StripeCustomerID == "" {
		return
	}

	go h.Stripe.ReportMeterEvent(context.Background(), sub.StripeCustomerID, eventName, value, map[string]string{
		"workspace_id":   workspaceID,
		"operation_type": op,
	})
}
