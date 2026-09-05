package host_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/contextgraph"
	coregraph "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/facetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// edgeKey is the identity a uses_term edge folds occurrences onto: one block,
// one term, one language of text.
type edgeKey struct{ contentKey, term, locale string }

// The equivalence the whole arrangement rests on, proved once against a real
// extraction rather than a hand-built store: every occurrence the join finds is
// on an edge, nothing is on an edge the join does not find, and the counts and
// the places agree. After this, the faces read the edges and never the join.
func TestMaterializedEdgesAreTheJoin(t *testing.T) {
	p := facetest.Write(t)
	ctx := t.Context()
	a := &host.App{}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)

	db, err := a.ProjectDB(ctx, p.Root)
	require.NoError(t, err)
	g, err := a.ProjectGraph(ctx, p.Root)
	require.NoError(t, err)
	proj, err := project.Load(p.Recipe)
	require.NoError(t, err)
	scope := host.ProjectScope(proj)

	concepts, err := db.Terms().Concepts(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, concepts)

	var joinTotal, edgeTotal int
	for _, c := range concepts {
		res, err := occurrence.Find(ctx, occurrence.Sources{Terms: db.Terms(), Blocks: db.BlocksAutocommit()},
			occurrence.Query{Subject: c.ID})
		require.NoError(t, err)
		join := map[edgeKey]int{}
		joinPlace := map[edgeKey][2]string{}
		for _, o := range res.Occurrences {
			k := edgeKey{o.BlockHash, o.Term, o.Locale}
			join[k]++
			joinPlace[k] = [2]string{o.Document, o.BlockID}
		}
		joinTotal += res.Total

		rollup, err := contextgraph.UsesByProject(ctx, g, scope, c.ID, coregraph.Scope{})
		require.NoError(t, err)
		edges := map[edgeKey]int{}
		edgePlace := map[edgeKey][2]string{}
		for _, pu := range rollup {
			for _, u := range pu.Uses {
				k := edgeKey{u.ContentKey, u.Term, u.Locale}
				edges[k] += u.Occurrences
				edgePlace[k] = [2]string{u.Document, u.BlockID}
				edgeTotal += u.Occurrences
			}
		}

		assert.Equal(t, join, edges, "concept %s: the edges are exactly the join, counts included", c.ID)
		assert.Equal(t, joinPlace, edgePlace, "concept %s: each edge names the place the join found", c.ID)
	}
	assert.Positive(t, joinTotal, "the fixture uses its vocabulary")
	assert.Equal(t, joinTotal, edgeTotal)
}

// The fixture's own count, as every face will report it: the record holds the
// faces to this, so the number is pinned here where it is derived.
func TestFindContextUsesReadsTheFixtureGraph(t *testing.T) {
	p := facetest.Write(t)
	ctx := t.Context()
	a := &host.App{}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)

	cmd := host.NewEnvCommand(ctx, "context-search")
	host.AddProjectFlag(cmd)
	host.AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("project", p.Recipe))
	src, done := a.ContextSearchSourcesFor(cmd, "", "")
	defer done()
	require.NotNil(t, src.Graph, "a project with a store binds its graph")
	require.NoError(t, src.GraphErr)
	assert.False(t, src.Unextracted, "the fixture is extracted when it is written")

	uses, err := host.FindContextUses(ctx, src, "translation memory", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"c-memory"}, uses.ConceptIDs)
	assert.Equal(t, 1, uses.Total, "docs/legacy.md says it once")
	assert.Equal(t, 1, uses.Blocks)
	require.Len(t, uses.Uses, 1)
	assert.Equal(t, "docs/legacy.md", uses.Uses[0].Document)
	assert.Equal(t, "translation memory", uses.Uses[0].Term)
	assert.Contains(t, uses.Uses[0].Snippet, "translation memory")
	assert.True(t, uses.Uses[0].Discouraged, "the edge carries the term's standing")
}

// A term text answers for that term alone, a concept id for every spelling:
// the same rule `kapi terms occurrences` applies, so the two surfaces resolve
// a subject identically.
func TestFindContextUsesResolvesTheSubjectLikeOccurrences(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	ctx := t.Context()

	byTerm, err := host.FindContextUses(ctx, p.src, "widget", 0)
	require.NoError(t, err)
	assert.Equal(t, 2, byTerm.Total, "only the widget edges")
	for _, u := range byTerm.Uses {
		assert.Equal(t, "widget", u.Term)
	}

	byConcept, err := host.FindContextUses(ctx, p.src, "c-widget", 0)
	require.NoError(t, err)
	assert.Equal(t, 4, byConcept.Total, "widget twice, gadget once, dings once")
	assert.Equal(t, 3, byConcept.Blocks)

	capped, err := host.FindContextUses(ctx, p.src, "c-widget", 2)
	require.NoError(t, err)
	assert.Len(t, capped.Uses, 2, "the cap applies to the rows")
	assert.Equal(t, 4, capped.Total, "and never to the total")

	_, err = host.FindContextUses(ctx, p.src, "no such word", 0)
	require.ErrorIs(t, err, occurrence.ErrUnknownSubject)
}

// Without a graph the answer resolves the subject and says why it counts
// nothing, rather than reaching for the join in the graph's place.
func TestFindContextUsesWithoutGraphSaysSo(t *testing.T) {
	p := newGraphProject(t, &project.KapiProject{Name: "uses"}, usesConcepts(), usesBlocks())
	uses, err := host.FindContextUses(t.Context(), host.ContextSearchSources{
		Terms:  p.src.Terms,
		Blocks: p.src.Blocks,
	}, "widget", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"c-widget"}, uses.ConceptIDs)
	assert.Zero(t, uses.Total)
	assert.Empty(t, uses.Uses)
	require.Len(t, uses.Notes, 1)
	assert.Contains(t, uses.Notes[0], "no context graph is bound")
}
