package host

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A merge used to teach the content memory one block at a time: N merged blocks,
// N write transactions. That was affordable while the content memory owned a
// database nobody else wrote to. In the merged store those N commits contend
// with the block cache and the working set for one writer, and SQLite's deferred
// write transactions do not queue fairly — a long run of one-row commits starves
// whichever writer arrives second.
//
// So the run stages what it learns and writes it once. These tests pin that
// shape from the outside: nothing is durable until flush, and then all of it is.
// A test cannot count SQLite transactions, but it can pin the property that
// makes the count bounded — the store is untouched for the duration of the run.

func absorberFixture(t *testing.T) *memoryAbsorber {
	t.Helper()
	a := &App{}
	t.Cleanup(a.Shutdown)
	db, err := a.ProjectDB(t.Context(), t.TempDir())
	require.NoError(t, err)
	absorber := a.newMemoryAbsorber(db.Memory())
	require.NotNil(t, absorber)
	return absorber
}

// mergedBlock builds a block that merged cleanly: source text plus a target for
// locale fr.
func mergedBlock(id, source, target string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: source}}})
	b.SetTargetRuns("fr", []model.Run{{Text: &model.TextRun{Text: target}}})
	return b
}

// TestMemoryAbsorber_WritesOncePerRun is the contract: absorbing N blocks writes
// nothing, and one flush makes all N durable. If a future edit puts the write
// back inside absorb, the mid-run assertion fails.
func TestMemoryAbsorber_WritesOncePerRun(t *testing.T) {
	absorber := absorberFixture(t)
	tm := absorber.tm

	const blocks = 25
	newTotal := 0
	for i := range blocks {
		n, u, err := absorber.absorb(t.Context(),
			mergedBlock(fmt.Sprintf("k%d", i), fmt.Sprintf("Source %d", i), fmt.Sprintf("Cible %d", i)),
			"en", "fr", "batch", "src/app.json", "src/app.fr.xliff")
		require.NoError(t, err)
		newTotal += n
		assert.Zero(t, u, "a fresh store has nothing to update")
	}
	assert.Equal(t, blocks, newTotal, "every block is counted as it is absorbed, before anything is written")

	entries, err := tm.Entries(t.Context())
	require.NoError(t, err)
	assert.Empty(t, entries, "absorbing must not write — the whole point is one transaction per run")

	require.NoError(t, absorber.flush(t.Context()))

	entries, err = tm.Entries(t.Context())
	require.NoError(t, err)
	assert.Len(t, entries, blocks, "flush makes the whole run durable at once")
}

// The bulk path skips per-row FTS5 maintenance, so a run that does not rebuild
// the indexes leaves everything it learned invisible to search and to fuzzy
// lookup — present in the store, unreachable by the next run's leverage. One
// rebuild per flush is what pays for the bulk write.
func TestMemoryAbsorber_FlushRebuildsTheSearchIndexes(t *testing.T) {
	absorber := absorberFixture(t)

	_, _, err := absorber.absorb(t.Context(),
		mergedBlock("k1", "Welcome back", "Bon retour"),
		"en", "fr", "batch", "src/app.json", "src/app.fr.xliff")
	require.NoError(t, err)
	require.NoError(t, absorber.flush(t.Context()))

	hits, _, err := absorber.tm.SearchEntries(t.Context(), memory.SearchParams{
		Query: "Welcome", AnyLocale: "en", Limit: 10,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, hits,
		"the bulk path skips FTS5 inserts, so flush must rebuild the search index or the run's work is unsearchable")
}

// One string merged from two files is one entry: the id is content-derived, so
// carrying it twice would put two rows with the same primary key through one
// bulk transaction and make the reported "new" count a lie.
func TestMemoryAbsorber_DedupesWithinARun(t *testing.T) {
	absorber := absorberFixture(t)

	first, _, err := absorber.absorb(t.Context(),
		mergedBlock("a", "Save", "Enregistrer"), "en", "fr", "batch", "a.json", "a.xliff")
	require.NoError(t, err)
	second, _, err := absorber.absorb(t.Context(),
		mergedBlock("b", "Save", "Enregistrer"), "en", "fr", "batch", "b.json", "b.xliff")
	require.NoError(t, err)

	assert.Equal(t, 1, first)
	assert.Zero(t, second, "the same content is one entry, counted once")

	require.NoError(t, absorber.flush(t.Context()))
	entries, err := absorber.tm.Entries(t.Context())
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// A run that learned nothing must write nothing — including no index rebuild,
// which is a full pass over the variant table and would otherwise be paid by
// every merge that happened to change no wording.
func TestMemoryAbsorber_EmptyRunWritesNothing(t *testing.T) {
	absorber := absorberFixture(t)

	// A block with no target teaches nothing.
	n, u, err := absorber.absorb(t.Context(),
		&model.Block{ID: "k1", Translatable: true}, "en", "fr", "batch", "a.json", "a.xliff")
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Zero(t, u)

	require.NoError(t, absorber.flush(t.Context()))
	entries, err := absorber.tm.Entries(t.Context())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// A nil absorber is the "--no-memory-update, or the store would not open" case, and
// every call site relies on it being inert rather than on guarding each call.
func TestMemoryAbsorber_NilIsInert(t *testing.T) {
	a := &App{}
	assert.Nil(t, a.newMemoryAbsorber(nil))

	var absorber *memoryAbsorber
	n, u, err := absorber.absorb(t.Context(), mergedBlock("k", "Save", "Enregistrer"),
		"en", "fr", "batch", "a.json", "a.xliff")
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Zero(t, u)
	require.NoError(t, absorber.flush(t.Context()))
}
