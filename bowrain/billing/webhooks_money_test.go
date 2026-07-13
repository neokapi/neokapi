package billing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// A stale, reordered subscription.updated must not resurrect a canceled
// subscription. Stripe does not guarantee event order, so an `updated` (active)
// can arrive after the `deleted` that canceled it — and a blind upsert would put
// the workspace back on Team with no live subscription behind it: Team service
// for free.
func TestSubscriptionUpdated_DoesNotResurrectCanceled(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	// The workspace has already been canceled (what handleSubscriptionDeleted
	// leaves behind: free/canceled, empty subscription id).
	store.upsertedSubs = append(store.upsertedSubs, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanFree, Status: "canceled", StripeSubscriptionID: "",
	})
	before := len(store.upsertedSubs)

	raw, err := json.Marshal(stripe.Subscription{
		ID:       "sub_old",
		Customer: &stripe.Customer{ID: "cus_1"},
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{"workspace_id": "ws-1"},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{{
				Quantity: 5,
				Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "team"}},
			}},
		},
	})
	require.NoError(t, err)

	require.NoError(t, handler.handleSubscriptionUpdated(t.Context(), stripe.Event{
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: raw},
	}))

	assert.Len(t, store.upsertedSubs, before, "a canceled subscription must not be written back to active")
}

// A genuine reactivation still works: it arrives as a fresh checkout (new
// subscription id, status not canceled), so the terminal-state guard does not
// block a legitimately active subscription's updates.
func TestSubscriptionUpdated_AppliesToActiveSubscription(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	store.upsertedSubs = append(store.upsertedSubs, &Subscription{
		WorkspaceID: "ws-1", Plan: PlanPro, Status: "active", StripeSubscriptionID: "sub_live",
	})

	raw, err := json.Marshal(stripe.Subscription{
		ID:       "sub_live",
		Customer: &stripe.Customer{ID: "cus_1"},
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{"workspace_id": "ws-1"},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{{
				Quantity: 5,
				Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "team"}},
			}},
		},
	})
	require.NoError(t, err)

	require.NoError(t, handler.handleSubscriptionUpdated(t.Context(), stripe.Event{
		Type: "customer.subscription.updated",
		Data: &stripe.EventData{Raw: raw},
	}))

	last := store.upsertedSubs[len(store.upsertedSubs)-1]
	assert.Equal(t, PlanTeam, last.Plan, "an active subscription's update must still apply")
}
