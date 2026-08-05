//go:build !wasm

package sqlitestore

import (
	"context"
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func textBlock(hash, text string, targets map[string][]model.Run) *blockstore.Block {
	b := &blockstore.Block{
		Hash:         hash,
		ID:           hash,
		Translatable: true,
		Source:       []model.Run{model.TextR(text)},
	}
	if targets != nil {
		b.Targets = map[string][]model.Run{}
		maps.Copy(b.Targets, targets)
	}
	return b
}

// seedStore writes blocks into a fresh store at a temp path.
func seedStore(t *testing.T, blocks map[string][]*blockstore.Block) blockstore.Store {
	t.Helper()
	store, err := New(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	writeBlocks(t, store, blocks)
	return store
}

func writeBlocks(t *testing.T, store blockstore.Store, blocks map[string][]*blockstore.Block) {
	t.Helper()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	for collection, bs := range blocks {
		for _, b := range bs {
			require.NoError(t, sess.PutBlock(collection, b))
		}
	}
	require.NoError(t, sess.Commit())
}

func hashesOf(hits []blockstore.TextHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Hash)
	}
	return out
}

func TestSearchBlockText(t *testing.T) {
	store := seedStore(t, map[string][]*blockstore.Block{
		"docs": {
			textBlock("b1", "The content memory recycles approved wording.", nil),
			textBlock("b2", "Content Memory is not a cache.", nil),
			textBlock("b3", "Nothing to see here.", nil),
			textBlock("b4", "An AI reviews the draft.", nil),
			textBlock("b5", "Escape the 100% case_sensitive path.", nil),
		},
		"site": {
			textBlock("b6", "The content memory again, elsewhere.", map[string][]model.Run{
				"nb": {model.TextR("Innholdsminnet igjen, andre steder.")},
			}),
		},
	})

	tests := []struct {
		name   string
		needle string
		opts   blockstore.TextSearchOptions
		want   []string
	}{
		{
			name:   "multi-word phrase, case-insensitive",
			needle: "content memory",
			want:   []string{"b1", "b2", "b6"},
		},
		{
			name:   "the needle's own casing does not matter",
			needle: "CONTENT MEMORY",
			want:   []string{"b1", "b2", "b6"},
		},
		{
			// A store-level search is a substring filter, deliberately: it
			// narrows, and the caller decides what counts as a use of a term.
			// "AI" is inside "again", and the filter says so.
			name:   "a two-letter needle takes the LIKE path",
			needle: "AI",
			want:   []string{"b4", "b6"},
		},
		{
			name:   "LIKE wildcards in the needle are literal",
			needle: "100%",
			want:   []string{"b5"},
		},
		{
			name:   "underscore is literal too",
			needle: "case_sensitive",
			want:   []string{"b5"},
		},
		{
			name:   "collection narrows the search",
			needle: "content memory",
			opts:   blockstore.TextSearchOptions{Collection: "site"},
			want:   []string{"b6"},
		},
		{
			name:   "target text is indexed under its own locale",
			needle: "Innholdsminnet",
			want:   []string{"b6"},
		},
		{
			name:   "locales restrict which text is searched",
			needle: "Innholdsminnet",
			opts:   blockstore.TextSearchOptions{Locales: []string{blockstore.SourceLocale}},
			want:   nil,
		},
		{
			name:   "no match at all",
			needle: "quantum entanglement",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits, err := blockstore.SearchText(t.Context(), store, tt.needle, tt.opts)
			require.NoError(t, err)
			// The index is a candidate filter, so verify the way callers do.
			verified := verifyHits(hits, tt.needle)
			assert.ElementsMatch(t, tt.want, hashesOf(verified))
			for _, h := range verified {
				assert.NotNil(t, h.Block, "a hit carries its block")
			}
		})
	}
}

// verifyHits filters candidates the way every caller of SearchText must: the
// index narrows, the caller decides.
func verifyHits(hits []blockstore.TextHit, needle string) []blockstore.TextHit {
	var out []blockstore.TextHit
	for _, h := range hits {
		if strings.Contains(strings.ToLower(h.Text), strings.ToLower(needle)) {
			out = append(out, h)
		}
	}
	return out
}

// The scan path and the index path answer the same question. The browser build
// has no SQLite at all, and a lab result that differed from the desktop one for
// the same corpus would be a bug nobody could see.
func TestSearchBlockTextMatchesTheScanPath(t *testing.T) {
	blocks := map[string][]*blockstore.Block{
		"docs": {
			textBlock("b1", "The content memory recycles approved wording.", nil),
			textBlock("b2", "Content Memory is not a cache.", nil),
			textBlock("b3", "Terms govern the wording.", nil),
		},
	}
	indexed := seedStore(t, blocks)
	scanned := blockstore.NewMemoryStore()
	writeBlocks(t, scanned, blocks)

	for _, needle := range []string{"content memory", "wording", "TERMS", "absent"} {
		t.Run(needle, func(t *testing.T) {
			a, err := blockstore.SearchText(t.Context(), indexed, needle, blockstore.TextSearchOptions{})
			require.NoError(t, err)
			b, err := blockstore.SearchText(t.Context(), scanned, needle, blockstore.TextSearchOptions{})
			require.NoError(t, err)
			assert.ElementsMatch(t,
				hashesOf(verifyHits(a, needle)),
				hashesOf(verifyHits(b, needle)),
				"the index and the scan agree")
		})
	}
}

// A re-extraction purges and refills. The text rows and the FTS index must go
// with the blocks, or a deleted block keeps answering searches.
func TestPurgeClearsTheTextIndex(t *testing.T) {
	store := seedStore(t, map[string][]*blockstore.Block{
		"docs": {textBlock("b1", "The content memory recycles wording.", nil)},
	})

	sess, err := store.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sess.(blockstore.BlockPurger).DeleteBlocks())
	require.NoError(t, sess.Commit())

	hits, err := blockstore.SearchText(t.Context(), store, "content memory", blockstore.TextSearchOptions{})
	require.NoError(t, err)
	assert.Empty(t, hits, "a purged block is out of the index")

	// And the index is still healthy enough to take new content.
	writeBlocks(t, store, map[string][]*blockstore.Block{
		"docs": {textBlock("b2", "Terms govern the content memory.", nil)},
	})
	hits, err = blockstore.SearchText(t.Context(), store, "content memory", blockstore.TextSearchOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"b2"}, hashesOf(hits))
}

// Re-writing a block replaces its text rather than adding to it, and a target
// that disappears takes its row with it.
func TestRewritingABlockReplacesItsText(t *testing.T) {
	store := seedStore(t, map[string][]*blockstore.Block{
		"docs": {textBlock("b1", "Original wording.", map[string][]model.Run{
			"nb": {model.TextR("Opprinnelig ordlyd.")},
		})},
	})

	writeBlocks(t, store, map[string][]*blockstore.Block{
		"docs": {textBlock("b1", "Revised wording.", nil)},
	})

	for needle, want := range map[string][]string{
		"Original wording":   nil,
		"Opprinnelig":        nil,
		"Revised wording":    {"b1"},
		"integrity-untested": nil,
	} {
		hits, err := blockstore.SearchText(t.Context(), store, needle, blockstore.TextSearchOptions{})
		require.NoError(t, err)
		assert.ElementsMatch(t, want, hashesOf(verifyHits(hits, needle)), "needle %q", needle)
	}

	assertIndexIntact(t, store)
}

// Blocks written before the index existed are picked up by the first search. A
// store that only indexed new writes would answer nothing on any existing
// project until the next full extraction, and say nothing about why.
func TestSearchIndexesPreexistingBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")

	// Build a store at migration 1 only, then write a block the way v1 did.
	db, err := storage.Open(path)
	require.NoError(t, err)
	require.NoError(t, storage.Migrate(db, "cache_migrations", cacheMigrations[:1]))
	legacy := &cacheStore{db: db, owns: true}
	sess, err := legacy.Begin(t.Context())
	require.NoError(t, err)
	cs := sess.(*cacheSession)
	payload := mustJSON(t, textBlock("b1", "The content memory recycles wording.", nil))
	_, err = cs.q().ExecContext(t.Context(),
		`INSERT INTO blocks (hash, collection, translatable, payload) VALUES (?, ?, ?, ?)`,
		"b1", "docs", 1, payload)
	require.NoError(t, err)
	require.NoError(t, sess.Commit())
	require.NoError(t, db.Close())

	// Reopening runs migration 2; the first search builds the index.
	store, err := New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	hits, err := blockstore.SearchText(t.Context(), store, "content memory", blockstore.TextSearchOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"b1"}, hashesOf(hits), "the pre-existing block is searchable")

	// Nothing is left stale, so the next search builds nothing.
	var pending int
	require.NoError(t, store.(*cacheStore).db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM blocks WHERE text_indexed = 0`).Scan(&pending))
	assert.Zero(t, pending, "the build runs once per change, not once per search")

	assertIndexIntact(t, store)
}

// Extraction pays for nothing it does not use: a write marks the block stale
// and stops there. The index catches up when somebody searches.
func TestWritingDoesNotBuildTheIndex(t *testing.T) {
	store := seedStore(t, map[string][]*blockstore.Block{
		"docs": {textBlock("b1", "The content memory recycles wording.", nil)},
	})
	k := store.(*cacheStore)

	var rows, stale int
	require.NoError(t, k.db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM block_texts`).Scan(&rows))
	assert.Zero(t, rows, "a write indexes no text")
	require.NoError(t, k.db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM blocks WHERE text_indexed = 0`).Scan(&stale))
	assert.Equal(t, 1, stale, "the write marks the block instead")

	hits, err := blockstore.SearchText(t.Context(), store, "content memory", blockstore.TextSearchOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"b1"}, hashesOf(hits))

	require.NoError(t, k.db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM blocks WHERE text_indexed = 0`).Scan(&stale))
	assert.Zero(t, stale, "the search settled the debt")
}

func TestEmptyNeedleMatchesNothing(t *testing.T) {
	store := seedStore(t, map[string][]*blockstore.Block{
		"docs": {textBlock("b1", "Anything at all.", nil)},
	})
	hits, err := blockstore.SearchText(t.Context(), store, "", blockstore.TextSearchOptions{})
	require.NoError(t, err)
	assert.Empty(t, hits)
}

// A contentless FTS5 index that has been fed a mismatched 'delete' reports
// corruption only when something reads it. Assert health explicitly wherever a
// test drives the delete/update triggers.
func assertIndexIntact(t *testing.T, store blockstore.Store) {
	t.Helper()
	k, ok := store.(*cacheStore)
	require.True(t, ok)
	_, err := k.db.ExecContext(t.Context(),
		`INSERT INTO block_texts_trigram(block_texts_trigram) VALUES ('integrity-check')`)
	require.NoError(t, err, "the FTS index is intact")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
