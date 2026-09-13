package occurrence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// formsBackends holds a Norwegian plural that does not contain its term's
// spelling, so only a query that reads the term's forms can find it.
func formsBackends(t *testing.T) map[string]Sources {
	t.Helper()
	blocks := []*blockstore.Block{
		block("f1", "nav.alerts", "site/en.json", "Alerts", map[string]string{"nb": "Varsler"}),
		block("f2", "alerts.read", "site/en.json", "Read alerts", map[string]string{"nb": "Lese varsler"}),
		block("f3", "alerts.one", "site/en.json", "One alert", map[string]string{"nb": "Ett varsel"}),
	}
	concept := terms.Concept{ID: "c-alert", Terms: []terms.Term{
		{Text: "alert", Locale: "en", Status: model.TermPreferred, Forms: []string{"alerts"}},
		{Text: "varsel", Locale: "nb", Status: model.TermPreferred, Forms: []string{"varsler"}},
	}}

	sqlBlocks, err := sqlitestore.New(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlBlocks.Close() })
	memBlocks := blockstore.NewMemoryStore()
	t.Cleanup(func() { _ = memBlocks.Close() })
	for _, store := range []blockstore.Store{sqlBlocks, memBlocks} {
		sess, err := store.Begin(context.Background())
		require.NoError(t, err)
		for _, b := range blocks {
			require.NoError(t, sess.PutBlock("site", b))
		}
		require.NoError(t, sess.Commit())
	}

	sqlTerms, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlTerms.Close() })
	memTerms := terms.NewInMemoryStore()
	for _, tb := range []terms.Terminology{sqlTerms, memTerms} {
		require.NoError(t, tb.AddConcept(context.Background(), concept))
	}
	return map[string]Sources{
		"sqlite":    {Terms: sqlTerms, Blocks: sqlBlocks},
		"in-memory": {Terms: memTerms, Blocks: memBlocks},
	}
}

func TestFindUnderDeclaredForms(t *testing.T) {
	for name, src := range formsBackends(t) {
		t.Run(name, func(t *testing.T) {
			res, err := Find(context.Background(), src, Query{Subject: "c-alert", Locales: []string{"nb"}})
			require.NoError(t, err)
			var matched []string
			for _, o := range res.Occurrences {
				matched = append(matched, o.BlockID+":"+o.Matched)
				assert.Equal(t, "varsel", o.Term, "an occurrence under a form names the term")
			}
			assert.ElementsMatch(t, []string{"nav.alerts:Varsler", "alerts.read:varsler", "alerts.one:varsel"}, matched,
				"a block holding only the plural is found, once")
			assert.Equal(t, 3, res.Blocks)

			source, err := Find(context.Background(), src, Query{Subject: "alerts", Locales: []string{SourceLocale}})
			require.NoError(t, err, "a subject typed as a form resolves to its term")
			assert.Equal(t, []string{"c-alert"}, source.ConceptIDs)
			assert.Equal(t, 3, source.Total)
		})
	}
}
