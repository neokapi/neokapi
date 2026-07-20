package billing

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSandboxE2E_RealCheckoutSessions drives the app's OWN Stripe client against a
// Stripe sandbox (test mode) — the real seam that the constructed-event webhook
// tests can't cover: does our CreateCustomer + subscription Checkout, with our
// metadata and seat quantity, actually work against a real key + real Price IDs?
//
// Env-gated: it SKIPS in CI and without credentials. Supply out-of-band (never
// committed), e.g.:
//
//	BOWRAIN_SANDBOX_STRIPE_KEY=sk_test_… \
//	BOWRAIN_SANDBOX_PRO_PRICE_ID=price_… \
//	BOWRAIN_SANDBOX_TEAM_PRICE_ID=price_… \
//	  go test ./billing/ -run TestSandboxE2E -v
//
// It creates real (throwaway) test-mode objects — a customer and two Checkout
// Sessions — and logs their hosted-checkout URLs so a human can complete one with
// card 4242 4242 4242 4242.
func TestSandboxE2E_RealCheckoutSessions(t *testing.T) {
	key := os.Getenv("BOWRAIN_SANDBOX_STRIPE_KEY")
	proPrice := os.Getenv("BOWRAIN_SANDBOX_PRO_PRICE_ID")
	teamPrice := os.Getenv("BOWRAIN_SANDBOX_TEAM_PRICE_ID")
	if !strings.HasPrefix(key, "sk_test_") || proPrice == "" || teamPrice == "" {
		t.Skip("sandbox not configured: set BOWRAIN_SANDBOX_STRIPE_KEY (sk_test_…) + _PRO_/_TEAM_PRICE_ID")
	}
	require.True(t, IsStripeSecretKey(key), "key must satisfy the app's own validation")

	client := NewStripeClient(key)
	ctx := context.Background()

	// The app resolves/creates a per-workspace Stripe customer before checkout.
	cust, err := client.CreateCustomer(ctx, "ws-sandbox", "sandbox@bowrain.cloud", "Sandbox Workspace")
	require.NoError(t, err, "CreateCustomer against sandbox")
	require.True(t, strings.HasPrefix(cust, "cus_"), "customer id = %q", cust)

	const okURL = "https://app.bowrain.cloud/settings/billing?ok=1"
	const cancelURL = "https://app.bowrain.cloud/settings/billing"

	// Pro: single-seat subscription checkout, carrying the metadata the webhook
	// routes on (workspace_id + plan) — the exact shape HandleCreateCheckout sends.
	proURL, err := client.CreateCheckoutSessionWithOptions(cust, proPrice, okURL, cancelURL,
		CheckoutOptions{Metadata: map[string]string{"workspace_id": "ws-sandbox", "plan": "pro"}})
	require.NoError(t, err, "create Pro checkout session")
	assert.Contains(t, proURL, "checkout.stripe.com", "Pro checkout URL")
	t.Logf("Pro checkout (pay with 4242 4242 4242 4242): %s", proURL)

	// Team: per-seat plan — the seat count rides as the subscription quantity.
	teamURL, err := client.CreateCheckoutSessionWithOptions(cust, teamPrice, okURL, cancelURL,
		CheckoutOptions{Quantity: 5, Metadata: map[string]string{"workspace_id": "ws-sandbox", "plan": "team", "seats": "5"}})
	require.NoError(t, err, "create Team checkout session")
	assert.Contains(t, teamURL, "checkout.stripe.com", "Team checkout URL")
	t.Logf("Team (5 seats) checkout: %s", teamURL)
}
