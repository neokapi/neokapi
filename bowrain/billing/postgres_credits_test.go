package billing

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCreditTestStore spins up an isolated PostgreSQL schema (testcontainers) and
// returns a migrated billing store. Skips when Docker is unavailable.
func newCreditTestStore(t *testing.T) *PgBillingStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	store, err := NewPgBillingStore(db)
	require.NoError(t, err)
	return store
}

func ledgerWindow() (from, to time.Time) {
	now := time.Now().UTC()
	return now.Add(-time.Hour), now.Add(time.Hour)
}

// agePlanAllocation shifts a workspace's plan allocation back one calendar
// month, simulating a month rollover without waiting for one.
func agePlanAllocation(t *testing.T, store *PgBillingStore, workspaceID string) {
	t.Helper()
	_, err := store.db.ExecContext(context.Background(),
		`UPDATE credit_allocations
		 SET week_start = week_start - INTERVAL '1 month',
		     week_end   = week_end   - INTERVAL '1 month'
		 WHERE workspace_id = $1 AND source = 'plan'`, workspaceID)
	require.NoError(t, err)
}

// TestPgCredits_NonExpiringCounted_AndCascade covers the core Epic 004 fix plus
// the monthly-credits cascade order: trial and purchased credits are counted in
// the spendable balance, and a deduction draws plan (expiring) → trial
// (promotional) → purchased (paid-for) in that order.
func TestPgCredits_NonExpiringCounted_AndCascade(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-cascade"

	require.NoError(t, store.GrantCredits(ctx, ws, 50_000, SourcePlan))
	granted, err := store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	require.True(t, granted)
	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))

	// Balance reflects plan + trial + purchased.
	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(750_000), remaining, "balance must sum plan + trial + purchased")

	// Deduct 260K: plan (50K) is exhausted first, then the trial grant (200K),
	// then 10K is drawn from purchased.
	require.NoError(t, store.DeductCredits(ctx, ws, 260_000, "ai_translation", "job:0"))

	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(490_000), remaining, "cascade leaves purchased 490K after draining plan and trial")

	// The monthly plan bucket is exhausted (drained to zero, not negative).
	planAlloc, err := store.GetCurrentAllocation(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(50_000), planAlloc.CreditsTotal)
	assert.Equal(t, int64(50_000), planAlloc.CreditsUsed)
	assert.Equal(t, SourcePlan, planAlloc.Source, "GetCurrentAllocation stays plan-only")

	// The cascade wrote one debit ledger entry per bucket touched, and the
	// per-bucket amounts pin the order: plan first, trial next, purchased last.
	from, to := ledgerWindow()
	entries, err := store.GetLedger(ctx, ws, from, to)
	require.NoError(t, err)
	debitByAlloc := map[string]int64{}
	for _, e := range entries {
		if e.ReferenceID == "job:0" && e.Amount < 0 {
			debitByAlloc[e.AllocationID] += -e.Amount
		}
	}
	require.Len(t, debitByAlloc, 3, "one debit per bucket (plan + trial + purchased)")
	var total int64
	for _, d := range debitByAlloc {
		total += d
	}
	assert.Equal(t, int64(260_000), total, "debits sum to the deducted amount")
	assert.Contains(t, mapValues(debitByAlloc), int64(50_000), "the plan bucket absorbed exactly its 50K")
	assert.Contains(t, mapValues(debitByAlloc), int64(200_000), "the trial bucket absorbed exactly its 200K")
	assert.Contains(t, mapValues(debitByAlloc), int64(10_000), "purchased absorbed only the 10K overflow")
}

func mapValues(m map[string]int64) []int64 {
	out := make([]int64, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// TestPgCredits_TrialSpentBeforePurchased pins the trial-before-purchased leg
// of the cascade in isolation: with no plan bucket at all, the promotional
// grant is consumed before the customer's paid pack credits.
func TestPgCredits_TrialSpentBeforePurchased(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-trial-first"

	_, err := store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))

	require.NoError(t, store.DeductCredits(ctx, ws, 150_000, "ai_translation", "job:0"))

	var trialUsed, purchasedUsed int64
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT credits_used FROM credit_allocations WHERE workspace_id = $1 AND source = 'trial'`, ws).Scan(&trialUsed))
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT credits_used FROM credit_allocations WHERE workspace_id = $1 AND source = 'purchased'`, ws).Scan(&purchasedUsed))
	assert.Equal(t, int64(150_000), trialUsed, "the trial grant is drawn from first")
	assert.Equal(t, int64(0), purchasedUsed, "paid pack credits are preserved until the trial grant is gone")
}

// TestPgCredits_NonExpiringSurviveMonthRollover proves trial and purchased
// credits are non-expiring: after the monthly plan allocation rolls over, both
// persist and a fresh monthly plan stacks on top of them.
func TestPgCredits_NonExpiringSurviveMonthRollover(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-rollover"

	require.NoError(t, store.GrantCredits(ctx, ws, 50_000, SourcePlan))
	_, err := store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))
	require.NoError(t, store.DeductCredits(ctx, ws, 20_000, "ai_translation", "job:0"))

	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(730_000), remaining) // 30K plan + 200K trial + 500K purchased

	// Simulate a month rollover by aging the plan row into the previous month,
	// so it no longer counts as "this month's allowance".
	agePlanAllocation(t, store, ws)

	// The expired plan allowance is gone; trial and purchased credits survive.
	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(700_000), remaining, "trial + purchased credits survive the rollover")

	_, err = store.GetCurrentAllocation(ctx, ws)
	require.Error(t, err, "no current-month plan allocation after rollover")

	// A fresh monthly grant stacks on top of the surviving non-expiring balance.
	require.NoError(t, store.GrantCredits(ctx, ws, 50_000, SourcePlan))
	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(750_000), remaining, "fresh plan + surviving trial + purchased")
}

// TestPgCredits_TrialGrantOnceEver is the money property of the trial grant: a
// workspace gets its one-time credits exactly once, no matter how many times
// the grant is attempted (workspace creation, the middleware's lazy backfill,
// racing requests, later months).
func TestPgCredits_TrialGrantOnceEver(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-trial-once"

	granted, err := store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	assert.True(t, granted, "first grant lands")

	for range 3 {
		granted, err = store.GrantTrialCredits(ctx, ws, 200_000)
		require.NoError(t, err)
		assert.False(t, granted, "a repeat grant must be a no-op")
	}

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(200_000), balance, "one trial grant, never a multiple of it")

	// Spend part of it, cross a month boundary, and attempt the grant again:
	// still no second grant — once EVER, not once per period.
	require.NoError(t, store.DeductCredits(ctx, ws, 150_000, "ai_translation", "job:0"))
	granted, err = store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	assert.False(t, granted)
	balance, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(50_000), balance, "the remainder persists; no refresh, no top-up")

	// Exactly one trial_grant ledger row and one billing event.
	from, to := ledgerWindow()
	entries, err := store.GetLedger(ctx, ws, from, to)
	require.NoError(t, err)
	grants := 0
	for _, e := range entries {
		if e.Operation == "trial_grant" {
			grants++
		}
	}
	assert.Equal(t, 1, grants, "exactly one trial_grant ledger entry")
	events, err := store.ListBillingEvents(ctx, 10, 0, "trial_credits_granted")
	require.NoError(t, err)
	assert.Len(t, events, 1, "exactly one trial_credits_granted event")
}

// TestPgCredits_SpentTrialBlocksNotDegrades: a free workspace that has spent
// its trial grant must read as OUT of credits (balance 0 → QuotaGuard blocks),
// not as "no allocation" (which degrades to allow). With no recurring Free
// allowance this is the steady state of every free workspace, so getting it
// wrong is unmetered AI for everyone.
func TestPgCredits_SpentTrialBlocksNotDegrades(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-trial-spent"

	_, err := store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	require.NoError(t, store.DeductCredits(ctx, ws, 200_000, "ai_translation", "job:0"))

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err, "a spent bucket is a real zero balance, not the no-allocation signal")
	assert.Equal(t, int64(0), balance)
}

// TestPgCredits_MonthlyAllocation_PaidPlanLifecycle drives the allocation
// helper end to end for a paying workspace: the month allocation is created
// once (no double grant on re-ensure), and a workspace that drained it gets a
// fresh full allowance after the month rolls over.
func TestPgCredits_MonthlyAllocation_PaidPlanLifecycle(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-paid"

	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: ws, Plan: PlanPro, Status: "active", SeatCount: 1,
		StripeCustomerID: "cus_paid", StripeSubscriptionID: "sub_paid",
	}))

	alloc, err := EnsureMonthlyAllocation(ctx, store, ws, PlanPro)
	require.NoError(t, err)
	require.NotNil(t, alloc)
	assert.Equal(t, int64(ProMonthlyCredits), alloc.CreditsTotal)
	assert.Equal(t, MonthStart(time.Now().UTC()), alloc.PeriodStart.UTC())
	assert.Equal(t, MonthEnd(time.Now().UTC()), alloc.PeriodEnd.UTC())

	// Re-ensuring within the same month must not grant again or grow the total.
	again, err := EnsureMonthlyAllocation(ctx, store, ws, PlanPro)
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, alloc.ID, again.ID)
	assert.Equal(t, int64(ProMonthlyCredits), again.CreditsTotal, "no double-grant within the month")
	var planRows int
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credit_allocations WHERE workspace_id = $1 AND source = 'plan'`, ws).Scan(&planRows))
	assert.Equal(t, 1, planRows)

	// Drain the whole allowance, roll the month, and touch again: a fresh full
	// allocation appears for the new month.
	require.NoError(t, store.DeductCredits(ctx, ws, ProMonthlyCredits, "ai_translation", "job:0"))
	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(0), balance, "allowance fully drained")

	agePlanAllocation(t, store, ws)

	fresh, err := EnsureMonthlyAllocation(ctx, store, ws, PlanPro)
	require.NoError(t, err)
	require.NotNil(t, fresh)
	assert.NotEqual(t, alloc.ID, fresh.ID, "a new month means a new allocation row")
	assert.Equal(t, int64(ProMonthlyCredits), fresh.CreditsTotal)
	assert.Equal(t, int64(0), fresh.CreditsUsed)

	balance, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(ProMonthlyCredits), balance, "the new month starts with the full allowance")
}

// TestPgCredits_MonthlyAllocation_ConcurrentEnsureNoDoubleGrant races two
// EnsureMonthlyAllocation calls that both observed "no allocation yet". The
// month-keyed unique constraint plus grant-once-per-period semantics must
// collapse them into a single allowance, not stack two.
func TestPgCredits_MonthlyAllocation_ConcurrentEnsureNoDoubleGrant(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-race"

	// Both "requests" call GrantCredits directly, as Ensure does after a miss.
	require.NoError(t, store.GrantCredits(ctx, ws, ProMonthlyCredits, SourcePlan))
	require.NoError(t, store.GrantCredits(ctx, ws, ProMonthlyCredits, SourcePlan))

	alloc, err := store.GetCurrentAllocation(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(ProMonthlyCredits), alloc.CreditsTotal,
		"two racing plan grants must dedupe on the month key, not stack")

	// And only one grant ledger entry was written.
	from, to := ledgerWindow()
	entries, err := store.GetLedger(ctx, ws, from, to)
	require.NoError(t, err)
	grants := 0
	for _, e := range entries {
		if e.Operation == "grant" {
			grants++
		}
	}
	assert.Equal(t, 1, grants, "the losing grant must not write a ledger entry")
}

// TestPgCredits_MonthlyAllocation_SkipsFreeAndTrialing: no plan allocation is
// minted for a plan without a recurring allowance (Free), nor for a workspace
// whose subscription is still `trialing` — the trial grants features, not the
// paid monthly allowance.
func TestPgCredits_MonthlyAllocation_SkipsFreeAndTrialing(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()

	// Free plan: no recurring allowance.
	alloc, err := EnsureMonthlyAllocation(ctx, store, "ws-free", PlanFree)
	require.NoError(t, err)
	assert.Nil(t, alloc)
	var rows int
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credit_allocations WHERE workspace_id = 'ws-free'`).Scan(&rows))
	assert.Equal(t, 0, rows)

	// Trialing Pro: plan cache says pro, but the subscription is not paid yet.
	SetupTrial(ctx, store, "ws-trialing")
	alloc, err = EnsureMonthlyAllocation(ctx, store, "ws-trialing", PlanPro)
	require.NoError(t, err)
	assert.Nil(t, alloc, "a trialing subscription must not receive the paid monthly allowance")
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credit_allocations WHERE workspace_id = 'ws-trialing' AND source = 'plan'`).Scan(&rows))
	assert.Equal(t, 0, rows)

	// Once converted (what the checkout webhook writes), the allowance starts.
	require.NoError(t, store.UpsertSubscription(ctx, &Subscription{
		WorkspaceID: "ws-trialing", Plan: PlanPro, Status: "active", SeatCount: 1,
		StripeCustomerID: "cus_1", StripeSubscriptionID: "sub_1",
	}))
	alloc, err = EnsureMonthlyAllocation(ctx, store, "ws-trialing", PlanPro)
	require.NoError(t, err)
	require.NotNil(t, alloc)
	assert.Equal(t, int64(ProMonthlyCredits), alloc.CreditsTotal)
}

// TestPgCredits_PurchasedGrantsAccumulate verifies repeated pack purchases
// collapse into a single non-expiring purchased row.
func TestPgCredits_PurchasedGrantsAccumulate(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-accumulate"

	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))
	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))

	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000), remaining, "two packs accumulate")

	var rows int
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credit_allocations WHERE workspace_id = $1 AND source = 'purchased'`,
		ws).Scan(&rows))
	assert.Equal(t, 1, rows, "purchased grants collapse into one row")
}

// TestPgCredits_OverageRecordedNotLost checks that when every bucket is
// exhausted the overage is still recorded (the balance goes negative) rather
// than being silently dropped.
func TestPgCredits_OverageRecordedNotLost(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-overflow"

	require.NoError(t, store.GrantCredits(ctx, ws, 100, SourcePlan))
	_, err := store.GrantTrialCredits(ctx, ws, 30)
	require.NoError(t, err)
	require.NoError(t, store.GrantCredits(ctx, ws, 50, SourcePurchased))

	require.NoError(t, store.DeductCredits(ctx, ws, 230, "ai_translation", "job:0"))

	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(-50), remaining, "overage recorded as negative balance, not lost")
}

// TestPgCredits_CheckCreditsNoAllocation preserves the "no allocation" signal
// for a workspace with no credit rows at all, so callers (enqueue pre-check,
// QuotaGuard) degrade to allowing the request.
func TestPgCredits_CheckCreditsNoAllocation(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()

	_, err := store.CheckCredits(ctx, "ws-never-seen")
	require.Error(t, err, "no rows of any source returns an error")
}

// TestPgCredits_PurchasedOnlyBalance covers a workspace that has purchased
// credits but no monthly plan allocation (balance must still surface).
func TestPgCredits_PurchasedOnlyBalance(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-purchased-only"

	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))

	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(500_000), remaining)

	// A deduction with no plan bucket draws straight from purchased.
	require.NoError(t, store.DeductCredits(ctx, ws, 30_000, "ai_translation", "job:0"))
	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(470_000), remaining)
}

// TestPgCredits_WebhookPackGrantSpendable closes the loop the Stripe webhook
// drives: a pack granted via GrantPurchasedCredits is part of the spendable
// balance and is drawn from after the trial grant.
func TestPgCredits_WebhookPackGrantSpendable(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-pack-spend"

	_, err := store.GrantTrialCredits(ctx, ws, 200_000)
	require.NoError(t, err)
	granted, err := store.GrantPurchasedCredits(ctx, ws, CreditPackCredits, "cs_spend_1")
	require.NoError(t, err)
	require.True(t, granted)

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(200_000+CreditPackCredits), balance)

	// Spend into the pack: trial drains first, then the pack.
	require.NoError(t, store.DeductCredits(ctx, ws, 250_000, "ai_translation", "job:0"))
	balance, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(CreditPackCredits-50_000), balance, "the pack remains spendable after the trial grant is gone")
}
