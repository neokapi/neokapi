package billing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"
)

// A Team purchase must land the workspace on Team.
//
// It used to land on Pro: handleCheckoutCompleted hardcoded PlanPro and left the
// correction to `customer.subscription.updated` — an event Stripe only sends when
// something *changes* after creation, so for a fresh subscription it may never
// arrive. The buyer paid $20/seat and got the $25 flat plan's limits.
func TestCheckoutCompleted_TeamPurchaseLandsOnTeam(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	raw, err := json.Marshal(stripe.CheckoutSession{
		Customer:     &stripe.Customer{ID: "cus_1"},
		Subscription: &stripe.Subscription{ID: "sub_1"},
		Metadata: map[string]string{
			"workspace_id": "ws-1",
			"plan":         "team",
			"seats":        "5",
		},
	})
	require.NoError(t, err)

	err = handler.handleCheckoutCompleted(t.Context(), stripe.Event{
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	})
	require.NoError(t, err)

	require.Len(t, store.upsertedSubs, 1)
	assert.Equal(t, PlanTeam, store.upsertedSubs[0].Plan)
	assert.Equal(t, 5, store.upsertedSubs[0].SeatCount)
}

// An absent or junk plan in the metadata falls back to Pro — the cheapest paid
// plan — so a malformed event can never hand out a richer plan than was paid for.
func TestCheckoutCompleted_PlanFallsBackToPro(t *testing.T) {
	for _, md := range []map[string]string{
		{"workspace_id": "ws-1"},
		{"workspace_id": "ws-1", "plan": ""},
		{"workspace_id": "ws-1", "plan": "enterprise-lol"},
		{"workspace_id": "ws-1", "plan": "free"}, // free is not a purchase
	} {
		store := &recordingStore{}
		handler := NewWebhookHandler(store, "whsec_test")

		raw, err := json.Marshal(stripe.CheckoutSession{
			Customer:     &stripe.Customer{ID: "cus_1"},
			Subscription: &stripe.Subscription{ID: "sub_1"},
			Metadata:     md,
		})
		require.NoError(t, err)

		require.NoError(t, handler.handleCheckoutCompleted(t.Context(), stripe.Event{
			Type: "checkout.session.completed",
			Data: &stripe.EventData{Raw: raw},
		}))

		require.Len(t, store.upsertedSubs, 1)
		assert.Equal(t, PlanPro, store.upsertedSubs[0].Plan, "metadata %v", md)
		assert.Equal(t, 1, store.upsertedSubs[0].SeatCount, "metadata %v", md)
	}
}

// customer.subscription.created must be handled, not merely logged as unknown:
// it is the event Stripe is guaranteed to send for a new subscription, and it
// carries the price metadata that is the authoritative plan signal.
func TestDispatch_SubscriptionCreatedIsHandled(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	raw, err := json.Marshal(stripe.Subscription{
		ID:       "sub_1",
		Customer: &stripe.Customer{ID: "cus_1"},
		Status:   stripe.SubscriptionStatusActive,
		Metadata: map[string]string{"workspace_id": "ws-1"},
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{{
				Quantity: 3,
				Price:    &stripe.Price{Metadata: map[string]string{"bowrain_plan": "team"}},
			}},
		},
	})
	require.NoError(t, err)

	err = handler.dispatch(t.Context(), stripe.Event{
		Type: "customer.subscription.created",
		Data: &stripe.EventData{Raw: raw},
	})
	require.NoError(t, err)

	require.Len(t, store.upsertedSubs, 1, "customer.subscription.created must reach a handler")
	assert.Equal(t, PlanTeam, store.upsertedSubs[0].Plan)
	assert.Equal(t, 3, store.upsertedSubs[0].SeatCount)
}

// The credit pack grants exactly what the pricing page sells. The code granted
// 500K for a $5 pack advertised as 200K — a 2.5x giveaway that also priced the
// pack below the plan credits it is supposed to top up.
func TestCheckoutCompleted_CreditPackGrantsAdvertisedAmount(t *testing.T) {
	store := &recordingStore{}
	handler := NewWebhookHandler(store, "whsec_test")

	raw, err := json.Marshal(stripe.CheckoutSession{
		Customer: &stripe.Customer{ID: "cus_1"},
		Metadata: map[string]string{"workspace_id": "ws-1", "type": "credit_pack"},
	})
	require.NoError(t, err)

	require.NoError(t, handler.handleCheckoutCompleted(t.Context(), stripe.Event{
		Type: "checkout.session.completed",
		Data: &stripe.EventData{Raw: raw},
	}))

	assert.Equal(t, int64(CreditPackCredits), store.grantedCredits)
	assert.Equal(t, int64(200_000), store.grantedCredits, "the pack the pricing page sells is 200K for $5")
	assert.Empty(t, store.upsertedSubs, "a credit pack must not touch the subscription")
}

func TestIsStripeIdentifier(t *testing.T) {
	// The placeholder Terraform seeds the SSM parameters with must never read as
	// configuration: it is what makes an unprovisioned prod advertise billing.
	assert.False(t, IsStripeSecretKey("CHANGEME"))
	assert.False(t, IsStripeWebhookSecret("CHANGEME"))
	assert.False(t, IsStripePriceID("CHANGEME"))
	assert.True(t, IsPlaceholder("CHANGEME"))

	assert.True(t, IsStripeSecretKey("sk_test_123"))
	assert.True(t, IsStripeSecretKey("sk_live_123"))
	assert.True(t, IsStripeSecretKey("rk_live_123"))
	assert.False(t, IsStripeSecretKey(""))
	assert.False(t, IsStripeSecretKey("pk_live_123"), "a publishable key is not a secret key")

	assert.True(t, IsStripeWebhookSecret("whsec_123"))
	assert.True(t, IsStripePriceID("price_123"))
	assert.False(t, IsStripePriceID("prod_123"), "a product is not a price")
}

func TestDescribePlan(t *testing.T) {
	team := DescribePlan(PlanTeam, true, false)
	assert.Equal(t, PlanTeam, team.ID)
	assert.Equal(t, "Team", team.Name)
	assert.Equal(t, int64(TeamMonthlyCredits), team.MonthlyCredits)
	assert.True(t, team.PerSeat)
	assert.True(t, team.Purchasable)

	pro := DescribePlan(PlanPro, false, true)
	assert.False(t, pro.PerSeat, "Pro is a flat plan, not per-seat")
	assert.False(t, pro.Purchasable, "an unconfigured price means not purchasable")
	assert.True(t, pro.Current)

	free := DescribePlan(PlanFree, false, false)
	assert.Equal(t, int64(0), free.MonthlyCredits, "Free has no recurring allowance — its credits are the one-time trial grant")

	ent := DescribePlan(PlanEnterprise, false, false)
	assert.Equal(t, int64(-1), ent.MonthlyCredits, "Enterprise credits are unlimited")
}
