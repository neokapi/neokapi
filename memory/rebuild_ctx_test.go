package memory_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/require"
)

// The FTS5 rebuilds are the content memory's longest single write — seconds
// over a dogfood-sized corpus, and the whole time they hold the write lock the
// rest of the project store needs. They used to issue their statements on
// context.Background(), so a caller that had given up, or a command the user had
// interrupted, waited for them anyway. These assert that the caller's context is
// the one that reaches SQLite.

func TestRebuildFuzzyIndex_HonoursContext(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(t.Context(), trilingual("e1", "one", "un", "eins")))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, tm.RebuildFuzzyIndex(ctx), context.Canceled)
}

func TestRebuildSearchIndex_HonoursContext(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(t.Context(), trilingual("e1", "one", "un", "eins")))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, tm.RebuildSearchIndex(ctx), context.Canceled)
}

func TestRebuildIndexes_RepopulateAfterBulkAdd(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	entries := []memory.Entry{
		trilingual("e1", "the store keeps every block", "le magasin", "der Speicher"),
		trilingual("e2", "faithful write-back", "réécriture fidèle", "treues Zurückschreiben"),
	}
	require.NoError(t, tm.BulkAddWithStream(t.Context(), entries, ""))

	require.NoError(t, tm.RebuildFuzzyIndex(t.Context()))
	require.NoError(t, tm.RebuildSearchIndex(t.Context()))

	var n int
	require.NoError(t, tm.DB().QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM tm_variant_trigram`).Scan(&n))
	require.Positive(t, n, "the bulk path skips the trigram index; the rebuild is what fills it")
}
