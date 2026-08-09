package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedLedger grants a workspace credits and spends them across two operations,
// producing a ledger with both credits and debits.
func seedLedger(t *testing.T, store *PgBillingStore, ws string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, store.GrantCredits(ctx, ws, 1_000_000, SourcePlan))
	for i := range 6 {
		require.NoError(t, store.DeductCredits(ctx, ws, 100, "ai_translation", "job:"+string(rune('a'+i))))
	}
	for i := range 4 {
		require.NoError(t, store.DeductCredits(ctx, ws, 10, "bravo_message", "msg:"+string(rune('a'+i))))
	}
}

func TestPgGetLedgerPage(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-ledger-page"
	seedLedger(t, store, ws)
	from, to := ledgerWindow()

	t.Run("pages the window and reports the full total", func(t *testing.T) {
		page, err := store.GetLedgerPage(ctx, ws, LedgerQuery{From: from, To: to, Limit: 4})
		require.NoError(t, err)
		assert.Len(t, page.Entries, 4)
		assert.Equal(t, 11, page.Total, "1 grant + 10 debits")
		assert.Equal(t, 4, page.Limit)
		assert.Equal(t, 0, page.Offset)
	})

	t.Run("offset walks the same ordering without repeats", func(t *testing.T) {
		seen := map[int64]int{}
		for offset := 0; offset < 11; offset += 4 {
			page, err := store.GetLedgerPage(ctx, ws, LedgerQuery{From: from, To: to, Limit: 4, Offset: offset})
			require.NoError(t, err)
			for _, e := range page.Entries {
				seen[e.ID]++
			}
		}
		assert.Len(t, seen, 11)
		for entryID, n := range seen {
			assert.Equal(t, 1, n, "entry %d appeared more than once", entryID)
		}
	})

	t.Run("the operation filter narrows entries and total together", func(t *testing.T) {
		page, err := store.GetLedgerPage(ctx, ws, LedgerQuery{
			From: from, To: to, Operation: "bravo_message", Limit: 50,
		})
		require.NoError(t, err)
		assert.Equal(t, 4, page.Total)
		require.Len(t, page.Entries, 4)
		for _, e := range page.Entries {
			assert.Equal(t, "bravo_message", e.Operation)
		}
	})

	t.Run("an offset past the end is an empty page, not an error", func(t *testing.T) {
		page, err := store.GetLedgerPage(ctx, ws, LedgerQuery{From: from, To: to, Limit: 4, Offset: 100})
		require.NoError(t, err)
		assert.Empty(t, page.Entries)
		assert.Equal(t, 11, page.Total)
	})

	t.Run("limit is clamped", func(t *testing.T) {
		page, err := store.GetLedgerPage(ctx, ws, LedgerQuery{From: from, To: to, Limit: 10_000})
		require.NoError(t, err)
		assert.Equal(t, MaxLedgerPageSize, page.Limit)
	})

	t.Run("the window excludes other workspaces", func(t *testing.T) {
		page, err := store.GetLedgerPage(ctx, "ws-nobody", LedgerQuery{From: from, To: to, Limit: 50})
		require.NoError(t, err)
		assert.Empty(t, page.Entries)
		assert.Equal(t, 0, page.Total)
	})
}

func TestPgGetUsageByOperation(t *testing.T) {
	store := newCreditTestStore(t)
	ctx := t.Context()
	const ws = "ws-usage-by-op"
	seedLedger(t, store, ws)
	from, to := ledgerWindow()

	byOp, err := store.GetUsageByOperation(ctx, ws, from, to)
	require.NoError(t, err)

	// Debits only, returned positive; the grant contributes nothing.
	assert.Equal(t, map[string]int64{"ai_translation": 600, "bravo_message": 40}, byOp)
}
