package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGrantCredits_AccumulatingLedgerMatchesAllocation guards the admin-grant
// audit fix: repeated grants into the non-expiring bucket accumulate onto one
// allocation, so every ledger row must name THAT allocation and carry the
// running balance — not a never-inserted id with balance_after equal to just
// the latest grant. HandleAdminGrantCredits grants into SourcePurchased, so
// this is the path an operator hits from the second grant onward.
func TestGrantCredits_AccumulatingLedgerMatchesAllocation(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := context.Background()
	const ws = "ws-grant-audit"

	require.NoError(t, store.GrantCredits(ctx, ws, 100, SourcePurchased))
	require.NoError(t, store.GrantCredits(ctx, ws, 50, SourcePurchased))

	// The purchased bucket accumulated onto one allocation.
	var allocID string
	var total int64
	require.NoError(t, store.db.QueryRowContext(ctx,
		`SELECT id, credits_total FROM credit_allocations WHERE workspace_id=$1 AND source=$2`,
		ws, SourcePurchased).Scan(&allocID, &total))
	require.Equal(t, int64(150), total)

	from, to := ledgerWindow()
	entries, err := store.GetLedger(ctx, ws, from, to)
	require.NoError(t, err)

	var grants []LedgerEntry
	balances := map[int64]bool{}
	for _, e := range entries {
		if e.Operation != "grant" {
			continue
		}
		grants = append(grants, e)
		balances[e.BalanceAfter] = true

		// Every ledger row must reference an allocation that actually exists —
		// the pre-fix bug recorded a freshly generated id that was never inserted.
		assert.Equal(t, allocID, e.AllocationID,
			"grant ledger row must name the accumulating allocation")
		var exists bool
		require.NoError(t, store.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM credit_allocations WHERE id=$1)`, e.AllocationID).Scan(&exists))
		assert.True(t, exists, "ledger row %d references a nonexistent allocation %q", e.ID, e.AllocationID)
	}

	require.Len(t, grants, 2, "each grant writes one ledger row")
	assert.True(t, balances[100], "the first grant's balance_after must be 100")
	assert.True(t, balances[150], "the second grant's balance_after must be the running total 150, not 50")
}
