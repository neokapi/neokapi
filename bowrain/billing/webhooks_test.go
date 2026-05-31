package billing

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

// recordingStore wraps mockBillingStore to capture calls for webhook tests.
type recordingStore struct {
	mockBillingStore

	upsertedSubs []*Subscription
	recordedEvts []*BillingEvent

	// grantedCredits accumulates the total credits granted, used to assert that
	// a credit pack is granted exactly once across duplicate deliveries.
	grantedCredits int64
	grantCalls     int

	// processedEvents records Stripe event IDs that have been claimed, mirroring
	// the processed_stripe_events table's ON CONFLICT DO NOTHING semantics.
	processedEvents map[string]bool

	// grantErr, when set, makes GrantCredits fail to exercise the rollback path.
	grantErr error
}

func (r *recordingStore) UpsertSubscription(_ context.Context, sub *Subscription) error {
	r.upsertedSubs = append(r.upsertedSubs, sub)
	return nil
}

func (r *recordingStore) RecordBillingEvent(_ context.Context, evt *BillingEvent) error {
	r.recordedEvts = append(r.recordedEvts, evt)
	return nil
}

func (r *recordingStore) GrantCredits(_ context.Context, _ string, amount int64, _ string) error {
	if r.grantErr != nil {
		return r.grantErr
	}
	r.grantCalls++
	r.grantedCredits += amount
	return nil
}

func (r *recordingStore) MarkStripeEventProcessed(_ context.Context, eventID, _ string) (bool, error) {
	if r.processedEvents == nil {
		r.processedEvents = make(map[string]bool)
	}
	if r.processedEvents[eventID] {
		return true, nil
	}
	r.processedEvents[eventID] = true
	return false, nil
}

func (r *recordingStore) UnmarkStripeEvent(_ context.Context, eventID string) error {
	delete(r.processedEvents, eventID)
	return nil
}

func (r *recordingStore) GetSubscription(_ context.Context, _ string) (*Subscription, error) {
	if len(r.upsertedSubs) > 0 {
		return r.upsertedSubs[len(r.upsertedSubs)-1], nil
	}
	return &Subscription{
		WorkspaceID:      "ws-1",
		StripeCustomerID: "cus_test",
		Plan:             PlanPro,
		Status:           "active",
		SeatCount:        1,
	}, nil
}

func TestPlanFromSubscription(t *testing.T) {
	tests := []struct {
		name     string
		sub      *stripe.Subscription
		wantPlan Plan
	}{
		{
			"no items defaults to free",
			&stripe.Subscription{Items: &stripe.SubscriptionItemList{}},
			PlanFree,
		},
		{
			"metadata bowrain_plan=pro",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{
							Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "pro"}},
							Quantity: 1,
						},
					},
				},
			},
			PlanPro,
		},
		{
			"metadata bowrain_plan=team",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{
							Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "team"}},
							Quantity: 3,
						},
					},
				},
			},
			PlanTeam,
		},
		{
			"invalid plan in metadata falls back to quantity heuristic",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{
							Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "invalid"}},
							Quantity: 5,
						},
					},
				},
			},
			PlanTeam, // quantity > 1 => team fallback
		},
		{
			"no metadata quantity=1 defaults to pro",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{
							Price:    &stripe.Price{},
							Quantity: 1,
						},
					},
				},
			},
			PlanPro,
		},
		{
			"no metadata quantity>1 defaults to team",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{
							Price:    &stripe.Price{},
							Quantity: 3,
						},
					},
				},
			},
			PlanTeam,
		},
		{
			"nil price falls back to quantity heuristic",
			&stripe.Subscription{
				Items: &stripe.SubscriptionItemList{
					Data: []*stripe.SubscriptionItem{
						{Price: nil, Quantity: 1},
					},
				},
			},
			PlanPro,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planFromSubscription(tt.sub)
			assert.Equal(t, tt.wantPlan, got)
		})
	}
}

func TestHandleCheckoutCompleted(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	sess := stripe.CheckoutSession{
		Customer:     &stripe.Customer{ID: "cus_123"},
		Subscription: &stripe.Subscription{ID: "sub_456"},
		Metadata:     map[string]string{"workspace_id": "ws-1"},
	}
	raw, err := json.Marshal(sess)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleCheckoutCompleted(t.Context(), event)
	require.NoError(t, err)

	require.Len(t, store.upsertedSubs, 1)
	assert.Equal(t, "ws-1", store.upsertedSubs[0].WorkspaceID)
	assert.Equal(t, "cus_123", store.upsertedSubs[0].StripeCustomerID)
	assert.Equal(t, "sub_456", store.upsertedSubs[0].StripeSubscriptionID)
	assert.Equal(t, PlanPro, store.upsertedSubs[0].Plan) // default plan
	assert.Equal(t, "active", store.upsertedSubs[0].Status)

	require.Len(t, store.recordedEvts, 1)
	assert.Equal(t, "subscription_created", store.recordedEvts[0].EventType)
}

func TestHandleCheckoutCompleted_NoWorkspaceID(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	sess := stripe.CheckoutSession{
		Customer:     &stripe.Customer{ID: "cus_123"},
		Subscription: &stripe.Subscription{ID: "sub_456"},
		Metadata:     map[string]string{},
	}
	raw, err := json.Marshal(sess)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleCheckoutCompleted(t.Context(), event)
	require.NoError(t, err)
	assert.Len(t, store.upsertedSubs, 0, "no subscription should be created without workspace_id")
}

func TestHandleSubscriptionUpdated(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	cancelAt := time.Now().Add(30 * 24 * time.Hour).Unix()
	stripeSub := stripe.Subscription{
		ID:       "sub_789",
		Customer: &stripe.Customer{ID: "cus_123"},
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{"workspace_id": "ws-2"},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{
				{
					Price:              &stripe.Price{Metadata: map[string]string{"bowrain_plan": "team"}},
					Quantity:           5,
					CurrentPeriodStart: time.Now().Unix(),
					CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour).Unix(),
				},
			},
		},
		CancelAt: cancelAt,
	}
	raw, err := json.Marshal(stripeSub)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleSubscriptionUpdated(t.Context(), event)
	require.NoError(t, err)

	require.Len(t, store.upsertedSubs, 1)
	sub := store.upsertedSubs[0]
	assert.Equal(t, "ws-2", sub.WorkspaceID)
	assert.Equal(t, PlanTeam, sub.Plan)
	assert.Equal(t, 5, sub.SeatCount)
	assert.Equal(t, "sub_789", sub.StripeSubscriptionID)
	assert.NotNil(t, sub.CancelAt)
	assert.False(t, sub.CurrentPeriodEnd.IsZero())

	require.Len(t, store.recordedEvts, 1)
	assert.Equal(t, "subscription_updated", store.recordedEvts[0].EventType)
}

func TestHandleSubscriptionUpdated_NoWorkspaceID(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	stripeSub := stripe.Subscription{
		ID:       "sub_789",
		Customer: &stripe.Customer{ID: "cus_123"},
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{},
		Items:    &stripe.SubscriptionItemList{},
	}
	raw, err := json.Marshal(stripeSub)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleSubscriptionUpdated(t.Context(), event)
	require.NoError(t, err)
	assert.Len(t, store.upsertedSubs, 0)
}

func TestHandleSubscriptionUpdated_MinSeatCount(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	stripeSub := stripe.Subscription{
		ID:       "sub_min",
		Customer: &stripe.Customer{ID: "cus_min"},
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{"workspace_id": "ws-min"},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{
				{
					Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "pro"}},
					Quantity: 0, // should clamp to 1
				},
			},
		},
	}
	raw, err := json.Marshal(stripeSub)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleSubscriptionUpdated(t.Context(), event)
	require.NoError(t, err)

	require.Len(t, store.upsertedSubs, 1)
	assert.Equal(t, 1, store.upsertedSubs[0].SeatCount, "seat count should be at least 1")
}

func TestHandleSubscriptionDeleted(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	stripeSub := stripe.Subscription{
		ID:       "sub_del",
		Customer: &stripe.Customer{ID: "cus_del"},
		Metadata: map[string]string{"workspace_id": "ws-3"},
		Items:    &stripe.SubscriptionItemList{},
	}
	raw, err := json.Marshal(stripeSub)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleSubscriptionDeleted(t.Context(), event)
	require.NoError(t, err)

	require.Len(t, store.upsertedSubs, 1)
	sub := store.upsertedSubs[0]
	assert.Equal(t, PlanFree, sub.Plan, "should downgrade to free")
	assert.Equal(t, "canceled", sub.Status)
	assert.Empty(t, sub.StripeSubscriptionID)
	assert.Equal(t, 1, sub.SeatCount)

	require.Len(t, store.recordedEvts, 1)
	assert.Equal(t, "subscription_deleted", store.recordedEvts[0].EventType)
}

func TestHandleInvoicePaid(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	inv := stripe.Invoice{
		ID:         "inv_paid",
		AmountPaid: 2500,
		Currency:   "usd",
		Parent: &stripe.InvoiceParent{
			SubscriptionDetails: &stripe.InvoiceParentSubscriptionDetails{
				Metadata: map[string]string{"workspace_id": "ws-4"},
			},
		},
		Metadata: map[string]string{},
	}
	raw, err := json.Marshal(inv)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "invoice.paid",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleInvoicePaid(t.Context(), event)
	require.NoError(t, err)

	require.Len(t, store.recordedEvts, 1)
	assert.Equal(t, "invoice_paid", store.recordedEvts[0].EventType)
	assert.Equal(t, "ws-4", store.recordedEvts[0].WorkspaceID)
}

func TestHandleInvoicePaid_SkipsNonSubscription(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	inv := stripe.Invoice{
		ID:     "inv_skip",
		Parent: nil,
	}
	raw, err := json.Marshal(inv)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "invoice.paid",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handleInvoicePaid(t.Context(), event)
	require.NoError(t, err)
	assert.Len(t, store.recordedEvts, 0, "non-subscription invoice should be skipped")
}

func TestHandlePaymentFailed(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	inv := stripe.Invoice{
		ID:        "inv_fail",
		AmountDue: 2500,
		Currency:  "usd",
		Metadata:  map[string]string{"workspace_id": "ws-5"},
		Parent: &stripe.InvoiceParent{
			SubscriptionDetails: &stripe.InvoiceParentSubscriptionDetails{
				Metadata: map[string]string{"workspace_id": "ws-5"},
			},
		},
	}
	raw, err := json.Marshal(inv)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handlePaymentFailed(t.Context(), event)
	require.NoError(t, err)

	// Should update subscription to past_due.
	require.Len(t, store.upsertedSubs, 1)
	assert.Equal(t, "past_due", store.upsertedSubs[0].Status)

	require.Len(t, store.recordedEvts, 1)
	assert.Equal(t, "payment_failed", store.recordedEvts[0].EventType)
}

func TestHandlePaymentFailed_NoWorkspaceID(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	inv := stripe.Invoice{
		ID:       "inv_nows",
		Parent:   nil,
		Metadata: map[string]string{},
	}
	raw, err := json.Marshal(inv)
	require.NoError(t, err)

	event := stripe.Event{
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{Raw: raw},
	}

	err = handler.handlePaymentFailed(t.Context(), event)
	require.NoError(t, err)
	assert.Len(t, store.recordedEvts, 0)
}

func TestNewWebhookHandler(t *testing.T) {
	store := &mockBillingStore{}
	h := NewWebhookHandler(store, "whsec_secret")
	assert.NotNil(t, h)
	assert.Equal(t, "whsec_secret", h.webhookSecret)
}

// signWebhook builds a payload + Stripe-Signature header that ConstructEvent
// accepts, so tests can exercise the full HandleWebhook path including the
// idempotency guard.
func signWebhook(t *testing.T, secret string, event stripe.Event) ([]byte, string) {
	t.Helper()
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	ts := time.Now()
	sig := webhook.ComputeSignature(ts, payload, secret)
	header := "t=" + strconv.FormatInt(ts.Unix(), 10) + ",v1=" + hex.EncodeToString(sig)
	return payload, header
}

func creditPackEvent(t *testing.T, eventID, workspaceID string) stripe.Event {
	t.Helper()
	sess := stripe.CheckoutSession{
		Customer: &stripe.Customer{ID: "cus_pack"},
		Metadata: map[string]string{"workspace_id": workspaceID, "type": "credit_pack"},
	}
	raw, err := json.Marshal(sess)
	require.NoError(t, err)
	return stripe.Event{
		ID:         eventID,
		APIVersion: stripe.APIVersion,
		Type:       "checkout.session.completed",
		Data:       &stripe.EventData{Raw: raw},
	}
}

func TestHandleWebhook_DuplicateEventGrantsCreditsOnce(t *testing.T) {
	const secret = "whsec_test"
	store := &recordingStore{}
	handler := NewWebhookHandler(store, secret)

	event := creditPackEvent(t, "evt_dup_1", "ws-pack")
	payload, header := signWebhook(t, secret, event)

	// First delivery: credits granted.
	require.NoError(t, handler.HandleWebhook(payload, header))
	assert.Equal(t, 1, store.grantCalls)
	assert.Equal(t, int64(500_000), store.grantedCredits)

	// Duplicate delivery of the SAME event.ID: must be a no-op (200, no grant).
	require.NoError(t, handler.HandleWebhook(payload, header))
	assert.Equal(t, 1, store.grantCalls, "duplicate event must not re-grant credits")
	assert.Equal(t, int64(500_000), store.grantedCredits)

	// And again, to be sure repeated retries stay idempotent.
	require.NoError(t, handler.HandleWebhook(payload, header))
	assert.Equal(t, 1, store.grantCalls)
}

func TestHandleWebhook_DistinctEventsGrantSeparately(t *testing.T) {
	const secret = "whsec_test"
	store := &recordingStore{}
	handler := NewWebhookHandler(store, secret)

	for _, id := range []string{"evt_a", "evt_b"} {
		event := creditPackEvent(t, id, "ws-pack")
		payload, header := signWebhook(t, secret, event)
		require.NoError(t, handler.HandleWebhook(payload, header))
	}

	assert.Equal(t, 2, store.grantCalls, "distinct event IDs each grant once")
	assert.Equal(t, int64(1_000_000), store.grantedCredits)
}

func TestHandleWebhook_FailureRollsBackMarkerAndRetrySucceeds(t *testing.T) {
	const secret = "whsec_test"
	store := &recordingStore{grantErr: assert.AnError}
	handler := NewWebhookHandler(store, secret)

	event := creditPackEvent(t, "evt_retry", "ws-pack")
	payload, header := signWebhook(t, secret, event)

	// First delivery fails inside GrantCredits -> non-nil error (Stripe will retry)
	// and the idempotency marker is rolled back.
	err := handler.HandleWebhook(payload, header)
	require.Error(t, err)
	assert.False(t, store.processedEvents["evt_retry"], "marker must be rolled back on failure")

	// Retry now succeeds and grants exactly once.
	store.grantErr = nil
	require.NoError(t, handler.HandleWebhook(payload, header))
	assert.Equal(t, 1, store.grantCalls)
	assert.Equal(t, int64(500_000), store.grantedCredits)
}

func TestHandleWebhook_UnhandledEventTypeReturnsNil(t *testing.T) {
	const secret = "whsec_test"
	store := &recordingStore{}
	handler := NewWebhookHandler(store, secret)

	event := stripe.Event{
		ID:         "evt_ignored",
		APIVersion: stripe.APIVersion,
		Type:       "customer.created", // not handled
		Data:       &stripe.EventData{Raw: []byte("{}")},
	}
	payload, header := signWebhook(t, secret, event)

	// Intentionally-ignored event types return nil (HTTP 200) so Stripe stops retrying.
	require.NoError(t, handler.HandleWebhook(payload, header))
	assert.True(t, store.processedEvents["evt_ignored"], "ignored event is still marked processed")
}
