package memory_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fuzzy tier reaches for the FTS5 trigram index, which the bulk write path
// deliberately leaves empty. An empty index must therefore mean "ask the
// scanner", not "no match" — otherwise a content memory that was bulk-imported
// and never re-indexed answers every fuzzy lookup with nothing at all, and the
// SQLite and in-memory backends of one interface disagree on the same input.

const (
	// fuzzySource and fuzzyQuery differ by a couple of characters and are close
	// enough in length to survive the length filter the scanning fallback
	// applies, so both the indexed and the scanning path can find the match and
	// the tests distinguish "fell back" from "found nothing".
	fuzzySource = "The document was translated successfully"
	fuzzyQuery  = "The document was translated successful"
)

func fuzzyEntry(id, en string) memory.Entry {
	return memory.Entry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: en}}},
			"fr": {{Text: &model.TextRun{Text: "Le document a été traduit"}}},
		},
		HintSrcLang: "en",
	}
}

func trigramRows(t *testing.T, tm *memory.SQLiteStore) int {
	t.Helper()
	var n int
	require.NoError(t, tm.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM tm_variant_trigram`).Scan(&n))
	return n
}

func requireFuzzyHit(t *testing.T, tm memory.ContentMemory, wantID string) {
	t.Helper()
	matches, err := tm.LookupText(context.Background(), fuzzyQuery, "en", "fr",
		memory.DefaultLookupOptions())
	require.NoError(t, err)
	require.Len(t, matches, 1, "fuzzy lookup must find the entry")
	assert.Equal(t, wantID, matches[0].Entry.ID)
	assert.Equal(t, memory.MatchFuzzy, matches[0].MatchType)
}

// TestFuzzyLookupAfterBulkImportWithoutRebuild is the reported failure: entries
// written by the bulk path are invisible to fuzzy lookup until someone thinks
// to rebuild the index.
func TestFuzzyLookupAfterBulkImportWithoutRebuild(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.BulkAddWithStream(t.Context(),
		[]memory.Entry{fuzzyEntry("e1", fuzzySource)}, ""))
	require.Zero(t, trigramRows(t, tm), "the bulk path leaves the index empty")

	requireFuzzyHit(t, tm, "e1")
}

// TestFuzzyLookupWithPartiallyIndexedStore covers the harder shape: the index
// is non-empty, so its emptiness is no longer the signal — an entry added
// normally is indexed, and a later bulk add is not.
func TestFuzzyLookupWithPartiallyIndexedStore(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.Add(t.Context(), fuzzyEntry("indexed", "Something else entirely")))
	require.Positive(t, trigramRows(t, tm), "the single-entry path maintains the index")

	require.NoError(t, tm.BulkAddWithStream(t.Context(),
		[]memory.Entry{fuzzyEntry("bulk", fuzzySource)}, ""))

	requireFuzzyHit(t, tm, "bulk")
}

// TestFuzzyLookupOnReopenedUnindexedStore is the same store seen by a later
// process: nothing in memory records that the import skipped the index, so the
// store has to work it out from what is on disk.
func TestFuzzyLookupOnReopenedUnindexedStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")

	writer, err := memory.NewSQLiteStore(path)
	require.NoError(t, err)
	require.NoError(t, writer.BulkAddWithStream(t.Context(),
		[]memory.Entry{fuzzyEntry("e1", fuzzySource)}, ""))
	require.NoError(t, writer.Close())

	reader, err := memory.NewSQLiteStore(path)
	require.NoError(t, err)
	defer reader.Close()
	require.Zero(t, trigramRows(t, reader))

	requireFuzzyHit(t, reader, "e1")
}

// TestFuzzyLookupUsesIndexOnceRebuilt keeps the fallback from becoming the only
// path: after a rebuild the indexed route answers the same lookup.
func TestFuzzyLookupUsesIndexOnceRebuilt(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	require.NoError(t, tm.BulkAddWithStream(t.Context(),
		[]memory.Entry{fuzzyEntry("e1", fuzzySource)}, ""))
	require.NoError(t, tm.RebuildFuzzyIndex(t.Context()))
	require.Positive(t, trigramRows(t, tm))

	requireFuzzyHit(t, tm, "e1")
}

// TestFuzzyLookupBackendsAgree runs the same input through both backends of the
// ContentMemory interface. They are meant to be substitutable; an index the
// SQLite one keeps and the in-memory one does not have must not change the
// answer.
func TestFuzzyLookupBackendsAgree(t *testing.T) {
	backends := map[string]func(t *testing.T) memory.ContentMemory{
		"sqlite-bulk-loaded": func(t *testing.T) memory.ContentMemory {
			tm, err := memory.NewSQLiteStore(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { tm.Close() })
			require.NoError(t, tm.BulkAddWithStream(t.Context(),
				[]memory.Entry{fuzzyEntry("e1", fuzzySource)}, ""))
			return tm
		},
		"in-memory": func(t *testing.T) memory.ContentMemory {
			tm := memory.NewInMemoryStore()
			t.Cleanup(func() { tm.Close() })
			require.NoError(t, tm.Add(t.Context(), fuzzyEntry("e1", fuzzySource)))
			return tm
		},
	}

	results := make(map[string][]memory.Match, len(backends))
	for name, open := range backends {
		t.Run(name, func(t *testing.T) {
			tm := open(t)
			matches, err := tm.LookupText(t.Context(), fuzzyQuery, "en", "fr",
				memory.DefaultLookupOptions())
			require.NoError(t, err)
			require.Len(t, matches, 1)
			assert.Equal(t, "e1", matches[0].Entry.ID)
			results[name] = matches
		})
	}

	require.Len(t, results, len(backends), "every backend must have produced a result")
	sqliteMatch := results["sqlite-bulk-loaded"][0]
	inMemoryMatch := results["in-memory"][0]
	assert.Equal(t, inMemoryMatch.Entry.ID, sqliteMatch.Entry.ID)
	assert.Equal(t, inMemoryMatch.MatchType, sqliteMatch.MatchType)
	assert.InDelta(t, inMemoryMatch.Score, sqliteMatch.Score, 0.0001,
		"the two backends must score the same candidate identically")
}
