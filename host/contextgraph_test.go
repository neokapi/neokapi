package host

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextgraph"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
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

// graphFixture is a project store holding four blocks across two collections and
// one concept, plus a recipe naming the collections and the point that governs
// them.
func graphFixture(t *testing.T, a *App, root string, blocks []graphFixtureBlock) {
	t.Helper()
	ctx := context.Background()
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)

	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	for _, bc := range blocks {
		require.NoError(t, sess.PutBlock(bc.collection, bc.block()))
	}
	require.NoError(t, sess.Commit())

	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID: "c-memory", Domain: "product",
		Terms: []terms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))
}

func defaultFixtureBlocks() []graphFixtureBlock {
	return []graphFixtureBlock{
		{"docs", "h1", "guide.intro", "docs/guide.md", "The content memory recycles approved wording."},
		{"docs", "h2", "guide.body", "docs/guide.md", "Content memory is not a cache; the content memory rebuilds."},
		{"site", "h3", "home.hero", "site/index.html", "The content memory, again."},
		{"site", "h4", "home.foot", "site/index.html", "Nothing relevant here."},
	}
}

func fixtureProject(name string) *project.KapiProject {
	return &project.KapiProject{
		Name: name,
		Profiles: map[string]project.Profile{
			"bowrain": {Channels: []project.Channel{{ID: "docs"}, {ID: "web"}}},
		},
		Collections: []project.Collection{
			{Name: "docs", Channel: "bowrain/docs"},
			{Name: "site", Channel: "bowrain/web"},
		},
	}
}

// After the block cache is (re)built, the App's materializer writes the context
// graph into the same merged store, and the graph answers term -> blocks ->
// collection with the same set as the direct join.
func TestMaterializeContextGraph(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	graphFixture(t, a, root, defaultFixtureBlocks())
	proj := fixtureProject("neokapi")
	scope := ProjectScope(proj)

	// Materialize, twice — the second pass proves a rebuild is idempotent.
	n, err := a.MaterializeContextGraph(ctx, root, proj)
	require.NoError(t, err)
	assert.Positive(t, n, "graph_edges is non-empty after extraction")
	n2, err := a.MaterializeContextGraph(ctx, root, proj)
	require.NoError(t, err)
	assert.Equal(t, n, n2, "a rebuild writes the same edge count, not duplicates")

	g, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)
	usesTerm, err := g.FindEdges(ctx, contextgraph.EdgeUsesTerm, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, usesTerm)

	// Every instance node carries the project it sits in, both in its id and as
	// a property — the dimensions are fields, so slicing is a filter.
	blocks, err := g.FindNodes(ctx, contextgraph.NodeBlock, map[string]string{contextgraph.PropProject: "neokapi"})
	require.NoError(t, err)
	assert.NotEmpty(t, blocks)
	for _, b := range blocks {
		parsed, ok := contextgraph.ParseNodeID(b.ID)
		require.True(t, ok, "block node id %q parses", b.ID)
		assert.Equal(t, "neokapi", parsed.Scope.Project)
		assert.Equal(t, parsed.Local, b.Properties[contextgraph.PropContentKey],
			"the content key is both the local identity and a property")
	}

	// The two-hop graph query equals the direct blocks×terms join.
	twoHop, err := contextgraph.Uses(ctx, g, scope, "c-memory")
	require.NoError(t, err)
	graphSet := map[[2]string]bool{}
	for _, u := range twoHop {
		graphSet[[2]string{u.ContentKey, u.Collection}] = true
	}

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	res, err := occurrence.Find(ctx, occurrence.Sources{Terms: db.Terms(), Blocks: db.BlocksAutocommit()},
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

// A re-parse that renumbers a document leaves the graph pointing at the same
// blocks: identity is the content key, and the structural id it renumbered is
// edge and property data.
func TestMaterializeContextGraphSurvivesReparse(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	graphFixture(t, a, root, defaultFixtureBlocks())
	proj := fixtureProject("neokapi")

	_, err := a.MaterializeContextGraph(ctx, root, proj)
	require.NoError(t, err)
	before := edgeIDs(t, ctx, a, root, contextgraph.EdgeUsesTerm)

	// Re-extract with every block renumbered, same content.
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	renumbered := defaultFixtureBlocks()
	for i := range renumbered {
		renumbered[i].id = "p" + renumbered[i].id
	}
	for _, bc := range renumbered {
		require.NoError(t, sess.PutBlock(bc.collection, bc.block()))
	}
	require.NoError(t, sess.Commit())

	_, err = a.MaterializeContextGraph(ctx, root, proj)
	require.NoError(t, err)
	after := edgeIDs(t, ctx, a, root, contextgraph.EdgeUsesTerm)

	assert.Equal(t, before, after, "a renumbered re-parse rewrites the same edges")
}

// Renaming the project re-keys every instance node, and the pass that writes the
// new keys removes the old ones: no edge is left pointing at a scope the project
// no longer occupies.
func TestMaterializeContextGraphSurvivesProjectRename(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	graphFixture(t, a, root, defaultFixtureBlocks())

	before := fixtureProject("neokapi")
	_, err := a.MaterializeContextGraph(ctx, root, before)
	require.NoError(t, err)
	beforeCount := len(edgeIDs(t, ctx, a, root, contextgraph.EdgeUsesTerm))
	require.Positive(t, beforeCount)

	after := fixtureProject("kapi")
	_, err = a.MaterializeContextGraph(ctx, root, after)
	require.NoError(t, err)

	g, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)
	for _, label := range []string{
		contextgraph.EdgeUsesTerm, contextgraph.EdgeInCollection,
		contextgraph.EdgeGovernedBy, contextgraph.EdgeBlesses,
	} {
		edges, ferr := g.FindEdges(ctx, label, nil)
		require.NoError(t, ferr)
		for _, e := range edges {
			src, ok := contextgraph.ParseNodeID(e.Source)
			require.True(t, ok)
			assert.NotEqual(t, "neokapi", src.Scope.Project, "no edge survives under the old project key")
			// Both endpoints still resolve to a node, so nothing dangles.
			_, gerr := g.GetNode(ctx, e.Source)
			assert.NoError(t, gerr, "edge %s source resolves", e.ID)
			_, gerr = g.GetNode(ctx, e.Target)
			assert.NoError(t, gerr, "edge %s target resolves", e.ID)
		}
	}

	// The same subgraph, under the new key.
	assert.Len(t, edgeIDs(t, ctx, a, root, contextgraph.EdgeUsesTerm), beforeCount)
	uses, err := contextgraph.Uses(ctx, g, ProjectScope(after), "c-memory")
	require.NoError(t, err)
	assert.NotEmpty(t, uses)
	stale, err := contextgraph.Uses(ctx, g, ProjectScope(before), "c-memory")
	require.NoError(t, err)
	assert.Empty(t, stale, "the old project key reaches nothing")
}

// `kapi terms occurrences` reads the live join, so its answer is unchanged by a
// re-key of the materialized graph — the two surfaces have to agree before and
// after, or a graph rewrite would silently move a user-visible answer.
func TestOccurrenceAnswerIsUnchangedByRekey(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	graphFixture(t, a, root, defaultFixtureBlocks())
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	src := occurrence.Sources{Terms: db.Terms(), Blocks: db.BlocksAutocommit()}

	_, err = a.MaterializeContextGraph(ctx, root, fixtureProject("neokapi"))
	require.NoError(t, err)
	before, err := occurrence.Find(ctx, src, occurrence.Query{Subject: "content memory"})
	require.NoError(t, err)

	_, err = a.MaterializeContextGraph(ctx, root, fixtureProject("kapi"))
	require.NoError(t, err)
	after, err := occurrence.Find(ctx, src, occurrence.Query{Subject: "content memory"})
	require.NoError(t, err)

	assert.Equal(t, before, after)
	assert.NotEmpty(t, after.Occurrences)
}

// The recipe's coordinate points become nodes, and each named collection is
// bound to the point that governs it.
func TestMaterializeContextGraphWritesCoordinates(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	graphFixture(t, a, root, defaultFixtureBlocks())
	proj := fixtureProject("neokapi")
	scope := ProjectScope(proj)

	_, err := a.MaterializeContextGraph(ctx, root, proj)
	require.NoError(t, err)
	g, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)

	coords, err := g.FindNodes(ctx, contextgraph.NodeCoordinate, nil)
	require.NoError(t, err)
	require.Len(t, coords, 2, "one node per declared point")
	for _, c := range coords {
		parsed, ok := contextgraph.ParseNodeID(c.ID)
		require.True(t, ok)
		assert.Empty(t, parsed.Scope.Project, "a coordinate is workspace vocabulary, not a project instance")
	}

	docs, err := contextgraph.CollectionsAtCoordinate(ctx, g, scope, "bowrain", "docs", coreg.Scope{})
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "docs", docs[0].Collection)
	assert.Equal(t, "neokapi", docs[0].Scope.Project)

	web, err := contextgraph.CollectionsAtCoordinate(ctx, g, scope, "bowrain", "web", coreg.Scope{})
	require.NoError(t, err)
	require.Len(t, web, 1)
	assert.Equal(t, "site", web[0].Collection)
}

// A unit-state record blesses the block it decided, at the target hash it was
// recorded against.
func TestMaterializeContextGraphWritesBlessings(t *testing.T) {
	ctx := context.Background()
	a := &App{}
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.LayoutAt(root)))
	graphFixture(t, a, root, defaultFixtureBlocks())
	proj := fixtureProject("neokapi")
	scope := ProjectScope(proj)

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	require.NoError(t, db.Work().Put(ctx, state.UnitState{
		Unit:       "guide.intro",
		Scope:      "docs/guide.md",
		Variant:    model.Variant("nb"),
		Status:     model.TargetStatusReviewed,
		TargetHash: "th-1",
		Decision:   state.Decision{ReviewState: "approved"},
	}))
	// A record whose block is gone keeps its node and gets no edge.
	require.NoError(t, db.Work().Put(ctx, state.UnitState{
		Unit:    "guide.removed",
		Scope:   "docs/guide.md",
		Variant: model.Variant("nb"),
		Status:  model.TargetStatusTranslated,
	}))

	_, err = a.MaterializeContextGraph(ctx, root, proj)
	require.NoError(t, err)
	g, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)

	states, err := g.FindNodes(ctx, contextgraph.NodeUnitState, nil)
	require.NoError(t, err)
	assert.Len(t, states, 2, "both records are nodes")

	blessings, err := contextgraph.BlessingsOfBlock(ctx, g, scope, "h1")
	require.NoError(t, err)
	require.Len(t, blessings, 1)
	assert.Equal(t, "guide.intro", blessings[0].Unit)
	assert.Equal(t, "nb", blessings[0].Variant)
	assert.Equal(t, "th-1", blessings[0].TargetHash)

	orphan, err := contextgraph.BlessingsOfBlock(ctx, g, scope, "h4")
	require.NoError(t, err)
	assert.Empty(t, orphan, "a block nobody decided is blessed by nobody")
}

func edgeIDs(t *testing.T, ctx context.Context, a *App, root, label string) []string {
	t.Helper()
	g, err := a.ProjectGraph(ctx, root)
	require.NoError(t, err)
	edges, err := g.FindEdges(ctx, label, nil)
	require.NoError(t, err)
	out := make([]string, 0, len(edges))
	for _, e := range edges {
		out = append(out, e.ID)
	}
	return out
}
