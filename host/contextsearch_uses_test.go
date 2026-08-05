package host_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usesTerms is a real in-memory terms store: the usage count runs the
// occurrence query, which reads concepts rather than just searching them, so a
// stub that answers only Search cannot stand in for it.
func usesTerms(t *testing.T) terms.Terminology {
	t.Helper()
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:         "c-widget",
		Domain:     "product",
		Definition: "The unit users place on a dashboard.",
		Terms: []terms.Term{
			{Text: "gadget", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "widget", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
			{Text: "dings", Locale: "nb", Status: model.TermApproved},
		},
	}))
	return tb
}

func usesBlocks(t *testing.T) blockstore.Store {
	t.Helper()
	store := blockstore.NewMemoryStore()
	t.Cleanup(func() { _ = store.Close() })

	put := func(hash, id, file, source string, targets map[string][]model.Run) {
		b := &blockstore.Block{Hash: hash, ID: id, Translatable: true,
			Source: []model.Run{model.TextR(source)}, Targets: targets}
		b.Properties.File = file
		sess, err := store.Begin(t.Context())
		require.NoError(t, err)
		require.NoError(t, sess.PutBlock("docs", b))
		require.NoError(t, sess.Commit())
	}
	put("h1", "guide.a", "docs/guide.md", "Drag a widget onto the board.", nil)
	put("h2", "guide.b", "docs/guide.md", "Every widget has a gadget inside.", nil)
	put("h3", "guide.c", "docs/guide.md", "Nothing of interest.", map[string][]model.Run{
		"nb": {model.TextR("En dings her.")},
	})
	return store
}

// A discouraged word with nothing using it is a settled question; the same word
// in three places is work. The count is what tells them apart, so it belongs in
// the same answer as the verdict.
func TestSearchContextCountsTermUses(t *testing.T) {
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms:  usesTerms(t),
		Blocks: usesBlocks(t),
	}, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	byTerm := map[string]host.ContextTermHit{}
	for _, h := range res.Terms {
		byTerm[h.Term] = h
	}

	widget := byTerm["widget"]
	assert.True(t, widget.Discouraged)
	assert.Equal(t, 2, widget.Uses, "two uses of the retired word")
	assert.Equal(t, 2, widget.UseBlocks, "in two blocks")
	require.Len(t, widget.TopUses, 2)
	assert.Equal(t, "docs/guide.md", widget.TopUses[0].Document)
	assert.Contains(t, widget.TopUses[0].Snippet, "widget")

	gadget := byTerm["gadget"]
	assert.Equal(t, 1, gadget.Uses, "the preferred word is counted too")

	// A term in another language is counted where that language is written.
	dings := byTerm["dings"]
	assert.Equal(t, 1, dings.Uses)
	require.Len(t, dings.TopUses, 1)
	assert.Equal(t, "nb", dings.TopUses[0].Locale)
}

// A project that has not been extracted must not look like a project where the
// word is unused. The two need different next steps, so the search says which.
func TestSearchContextWithoutBlocksSaysSo(t *testing.T) {
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms: usesTerms(t),
	}, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	require.NotEmpty(t, res.Terms)
	for _, h := range res.Terms {
		assert.Zero(t, h.Uses)
	}
	assert.Contains(t, notesJoined(res), "no block cache is bound")
}

// The note is about term usage, so it has no business appearing when the query
// matched no term at all.
func TestSearchContextWithoutTermHitsSaysNothingAboutUses(t *testing.T) {
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms: usesTerms(t),
	}, host.ContextSearchRequest{Query: "nothing at all like a term"})
	require.NoError(t, err)

	assert.Empty(t, res.Terms)
	assert.NotContains(t, notesJoined(res), "block cache")
}

func TestContextSearchResultRendersUses(t *testing.T) {
	res, err := host.SearchContext(t.Context(), host.ContextSearchSources{
		Terms:  usesTerms(t),
		Blocks: usesBlocks(t),
	}, host.ContextSearchRequest{Query: "widget"})
	require.NoError(t, err)

	var b testWriter
	require.NoError(t, res.FormatText(&b))
	out := b.String()
	assert.Contains(t, out, "used 2 time(s) in 2 block(s)")
	assert.Contains(t, out, "docs/guide.md guide.a")
}

func notesJoined(res *host.ContextSearchResult) string {
	var b strings.Builder
	for _, n := range res.Notes {
		b.WriteString(n + "\n")
	}
	return b.String()
}

type testWriter struct{ b []byte }

func (w *testWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
func (w *testWriter) String() string              { return string(w.b) }
