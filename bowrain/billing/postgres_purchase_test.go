package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The core money bug: a $5 pack must credit 200K exactly once, even though Stripe
// delivers the webhook more than once and the handler rolls its idempotency
// marker back on a partial failure. A grant keyed on the checkout session id is
// what makes the second delivery a no-op instead of a second 200K.
func TestGrantPurchasedCredits_IdempotentOnReference(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-pack"
	const session = "cs_test_123"

	granted, err := store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, session)
	require.NoError(t, err)
	assert.True(t, granted, "first delivery grants")

	// This is the double-grant sequence: the same session delivered again (a
	// retry after the marker was rolled back on a later failed step).
	granted, err = store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, session)
	require.NoError(t, err)
	assert.False(t, granted, "a re-delivery of the same checkout must not grant again")

	// And a third, to be sure repeated retries stay flat.
	granted, err = store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, session)
	require.NoError(t, err)
	assert.False(t, granted)

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(CreditPackCredits), balance, "one $5 pack is 200K credits, not a multiple of it")
}

// Distinct checkouts each grant — the idempotency is per session, not per
// workspace, so a customer who buys two packs gets 400K.
func TestGrantPurchasedCredits_DistinctSessionsAccumulate(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-pack"

	g1, err := store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, "cs_1")
	require.NoError(t, err)
	g2, err := store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, "cs_2")
	require.NoError(t, err)
	assert.True(t, g1)
	assert.True(t, g2)

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(2*CreditPackCredits), balance)
}

// A grant with no reference is refused rather than granted unsafely: without a
// key there is no way to dedupe a retry, so granting would reopen the very
// double-grant this method exists to close.
func TestGrantPurchasedCredits_RequiresReference(t *testing.T) {
	store := newCreditTestStore(t)

	_, err := store.GrantPurchasedCredits(t.Context(), "ws-1", CreditPackCredits, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference id")
}

// The grant records exactly one credits_purchased event, in the same transaction
// as the credits — so the audit trail can't drift from the money.
func TestGrantPurchasedCredits_RecordsEventOnce(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-pack"

	_, err := store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, "cs_1")
	require.NoError(t, err)
	_, err = store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, "cs_1") // duplicate
	require.NoError(t, err)

	events, err := store.ListBillingEvents(ctx, 10, 0, "credits_purchased")
	require.NoError(t, err)
	assert.Len(t, events, 1, "a duplicate delivery must not record a second purchase event")
}
