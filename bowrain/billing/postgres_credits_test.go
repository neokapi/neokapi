package billing

import (
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

// TestPgCredits_PurchasedCounted_AndCascade covers the core Epic 004 fix: a
// purchased pack is counted in the spendable balance, and a deduction draws from
// the weekly plan first, then purchased.
func TestPgCredits_PurchasedCounted_AndCascade(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-cascade"

	require.NoError(t, store.GrantCredits(ctx, ws, 50_000, SourcePlan))
	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))

	// Balance reflects plan + purchased (the bug: purchased was never counted).
	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(550_000), remaining, "balance must sum plan + purchased")

	// Deduct 60K: plan (50K) is exhausted first, then 10K is drawn from purchased.
	require.NoError(t, store.DeductCredits(ctx, ws, 60_000, "ai_translation", "job:0"))

	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(490_000), remaining, "cascade leaves purchased 490K after draining plan")

	// The weekly plan bucket is exhausted (drained to zero, not negative).
	planAlloc, err := store.GetCurrentAllocation(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(50_000), planAlloc.CreditsTotal)
	assert.Equal(t, int64(50_000), planAlloc.CreditsUsed)
	assert.Equal(t, SourcePlan, planAlloc.Source, "GetCurrentAllocation stays plan-only")

	// The cascade wrote one debit ledger entry per bucket touched.
	from, to := ledgerWindow()
	entries, err := store.GetLedger(ctx, ws, from, to)
	require.NoError(t, err)
	var debitTotal int64
	debits := 0
	for _, e := range entries {
		if e.ReferenceID == "job:0" && e.Amount < 0 {
			debits++
			debitTotal += -e.Amount
		}
	}
	assert.Equal(t, 2, debits, "one debit per bucket (plan + purchased)")
	assert.Equal(t, int64(60_000), debitTotal, "debits sum to the deducted amount")
}

// TestPgCredits_PurchasedSurvivesWeekRollover proves purchased packs are
// non-expiring: after the weekly plan allocation rolls over, purchased credits
// persist and a fresh weekly plan stacks on top of them.
func TestPgCredits_PurchasedSurvivesWeekRollover(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-rollover"

	require.NoError(t, store.GrantCredits(ctx, ws, 50_000, SourcePlan))
	require.NoError(t, store.GrantCredits(ctx, ws, 500_000, SourcePurchased))
	require.NoError(t, store.DeductCredits(ctx, ws, 20_000, "ai_translation", "job:0"))

	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(530_000), remaining) // 30K plan + 500K purchased

	// Simulate a week rollover by aging the plan row into the previous week, so
	// it no longer counts as "this week's allowance".
	_, err = store.db.ExecContext(ctx,
		`UPDATE credit_allocations
		 SET week_start = week_start - INTERVAL '7 days',
		     week_end   = week_end   - INTERVAL '7 days'
		 WHERE workspace_id = $1 AND source = 'plan'`, ws)
	require.NoError(t, err)

	// The expired plan allowance is gone; purchased credits survive untouched.
	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(500_000), remaining, "purchased credits survive the rollover")

	_, err = store.GetCurrentAllocation(ctx, ws)
	require.Error(t, err, "no current-week plan allocation after rollover")

	// A fresh weekly grant stacks on top of the surviving purchased balance.
	require.NoError(t, store.GrantCredits(ctx, ws, 50_000, SourcePlan))
	remaining, err = store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(550_000), remaining, "fresh plan + surviving purchased")
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

// TestPgCredits_OverageRecordedNotLost checks that when both buckets are
// exhausted the overage is still recorded (the balance goes negative) rather
// than being silently dropped.
func TestPgCredits_OverageRecordedNotLost(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-overflow"

	require.NoError(t, store.GrantCredits(ctx, ws, 100, SourcePlan))
	require.NoError(t, store.GrantCredits(ctx, ws, 50, SourcePurchased))

	require.NoError(t, store.DeductCredits(ctx, ws, 200, "ai_translation", "job:0"))

	remaining, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(-50), remaining, "overage recorded as negative balance, not lost")
}

// TestPgCredits_CheckCreditsNoAllocation preserves the "no allocation" signal so
// callers (enqueue pre-check, QuotaGuard) degrade to allowing the request.
func TestPgCredits_CheckCreditsNoAllocation(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()

	_, err := store.CheckCredits(ctx, "ws-never-seen")
	require.Error(t, err, "no plan and no purchased credits returns an error")
}

// TestPgCredits_PurchasedOnlyBalance covers a workspace that has purchased
// credits but no weekly plan allocation yet (balance must still surface).
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
