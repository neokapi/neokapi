package billing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"

	"github.com/neokapi/neokapi/bowrain/analytics"
)

// funnelTracker records analytics events fired by the webhook handler.
type funnelTracker struct {
	events []struct {
		distinctID string
		event      string
		props      map[string]any
	}
}

func (f *funnelTracker) CaptureEvent(distinctID, event string, properties map[string]any) {
	f.events = append(f.events, struct {
		distinctID string
		event      string
		props      map[string]any
	}{distinctID, event, properties})
}

func (f *funnelTracker) names() []string {
	out := make([]string, 0, len(f.events))
	for _, e := range f.events {
		out = append(out, e.event)
	}
	return out
}

func (f *funnelTracker) find(event string) map[string]any {
	for _, e := range f.events {
		if e.event == event {
			return e.props
		}
	}
	return nil
}

func checkoutEvent(t *testing.T, md map[string]string) stripe.Event {
	t.Helper()
	sess := stripe.CheckoutSession{
		ID:           "cs_test",
		Customer:     &stripe.Customer{ID: "cus_123"},
		Subscription: &stripe.Subscription{ID: "sub_456"},
		Metadata:     md,
	}
	raw, err := json.Marshal(sess)
	require.NoError(t, err)
	return stripe.Event{Type: "checkout.session.completed", Data: &stripe.EventData{Raw: raw}}
}

// TestFunnel_CheckoutCompleted proves the normalized checkout_completed event
// fires with workspace scope on a subscription checkout.
func TestFunnel_CheckoutCompleted(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")
	tracker := &funnelTracker{}
	handler.SetEventTracker(tracker)

	err := handler.handleCheckoutCompleted(t.Context(),
		checkoutEvent(t, map[string]string{"workspace_id": "ws-1", "plan": "team", "seats": "3"}))
	require.NoError(t, err)

	props := tracker.find(analytics.EventCheckoutCompleted)
	require.NotNil(t, props, "checkout_completed not captured")
	assert.Equal(t, "ws-1", props["workspace_id"])
	assert.Equal(t, "team", props["plan"])
	assert.Equal(t, 3, props["seats"])
	assert.NotContains(t, tracker.names(), analytics.EventTrialConverted,
		"an active (non-trialing) workspace must not emit trial_converted")
}

// TestFunnel_TrialConverted proves a checkout completing while the workspace
// subscription is trialing additionally emits trial_converted.
func TestFunnel_TrialConverted(t *testing.T) {
	store := &recordingStore{}
	// Seed a trialing subscription: the fake's GetSubscription returns the
	// latest upserted sub, which is what a live trial looks like.
	store.upsertedSubs = append(store.upsertedSubs, &Subscription{
		WorkspaceID: "ws-1",
		Plan:        PlanPro,
		Status:      "trialing",
	})
	handler := NewWebhookHandler(store, "whsec_test")
	tracker := &funnelTracker{}
	handler.SetEventTracker(tracker)

	err := handler.handleCheckoutCompleted(t.Context(),
		checkoutEvent(t, map[string]string{"workspace_id": "ws-1", "plan": "pro", "seats": "1"}))
	require.NoError(t, err)

	assert.Contains(t, tracker.names(), analytics.EventCheckoutCompleted)
	props := tracker.find(analytics.EventTrialConverted)
	require.NotNil(t, props, "trial_converted not captured for a trialing workspace")
	assert.Equal(t, "ws-1", props["workspace_id"])
	assert.Equal(t, "pro", props["plan"])
}

// TestFunnel_CreditPackCheckout proves credit-pack checkouts emit
// checkout_completed tagged as credit packs.
func TestFunnel_CreditPackCheckout(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")
	tracker := &funnelTracker{}
	handler.SetEventTracker(tracker)

	err := handler.handleCheckoutCompleted(t.Context(),
		checkoutEvent(t, map[string]string{"workspace_id": "ws-1", "type": "credit_pack"}))
	require.NoError(t, err)

	props := tracker.find(analytics.EventCheckoutCompleted)
	require.NotNil(t, props)
	assert.Equal(t, "credit_pack", props["type"])
}

// TestFunnel_SubscriptionCancelled proves customer.subscription.deleted emits
// subscription_cancelled.
func TestFunnel_SubscriptionCancelled(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")
	tracker := &funnelTracker{}
	handler.SetEventTracker(tracker)

	sub := stripe.Subscription{
		ID:       "sub_456",
		Customer: &stripe.Customer{ID: "cus_123"},
		Metadata: map[string]string{"workspace_id": "ws-1"},
	}
	raw, err := json.Marshal(sub)
	require.NoError(t, err)

	err = handler.handleSubscriptionDeleted(t.Context(),
		stripe.Event{Type: "customer.subscription.deleted", Data: &stripe.EventData{Raw: raw}})
	require.NoError(t, err)

	props := tracker.find(analytics.EventSubscriptionCancelled)
	require.NotNil(t, props, "subscription_cancelled not captured")
	assert.Equal(t, "ws-1", props["workspace_id"])
}

// TestFunnel_PaymentFailed proves invoice.payment_failed emits payment_failed.
func TestFunnel_PaymentFailed(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")
	tracker := &funnelTracker{}
	handler.SetEventTracker(tracker)

	inv := stripe.Invoice{
		ID:       "in_123",
		Metadata: map[string]string{"workspace_id": "ws-1"},
	}
	raw, err := json.Marshal(inv)
	require.NoError(t, err)

	err = handler.handlePaymentFailed(t.Context(),
		stripe.Event{Type: "invoice.payment_failed", Data: &stripe.EventData{Raw: raw}})
	require.NoError(t, err)

	props := tracker.find(analytics.EventPaymentFailed)
	require.NotNil(t, props, "payment_failed not captured")
	assert.Equal(t, "ws-1", props["workspace_id"])
}
