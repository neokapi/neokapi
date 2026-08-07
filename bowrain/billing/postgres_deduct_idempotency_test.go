package billing

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPgDeduct_ReferencedChargeAppliesOnce is the money property every metered
// path depends on: the queues are at-least-once, so the same charge arrives
// more than once and must be taken once.
func TestPgDeduct_ReferencedChargeAppliesOnce(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-deduct-once"

	require.NoError(t, store.GrantCredits(ctx, ws, 100_000, SourcePlan))

	for range 4 {
		require.NoError(t, store.DeductCredits(ctx, ws, 30_000, "ai_translation", "job-a:0"))
	}

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(70_000), balance, "four deliveries of one charge cost one charge")

	assert.Equal(t, 1, countLedger(t, store, ws, "ai_translation"),
		"one ledger entry per charge, not per delivery")
}

// TestPgDeduct_DistinctReferencesEachCharge is the other half: idempotency must
// not swallow the next chunk. Chunks of one job differ only by their offset.
func TestPgDeduct_DistinctReferencesEachCharge(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-deduct-distinct"

	require.NoError(t, store.GrantCredits(ctx, ws, 100_000, SourcePlan))

	for _, ref := range []string{"job-a:0", "job-a:50", "job-a:100"} {
		require.NoError(t, store.DeductCredits(ctx, ws, 10_000, "ai_translation", ref))
	}
	// Same reference, different operation: a distinct charge.
	require.NoError(t, store.DeductCredits(ctx, ws, 10_000, "brand_scan", "job-a:0"))

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(60_000), balance)
	assert.Equal(t, 3, countLedger(t, store, ws, "ai_translation"))
	assert.Equal(t, 1, countLedger(t, store, ws, "brand_scan"))
}

// TestPgDeduct_UnreferencedChargesStayIndependent keeps the guard off the
// callers that have no reference to offer: an empty reference is not a key, and
// two such charges are two charges.
func TestPgDeduct_UnreferencedChargesStayIndependent(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-deduct-unref"

	require.NoError(t, store.GrantCredits(ctx, ws, 100_000, SourcePlan))
	require.NoError(t, store.DeductCredits(ctx, ws, 10_000, "ai_translation", ""))
	require.NoError(t, store.DeductCredits(ctx, ws, 10_000, "ai_translation", ""))

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(80_000), balance)
	assert.Equal(t, 2, countLedger(t, store, ws, "ai_translation"))
}

// TestPgDeduct_RepeatAcrossBucketsAppliesOnce covers the cascade: one charge
// can span plan, trial and purchased, writing an entry per bucket. A replay
// re-derives a DIFFERENT split from the balances the first pass left behind, so
// a per-row guard alone would let part of it through.
func TestPgDeduct_RepeatAcrossBucketsAppliesOnce(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-deduct-cascade"

	require.NoError(t, store.GrantCredits(ctx, ws, 10_000, SourcePlan))
	granted, err := store.GrantTrialCredits(ctx, ws, 20_000)
	require.NoError(t, err)
	require.True(t, granted)
	require.NoError(t, store.GrantCredits(ctx, ws, 30_000, SourcePurchased))

	// 25K spans plan (10K) and trial (15K): two entries for one reference.
	require.NoError(t, store.DeductCredits(ctx, ws, 25_000, "ai_translation", "job-b:0"))
	require.Equal(t, 2, countLedger(t, store, ws, "ai_translation"))

	require.NoError(t, store.DeductCredits(ctx, ws, 25_000, "ai_translation", "job-b:0"))

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(35_000), balance, "the replay is free")
	assert.Equal(t, 2, countLedger(t, store, ws, "ai_translation"),
		"no bucket gains a second entry for the same reference")
}

// TestPgDeduct_RepeatAfterMonthRolloverAppliesOnce is the case the unique index
// cannot see. A job's two attempts can straddle a month boundary; the second
// then cascades into a plan allocation the first never touched, so the row it
// would write collides with nothing.
func TestPgDeduct_RepeatAfterMonthRolloverAppliesOnce(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-deduct-rollover"

	require.NoError(t, store.GrantCredits(ctx, ws, 40_000, SourcePlan))
	require.NoError(t, store.DeductCredits(ctx, ws, 25_000, "ai_translation", "job-c:0"))

	// Roll the month over and grant a fresh allowance, then redeliver.
	agePlanAllocation(t, store, ws)
	require.NoError(t, store.GrantCredits(ctx, ws, 40_000, SourcePlan))
	require.NoError(t, store.DeductCredits(ctx, ws, 25_000, "ai_translation", "job-c:0"))

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(40_000), balance, "the new month's allowance is untouched by the replay")
	assert.Equal(t, 1, countLedger(t, store, ws, "ai_translation"))
}

// TestPgDeduct_ConcurrentDuplicatesChargeOnce pins the guard under the race it
// exists for: two workers holding the same redelivered message.
func TestPgDeduct_ConcurrentDuplicatesChargeOnce(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-deduct-race"

	require.NoError(t, store.GrantCredits(ctx, ws, 100_000, SourcePlan))

	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = store.DeductCredits(ctx, ws, 20_000, "ai_translation", "job-d:0")
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(80_000), balance)
	assert.Equal(t, 1, countLedger(t, store, ws, "ai_translation"))
}

// TestPgDeduct_BaselineRepairsAlreadyDoubledLedger covers the databases that
// were double-charged before the guard existed. The baseline has to create a
// unique index over rows that violate it, so it gives the surplus charges back
// first — and a migration that could not do that would fail the boot.
func TestPgDeduct_BaselineRepairsAlreadyDoubledLedger(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-repair"

	require.NoError(t, store.GrantCredits(ctx, ws, 100_000, SourcePlan))
	require.NoError(t, store.DeductCredits(ctx, ws, 30_000, "ai_translation", "job-e:0"))

	// Reproduce what the pre-guard code wrote: the same charge again, twice,
	// bucket and ledger both, exactly as a job retried twice would have left it.
	_, err := store.db.ExecContext(ctx, `DROP INDEX credit_ledger_usage_ref`)
	require.NoError(t, err)
	var allocID string
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT id FROM credit_allocations WHERE workspace_id = $1 AND source = 'plan'`, ws).Scan(&allocID))
	for range 2 {
		_, err = store.db.ExecContext(ctx,
			`UPDATE credit_allocations SET credits_used = credits_used + 30000 WHERE id = $1`, allocID)
		require.NoError(t, err)
		_, err = store.db.ExecContext(ctx,
			`INSERT INTO credit_ledger (workspace_id, allocation_id, amount, balance_after, operation, reference_id)
			 SELECT $1, id, -30000, credits_total - credits_used, 'ai_translation', 'job-e:0'
			 FROM credit_allocations WHERE id = $2`, ws, allocID)
		require.NoError(t, err)
	}
	balance, err := store.CheckCredits(ctx, ws)
	require.NoError(t, err)
	require.Equal(t, int64(10_000), balance, "three charges for one chunk is the bug being repaired")
	require.Equal(t, 3, countLedger(t, store, ws, "ai_translation"))

	// Replay the baseline over it, as a deployment does.
	_, err = store.db.ExecContext(ctx, `TRUNCATE TABLE billing_schema_migrations`)
	require.NoError(t, err)
	repaired, err := NewPgBillingStore(store.db)
	require.NoError(t, err, "a database holding duplicates must still migrate")

	balance, err = repaired.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(70_000), balance, "the surplus charges are given back")
	assert.Equal(t, 1, countLedger(t, repaired, ws, "ai_translation"),
		"one entry survives per reference")

	// And the index it could now build holds.
	require.NoError(t, repaired.DeductCredits(ctx, ws, 30_000, "ai_translation", "job-e:0"))
	balance, err = repaired.CheckCredits(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, int64(70_000), balance)
}

// countLedger counts a workspace's ledger entries for one operation.
func countLedger(t *testing.T, store *PgBillingStore, workspaceID, op string) int {
	t.Helper()
	var n int
	require.NoError(t, store.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM credit_ledger WHERE workspace_id = $1 AND operation = $2`,
		workspaceID, op).Scan(&n))
	return n
}
