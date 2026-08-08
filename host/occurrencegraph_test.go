package host

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type graphFixtureBlock struct {
	collection, hash, id, file, source string
}

func (bc graphFixtureBlock) block() *blockstore.Block {
	b := &blockstore.Block{
		Hash:         bc.hash,
		ID:           bc.id,
		Translatable: true,
		Source:       []model.Run{model.TextR(bc.source)},
	}
	b.Properties.File = bc.file
	return b
}

// After the block cache is (re)built, the App's materializer writes the
// uses_term / in_collection subgraph into the same merged store, and the graph
// answers term -> blocks -> collection with the same set as the direct join.
func TestMaterializeUsesTermGraph(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	layout := project.LayoutAt(root)
	require.NoError(t, project.EnsureLayout(layout))

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)

	// Populate the block cache across two collections.
	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	for _, bc := range []graphFixtureBlock{
		{"docs", "h1", "guide.intro", "docs/guide.md", "The content memory recycles approved wording."},
		{"docs", "h2", "guide.body", "docs/guide.md", "Content memory is not a cache; the content memory rebuilds."},
		{"site", "h3", "home.hero", "site/index.html", "The content memory, again."},
		{"site", "h4", "home.foot", "site/index.html", "Nothing relevant here."},
	} {
		require.NoError(t, sess.PutBlock(bc.collection, bc.block()))
	}
	require.NoError(t, sess.Commit())

	// Populate the terms store.
	tb := db.Terms()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c-memory", Domain: "product",
		Terms: []terms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))

	// Materialize, twice — the second pass proves a rebuild is idempotent.
	n, err := a.MaterializeUsesTermGraph(ctx, root)
	require.NoError(t, err)
	assert.Positive(t, n, "graph_edges is non-empty after extraction")
	n2, err := a.MaterializeUsesTermGraph(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, n, n2, "a rebuild writes the same edge count, not duplicates")

	g, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)
	usesTerm, err := g.FindEdges(ctx, occurrence.EdgeLabelUsesTerm, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, usesTerm)

	// The two-hop graph query equals the direct blocks×terms join.
	twoHop, err := occurrence.UsesInCollections(ctx, g, "c-memory")
	require.NoError(t, err)
	graphSet := map[[2]string]bool{}
	for _, u := range twoHop {
		graphSet[[2]string{u.BlockHash, u.Collection}] = true
	}

	res, err := occurrence.Find(ctx, occurrence.Sources{Terms: tb, Blocks: db.BlocksAutocommit()},
		occurrence.Query{Subject: "c-memory"})
	require.NoError(t, err)
	joinSet := map[[2]string]bool{}
	for _, o := range res.Occurrences {
		joinSet[[2]string{o.BlockHash, o.Collection}] = true
	}

	assert.Equal(t, joinSet, graphSet, "term -> blocks -> collection traversal matches the join")
	assert.Contains(t, graphSet, [2]string{"h1", "docs"})
	assert.Contains(t, graphSet, [2]string{"h3", "site"})
}
