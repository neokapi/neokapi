package sqlitestore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPageQueriesSeekAnIndex holds every page query the iterators run to a
// seek in key order. A page that has to sort its candidates (a temporary
// B-tree in the plan) re-reads the whole filtered set for every page, so an
// iteration costs time quadratic in that set: reading a collection of 80,000
// blocks once took four seconds where one query takes a tenth of one.
func TestPageQueriesSeekAnIndex(t *testing.T) {
	st, err := open(filepath.Join(t.TempDir(), "cache.db"), true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	db := st.(*cacheStore).db

	yes, no := true, false
	type page struct {
		query string
		args  []any
	}
	blocks := func(f blockstore.BlockFilter) page {
		q, args := blockPageQuery(f)
		return page{q, append(append([]any{""}, args...), pageSize)}
	}
	cases := map[string]page{
		"every block":                   blocks(blockstore.BlockFilter{}),
		"a collection":                  blocks(blockstore.BlockFilter{Collection: "ui"}),
		"translatable blocks":           blocks(blockstore.BlockFilter{Translatable: &yes}),
		"a collection's untranslatable": blocks(blockstore.BlockFilter{Collection: "ui", Translatable: &no}),
		"one kind of overlay":           {overlaySelect + kindOverlaysPage, []any{"targets/nb", "", pageSize}},
		"every overlay":                 {overlaySelect + allOverlaysPage, []any{"", "", pageSize}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rows, err := db.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+c.query, c.args...)
			require.NoError(t, err)
			defer rows.Close()
			var plan []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
				plan = append(plan, detail)
			}
			require.NoError(t, rows.Err())
			text := strings.Join(plan, "\n")
			assert.NotContains(t, text, "TEMP B-TREE", "the page sorts its candidates instead of reading them in key order")
			assert.Contains(t, text, "SEARCH", "the page seeks an index")
		})
	}
}
