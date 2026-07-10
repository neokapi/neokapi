package billing

import (
	"context"
	"log/slog"
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
		slog.Info("billing: deduct credits for", "id", workspaceID, "error", err)
	}

	// Check credit thresholds for notifications.
	h.checkCreditThresholds(ctx, workspaceID)

	h.reportMeter(ctx, workspaceID, "ai_token_usage", int64(tokens), op, refID)
}

// DeductContainerTime deducts container-time credits and reports to Stripe.
func (h *UsageHooks) DeductContainerTime(ctx context.Context, workspaceID string, duration time.Duration, refID string) {
	if h == nil || h.Store == nil {
		return
	}

	credits := ContainerTimeCredits(duration)
	if err := h.Store.DeductCredits(ctx, workspaceID, credits, "bravo_container", refID); err != nil {
		slog.Info("billing: deduct container credits for", "id", workspaceID, "error", err)
	}

	h.reportMeter(ctx, workspaceID, "container_time_usage", int64(duration.Seconds()), "bravo_container", refID)
}

// checkCreditThresholds sends notifications when credits cross warning/exhaustion
// thresholds. Exhaustion is judged on the FULL spendable balance (plan +
// purchased) via CheckCredits — not the plan-only allocation — so a workspace
// that just bought a top-up pack is not wrongly told it is out of credits when
// only the weekly plan bucket is depleted (Epic 004). The 80%-used warning still
// tracks the weekly plan allowance, which is what resets and what an upgrade
// affects.
func (h *UsageHooks) checkCreditThresholds(ctx context.Context, workspaceID string) {
	if h.Notifier == nil || h.GetOwnerEmail == nil {
		return
	}

	email := h.GetOwnerEmail(ctx, workspaceID)
	if email == "" {
		return
	}

	// Exhaustion: the true spendable balance across both buckets.
	if spendable, err := h.Store.CheckCredits(ctx, workspaceID); err == nil && spendable <= 0 {
		h.Notifier.NotifyCreditsExhausted(ctx, email, workspaceID)
		return
	}

	// Warning: 80% of the weekly plan allowance consumed.
	alloc, err := h.Store.GetCurrentAllocation(ctx, workspaceID)
	if err != nil || alloc == nil || alloc.CreditsTotal == 0 {
		return
	}
	if usagePct := float64(alloc.CreditsUsed) / float64(alloc.CreditsTotal); usagePct >= 0.8 {
		h.Notifier.NotifyCreditsWarning(ctx, email, workspaceID, alloc.CreditsUsed, alloc.CreditsTotal)
	}
}

// reportMeter fires a Stripe meter event asynchronously. refID is the
// correlation id used to derive the deterministic idempotency key (Epic 004).
func (h *UsageHooks) reportMeter(ctx context.Context, workspaceID, eventName string, value int64, op, refID string) {
	if h.Stripe == nil {
		return
	}

	// Look up Stripe customer ID from subscription. Detached from the request
	// context: this gates a fire-and-forget meter event, so a client
	// disconnect mid-request must not drop the billing lookup.
	sub, err := h.Store.GetSubscription(context.WithoutCancel(ctx), workspaceID)
	if err != nil || sub == nil || sub.StripeCustomerID == "" {
		return
	}

	// Fire-and-forget: detach from the caller's cancellation (the meter event
	// outlives this call) but keep its trace/values.
	go h.Stripe.ReportMeterEvent(context.WithoutCancel(ctx), sub.StripeCustomerID, eventName, value, map[string]string{
		"workspace_id":   workspaceID,
		"operation_type": op,
		"reference_id":   refID,
	})
}
