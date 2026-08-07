package memory_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImportTMXSession_EntriesAreSearchable is the reported desktop failure: a
// TMX imported through ImportTMXSession went in via the bulk path, which skips
// the FTS5 word-search side-table, so the entries were invisible to
// SearchEntries until someone remembered to rebuild the index. The rebuild now
// happens inside the session, so a plain import leaves the store searchable.
func TestImportTMXSession_EntriesAreSearchable(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	body := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" segtype="sentence" adminlang="en-US" srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="t1">
      <tuv xml:lang="en"><seg>Checkout complete</seg></tuv>
      <tuv xml:lang="fr"><seg>Paiement terminé</seg></tuv>
    </tu>
  </body>
</tmx>`

	_, n, err := memory.ImportTMXSession(t.Context(), tm, strings.NewReader(body), memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	entries, total, err := tm.SearchEntries(t.Context(), memory.SearchParams{Query: "checkout", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "imported TMX entry must be reachable by text search")
	require.Len(t, entries, 1)
	assert.Equal(t, "Checkout complete", entries[0].VariantText("en"))
}

// TestImportTMXSession_SkipRebuildLeavesSearchStale documents the opt-out: a
// batching caller that sets SkipIndexRebuild owns the single end-of-batch
// rebuild, so search stays empty until it runs one.
func TestImportTMXSession_SkipRebuildLeavesSearchStale(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	body := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" segtype="sentence" adminlang="en-US" srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="t1"><tuv xml:lang="en"><seg>Deferred rebuild</seg></tuv><tuv xml:lang="fr"><seg>Reconstruction différée</seg></tuv></tu>
  </body>
</tmx>`

	_, n, err := memory.ImportTMXSession(t.Context(), tm, strings.NewReader(body),
		memory.ImportTMXOptions{OriginKey: "f.tmx", SkipIndexRebuild: true})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	_, total, err := tm.SearchEntries(t.Context(), memory.SearchParams{Query: "deferred", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 0, total, "SkipIndexRebuild defers indexing to the caller")

	require.NoError(t, tm.RebuildSearchIndex(t.Context()))
	_, total, err = tm.SearchEntries(t.Context(), memory.SearchParams{Query: "deferred", Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "rebuild makes the entry searchable")
}
