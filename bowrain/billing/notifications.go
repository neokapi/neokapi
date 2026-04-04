package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// EmailSender sends a rendered email to a recipient.
type EmailSender interface {
	Send(ctx context.Context, to, subject, htmlBody string) error
}

// BillingNotifier sends billing-related email notifications.
// All methods are safe to call on a nil receiver (no-op).
type BillingNotifier struct {
	Sender EmailSender
	Store  BillingStore
}

// NotifyCreditsWarning sends a warning when credits reach 80% usage.
func (n *BillingNotifier) NotifyCreditsWarning(ctx context.Context, email, workspaceID string, used, total int64) {
	if n == nil || n.Sender == nil {
		return
	}

	resetAt := WeekEnd(time.Now().UTC())
	pct := float64(used) / float64(total) * 100
	subject := "Your Bowrain credits are running low"
	body := fmt.Sprintf(`<p>Your workspace has used %.0f%% of its weekly AI credits (%d of %d).</p>
<p>Credits will reset on %s.</p>
<p>Need more credits? <a href="https://app.bowrain.cloud/pricing">Upgrade your plan</a> or purchase a credit pack.</p>`,
		pct, used, total, resetAt.Format("Monday, January 2"))
	if err := n.Sender.Send(ctx, email, subject, body); err != nil {
		slog.Info("billing: failed to send credits warning email to", "id", email, "error", err)
	}
}

// NotifyCreditsExhausted sends a notice when credits reach zero.
func (n *BillingNotifier) NotifyCreditsExhausted(ctx context.Context, email, workspaceID string) {
	if n == nil || n.Sender == nil {
		return
	}

	resetAt := WeekEnd(time.Now().UTC())
	subject := "Your Bowrain AI credits are exhausted"
	body := fmt.Sprintf(`<p>Your workspace has used all available AI credits for this week.</p>
<p>AI features are paused until credits reset on %s.</p>
<p><a href="https://app.bowrain.cloud/pricing">Upgrade your plan</a> or <a href="https://app.bowrain.cloud/billing">purchase a credit pack</a> for immediate access.</p>`,
		resetAt.Format("Monday, January 2"))
	if err := n.Sender.Send(ctx, email, subject, body); err != nil {
		slog.Info("billing: failed to send credits exhausted email to", "id", email, "error", err)
	}
}

// NotifyPaymentFailed sends a grace period notice when payment fails.
func (n *BillingNotifier) NotifyPaymentFailed(ctx context.Context, email, workspaceID string) {
	if n == nil || n.Sender == nil {
		return
	}

	subject := "Payment failed for your Bowrain subscription"
	body := `<p>We were unable to process your most recent payment.</p>
<p>Please update your payment method within 7 days to avoid service interruption.</p>
<p><a href="https://app.bowrain.cloud/billing">Update payment method</a></p>`
	if err := n.Sender.Send(ctx, email, subject, body); err != nil {
		slog.Info("billing: failed to send payment failed email to", "id", email, "error", err)
	}
}

// NotifySubscriptionChanged sends a confirmation when the subscription changes.
func (n *BillingNotifier) NotifySubscriptionChanged(ctx context.Context, email, workspaceID string, plan Plan, status string) {
	if n == nil || n.Sender == nil {
		return
	}

	subject := "Your Bowrain subscription has been updated"
	body := fmt.Sprintf(`<p>Your subscription has been updated:</p>
<ul>
<li><strong>Plan:</strong> %s</li>
<li><strong>Status:</strong> %s</li>
</ul>
<p><a href="https://app.bowrain.cloud/billing">View billing details</a></p>`,
		plan, status)
	if err := n.Sender.Send(ctx, email, subject, body); err != nil {
		slog.Info("billing: failed to send subscription changed email to", "id", email, "error", err)
	}
}
