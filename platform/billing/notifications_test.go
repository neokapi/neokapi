package billing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSendCall struct {
	To, Subject, Body string
}

type mockSender struct {
	calls []mockSendCall
}

func (m *mockSender) Send(_ context.Context, to, subject, body string) error {
	m.calls = append(m.calls, mockSendCall{To: to, Subject: subject, Body: body})
	return nil
}

func TestNotifyCreditsWarning(t *testing.T) {
	sender := &mockSender{}
	n := &BillingNotifier{Sender: sender}

	ctx := context.Background()
	n.NotifyCreditsWarning(ctx, "user@example.com", "ws-1", 800, 1000)

	require.Len(t, sender.calls, 1)
	call := sender.calls[0]

	assert.Equal(t, "user@example.com", call.To)
	assert.Contains(t, call.Subject, "credits")
	assert.Contains(t, strings.ToLower(call.Subject), "running low")

	// Body should contain the percentage
	assert.Contains(t, call.Body, "80%")
	// Body should contain used/total counts
	assert.Contains(t, call.Body, "800")
	assert.Contains(t, call.Body, "1000")
	// Body should contain the reset date
	resetAt := WeekEnd(time.Now().UTC())
	assert.Contains(t, call.Body, resetAt.Format("Monday, January 2"))
	// Body should contain upgrade link
	assert.Contains(t, call.Body, "https://app.bowrain.cloud/pricing")
}

func TestNotifyCreditsWarning_NilReceiver(t *testing.T) {
	var n *BillingNotifier
	assert.NotPanics(t, func() {
		n.NotifyCreditsWarning(context.Background(), "user@example.com", "ws-1", 800, 1000)
	})
}

func TestNotifyCreditsWarning_NilSender(t *testing.T) {
	n := &BillingNotifier{Sender: nil}
	assert.NotPanics(t, func() {
		n.NotifyCreditsWarning(context.Background(), "user@example.com", "ws-1", 800, 1000)
	})
}

func TestNotifyCreditsExhausted(t *testing.T) {
	sender := &mockSender{}
	n := &BillingNotifier{Sender: sender}

	ctx := context.Background()
	n.NotifyCreditsExhausted(ctx, "admin@example.com", "ws-2")

	require.Len(t, sender.calls, 1)
	call := sender.calls[0]

	assert.Equal(t, "admin@example.com", call.To)
	assert.Contains(t, call.Subject, "exhausted")

	// Body should contain the reset date
	resetAt := WeekEnd(time.Now().UTC())
	assert.Contains(t, call.Body, resetAt.Format("Monday, January 2"))
	// Body should contain upgrade link
	assert.Contains(t, call.Body, "https://app.bowrain.cloud/pricing")
	// Body should contain buy/billing link
	assert.Contains(t, call.Body, "https://app.bowrain.cloud/billing")
}

func TestNotifyCreditsExhausted_NilReceiver(t *testing.T) {
	var n *BillingNotifier
	assert.NotPanics(t, func() {
		n.NotifyCreditsExhausted(context.Background(), "user@example.com", "ws-1")
	})
}

func TestNotifyPaymentFailed(t *testing.T) {
	sender := &mockSender{}
	n := &BillingNotifier{Sender: sender}

	ctx := context.Background()
	n.NotifyPaymentFailed(ctx, "billing@example.com", "ws-3")

	require.Len(t, sender.calls, 1)
	call := sender.calls[0]

	assert.Equal(t, "billing@example.com", call.To)
	assert.Contains(t, call.Subject, "Payment failed")

	// Body should mention updating payment method
	assert.Contains(t, call.Body, "update your payment method")
	// Body should contain the billing link
	assert.Contains(t, call.Body, "https://app.bowrain.cloud/billing")
}

func TestNotifyPaymentFailed_NilReceiver(t *testing.T) {
	var n *BillingNotifier
	assert.NotPanics(t, func() {
		n.NotifyPaymentFailed(context.Background(), "user@example.com", "ws-1")
	})
}

func TestNotifySubscriptionChanged(t *testing.T) {
	sender := &mockSender{}
	n := &BillingNotifier{Sender: sender}

	ctx := context.Background()
	n.NotifySubscriptionChanged(ctx, "owner@example.com", "ws-4", PlanPro, "active")

	require.Len(t, sender.calls, 1)
	call := sender.calls[0]

	assert.Equal(t, "owner@example.com", call.To)
	assert.Contains(t, call.Subject, "subscription")
	assert.Contains(t, call.Subject, "updated")

	// Body should contain plan name
	assert.Contains(t, call.Body, string(PlanPro))
	// Body should contain status
	assert.Contains(t, call.Body, "active")
	// Body should contain billing link
	assert.Contains(t, call.Body, "https://app.bowrain.cloud/billing")
}

func TestNotifySubscriptionChanged_NilReceiver(t *testing.T) {
	var n *BillingNotifier
	assert.NotPanics(t, func() {
		n.NotifySubscriptionChanged(context.Background(), "user@example.com", "ws-1", PlanFree, "active")
	})
}
