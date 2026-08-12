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

// block builds a block with a structural name, a document and optional targets.
func block(hash, id, file, source string, targets map[string]string) *blockstore.Block {
	b := &blockstore.Block{
		Hash:         hash,
		ID:           id,
		Translatable: true,
		Source:       []model.Run{model.TextR(source)},
	}
	b.Properties.File = file
	if len(targets) > 0 {
		b.Targets = map[string][]model.Run{}
		for locale, text := range targets {
			b.Targets[locale] = []model.Run{model.TextR(text)}
		}
	}
	return b
}

func concept(id, domain string, ts ...terms.Term) terms.Concept {
	return terms.Concept{ID: id, Domain: domain, Terms: ts}
}

func term(text, locale string) terms.Term {
	return terms.Term{Text: text, Locale: model.LocaleID(locale), Status: model.TermApproved}
}

// corpus is the shared fixture: two concepts, six blocks across two
// collections, one of them translated.
func corpus() (map[string][]*blockstore.Block, []terms.Concept) {
	blocks := map[string][]*blockstore.Block{
		"docs": {
			block("h1", "guide.intro", "docs/guide.md",
				"The content memory recycles approved wording.", nil),
			block("h2", "guide.body", "docs/guide.md",
				"Content Memory is not a cache; the content memory rebuilds.", nil),
			block("h3", "faq.q1", "docs/faq.md",
				"A term governs the wording; terminology does not.", nil),
			block("h4", "faq.q2", "docs/faq.md",
				"Nothing relevant here at all.", nil),
		},
		"site": {
			block("h5", "home.hero", "site/index.html",
				"The content memory, again.", map[string]string{
					"nb": "Innholdsminnet, med innholdsminne igjen.",
				}),
			block("h6", "home.foot", "site/index.html",
				"A memory of something else.", nil),
		},
	}
	concepts := []terms.Concept{
		concept("c-memory", "product",
			term("content memory", "en"),
			term("innholdsminne", "nb"),
		),
		concept("c-term", "product",
			term("term", "en"),
		),
		concept("c-unused", "product",
			term("quantum entanglement", "en"),
		),
	}
	return blocks, concepts
}

// backends returns the two store pairings the query must answer identically:
// the merged SQLite store and the browser build's in-memory one.
func backends(t *testing.T) map[string]Sources {
	t.Helper()
	blocks, concepts := corpus()

	sqlBlocks, err := sqlitestore.New(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlBlocks.Close() })
	memBlocks := blockstore.NewMemoryStore()
	t.Cleanup(func() { _ = memBlocks.Close() })

	for _, store := range []blockstore.Store{sqlBlocks, memBlocks} {
		sess, err := store.Begin(context.Background())
		require.NoError(t, err)
		for collection, bs := range blocks {
			for _, b := range bs {
				require.NoError(t, sess.PutBlock(collection, b))
			}
		}
		require.NoError(t, sess.Commit())
	}

	sqlTerms, err := terms.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlTerms.Close() })
	memTerms := terms.NewInMemoryStore()
	for _, tb := range []terms.Terminology{sqlTerms, memTerms} {
		for _, c := range concepts {
			require.NoError(t, tb.AddConcept(context.Background(), c))
		}
	}

	return map[string]Sources{
		"sqlite":    {Terms: sqlTerms, Blocks: sqlBlocks},
		"in-memory": {Terms: memTerms, Blocks: memBlocks},
	}
}

type found struct {
	block   string
	locale  string
	term    string
	snippet string
}

func summarize(occ []Occurrence) []found {
	if len(occ) == 0 {
		return nil
	}
	out := make([]found, 0, len(occ))
	for _, o := range occ {
		out = append(out, found{block: o.BlockID, locale: o.Locale, term: o.Term, snippet: o.Snippet})
	}
	return out
}

func TestFind(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		want  []found
	}{
		{
			name:  "by term text, across collections and documents",
			query: Query{Subject: "content memory"},
			want: []found{
				{block: "guide.body", locale: "", term: "content memory",
					snippet: "Content Memory is not a cache; the content memory rebuilds."},
				{block: "guide.body", locale: "", term: "content memory",
					snippet: "Content Memory is not a cache; the content memory rebuilds."},
				{block: "guide.intro", locale: "", term: "content memory",
					snippet: "The content memory recycles approved wording."},
				{block: "home.hero", locale: "", term: "content memory",
					snippet: "The content memory, again."},
			},
		},
		{
			// The concept carries both languages, so its Norwegian term finds
			// the Norwegian target while its English term finds the source.
			name:  "by concept id, every term in every language",
			query: Query{Subject: "c-memory"},
			want: []found{
				{block: "guide.body", locale: "", term: "content memory",
					snippet: "Content Memory is not a cache; the content memory rebuilds."},
				{block: "guide.body", locale: "", term: "content memory",
					snippet: "Content Memory is not a cache; the content memory rebuilds."},
				{block: "guide.intro", locale: "", term: "content memory",
					snippet: "The content memory recycles approved wording."},
				{block: "home.hero", locale: "", term: "content memory",
					snippet: "The content memory, again."},
				{block: "home.hero", locale: "nb", term: "innholdsminne",
					snippet: "Innholdsminnet, med innholdsminne igjen."},
			},
		},
		{
			name:  "the boundary rule keeps a term out of a longer word",
			query: Query{Subject: "term"},
			want: []found{
				{block: "faq.q1", locale: "", term: "term",
					snippet: "A term governs the wording; terminology does not."},
			},
		},
		{
			name:  "a term nobody uses is an empty answer, not an error",
			query: Query{Subject: "quantum entanglement"},
			want:  nil,
		},
		{
			name:  "collection narrows the search",
			query: Query{Subject: "content memory", Collection: "site"},
			want: []found{
				{block: "home.hero", locale: "", term: "content memory",
					snippet: "The content memory, again."},
			},
		},
		{
			name:  "locales restrict which text is searched",
			query: Query{Subject: "c-memory", Locales: []string{"nb"}},
			want: []found{
				{block: "home.hero", locale: "nb", term: "innholdsminne",
					snippet: "Innholdsminnet, med innholdsminne igjen."},
			},
		},
		{
			name:  "an empty locale set matches nothing",
			query: Query{Subject: "c-memory", Locales: []string{}},
			want:  nil,
		},
	}

	for backend, src := range backends(t) {
		for _, tt := range tests {
			t.Run(backend+"/"+tt.name, func(t *testing.T) {
				res, err := Find(t.Context(), src, tt.query)
				require.NoError(t, err)
				assert.Equal(t, tt.want, summarize(res.Occurrences))
				assert.Equal(t, len(res.Occurrences), res.Total)
				assert.False(t, res.Truncated)
			})
		}
	}
}

// "Terms govern" and "a term" are two uses in one block; the count is of uses,
// the block count is of blocks.
func TestFindCountsUsesAndBlocksApart(t *testing.T) {
	for backend, src := range backends(t) {
		t.Run(backend, func(t *testing.T) {
			res, err := Find(t.Context(), src, Query{Subject: "content memory"})
			require.NoError(t, err)
			assert.Equal(t, 4, res.Total, "four uses")
			assert.Equal(t, 3, res.Blocks, "in three blocks")
		})
	}
}

func TestFindLimitTruncates(t *testing.T) {
	for backend, src := range backends(t) {
		t.Run(backend, func(t *testing.T) {
			res, err := Find(t.Context(), src, Query{Subject: "content memory", Limit: 2})
			require.NoError(t, err)
			assert.Len(t, res.Occurrences, 2)
			assert.Equal(t, 4, res.Total, "Total counts what was found, not what was shown")
			assert.True(t, res.Truncated)
			assert.Equal(t, "guide.body", res.Occurrences[0].BlockID, "and it truncates from a stable end")
		})
	}
}

// A typo is not an empty result. The two answers lead to different next steps,
// so they must not look alike.
func TestFindUnknownSubject(t *testing.T) {
	for backend, src := range backends(t) {
		t.Run(backend, func(t *testing.T) {
			_, err := Find(t.Context(), src, Query{Subject: "no such thing"})
			require.ErrorIs(t, err, ErrUnknownSubject)

			_, err = Find(t.Context(), src, Query{Subject: "  "})
			require.ErrorIs(t, err, ErrUnknownSubject)
		})
	}
}

// A project with terms but no extracted blocks answers cleanly: the term
// exists, it is used nowhere yet.
func TestFindWithNoBlocks(t *testing.T) {
	tb := terms.NewInMemoryStore()
	_, concepts := corpus()
	for _, c := range concepts {
		require.NoError(t, tb.AddConcept(t.Context(), c))
	}

	for name, blocks := range map[string]blockstore.Store{
		"absent block store": nil,
		"empty block store":  blockstore.NewMemoryStore(),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := Find(t.Context(), Sources{Terms: tb, Blocks: blocks}, Query{Subject: "content memory"})
			require.NoError(t, err)
			assert.Empty(t, res.Occurrences)
			assert.Zero(t, res.Total)
			assert.Equal(t, []string{"c-memory"}, res.ConceptIDs)
			assert.Equal(t, []string{"content memory"}, res.Terms)
		})
	}
}

func TestFindWithNoTermsStore(t *testing.T) {
	_, err := Find(t.Context(), Sources{}, Query{Subject: "content memory"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnknownSubject, "an absent store is not an unknown term")
}

// One term text may belong to several concepts. Occurrences report which.
func TestFindAcrossConcepts(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), concept("c-a", "product", term("bow", "en"))))
	require.NoError(t, tb.AddConcept(t.Context(), concept("c-b", "music", term("Bow", "en"))))

	store := blockstore.NewMemoryStore()
	sess, err := store.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("docs", block("h1", "b", "docs/x.md", "Draw the bow.", nil)))
	require.NoError(t, sess.Commit())

	res, err := Find(t.Context(), Sources{Terms: tb, Blocks: store}, Query{Subject: "bow"})
	require.NoError(t, err)
	assert.Equal(t, []string{"c-a", "c-b"}, res.ConceptIDs)
	require.Len(t, res.Occurrences, 2)
	assert.ElementsMatch(t, []string{"c-a", "c-b"},
		[]string{res.Occurrences[0].ConceptID, res.Occurrences[1].ConceptID})
}

// The edge an occurrence contributes to must be well-formed and stable: the
// materializer keys on exactly this, and folds repeat uses onto one relationship.
func TestOccurrenceEdge(t *testing.T) {
	o := Occurrence{
		ConceptID: "c-memory", Term: "content memory", TermLocale: "en",
		BlockHash: "k1_abc", BlockID: "guide.intro", Document: "docs/guide.md",
		Collection: "docs", Locale: "", Start: 4, End: 18,
	}
	e := o.Edge(testScope)
	assert.Equal(t, "uses_term", e.Label)
	assert.Equal(t, "block:/fixture/:k1_abc", e.Source, "edges key on the block's durable identity, qualified by scope")
	assert.Equal(t, "concept://:c-memory", e.Target, "the concept is workspace vocabulary, not a project instance")
	assert.Equal(t, "docs/guide.md", e.Properties["document"])
	assert.Equal(t, "fixture", e.Properties["project"], "the scope tuple rides as properties too")
	assert.Equal(t, o.Edge(testScope).ID, e.ID, "the id is deterministic")

	// The match position is not part of the identity: two uses of one term in one
	// block are one relationship, counted, not two edges.
	other := o
	other.Start = 40
	assert.Equal(t, e.ID, other.Edge(testScope).ID, "position does not split the relationship")

	// Locale is: the same term in a different locale's text is a different edge.
	nb := o
	nb.Locale = "nb"
	assert.NotEqual(t, e.ID, nb.Edge(testScope).ID, "locale is part of the relationship identity")
}
