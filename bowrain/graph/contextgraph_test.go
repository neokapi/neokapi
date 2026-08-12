package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextgraph"
	"github.com/neokapi/neokapi/core/contextgraph/graphtest"
)

// The server arm of the shared query-shape suite. The same table runs against
// the project's SQLite store, which is what makes "the same queries answer
// locally and on the server" a checked claim rather than a stated one: the
// queries are one implementation over an interface both stores satisfy, and
// this test is the second store answering.
func TestSQLContextGraphQueryShapes(t *testing.T) {
	graphtest.RunQueryShapes(t, newSQLTestStore(t))
}

// A scoped delete removes one project stream's rows and leaves every other
// tenant's standing. Locally the writer clears a label outright because the
// store is one project; here it may not, and this is the guard.
func TestScopedDeleteLeavesOtherProjectsAlone(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()
	graphtest.Load(t, store)

	before, err := contextgraph.Uses(ctx, store, graphtest.ScopeWorkspace, graphtest.ConceptShared)
	require.NoError(t, err)
	require.Len(t, before, 3)

	removed, err := store.DeleteEdgesByLabelInScope(ctx, graphtest.ScopeA.Properties(),
		contextgraph.EdgeUsesTerm, contextgraph.EdgeInCollection)
	require.NoError(t, err)
	assert.Positive(t, removed)
	_, err = store.DeleteNodesByLabelInScope(ctx, graphtest.ScopeA.Properties(),
		contextgraph.NodeBlock, contextgraph.NodeCollection)
	require.NoError(t, err)

	after, err := contextgraph.Uses(ctx, store, graphtest.ScopeWorkspace, graphtest.ConceptShared)
	require.NoError(t, err)
	require.Len(t, after, 1, "only the untouched project's use survives")
	assert.Equal(t, graphtest.ProjectB, after[0].Scope.Project)

	// The workspace vocabulary is not a project's to remove.
	projects, err := contextgraph.ProjectsUsingConcept(ctx, store, graphtest.ScopeWorkspace, graphtest.ConceptShared)
	require.NoError(t, err)
	assert.Equal(t, []contextgraph.Scope{{Workspace: graphtest.Workspace, Project: graphtest.ProjectB}}, projects)
}

// An unscoped delete over a graph that spans every tenant would erase the lot,
// and the failure would look like an empty graph rather than like a bug.
func TestUnscopedDeleteIsRefused(t *testing.T) {
	store := newSQLTestStore(t)
	_, err := store.DeleteNodesByLabelInScope(t.Context(), nil, contextgraph.NodeBlock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unscoped delete")
}

// An upsert is the writer's normal case: a rebuild writes the same ids again.
func TestBulkUpsertIsIdempotent(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()
	nodes, edges := graphtest.Fixture()

	require.NoError(t, store.BulkUpsertNodes(ctx, nodes))
	require.NoError(t, store.BulkUpsertEdges(ctx, edges))
	first, err := store.GetNode(ctx, nodes[0].ID)
	require.NoError(t, err)

	require.NoError(t, store.BulkUpsertNodes(ctx, nodes))
	require.NoError(t, store.BulkUpsertEdges(ctx, edges))
	second, err := store.GetNode(ctx, nodes[0].ID)
	require.NoError(t, err)

	assert.Equal(t, first.CreatedAt, second.CreatedAt, "a re-projected node keeps when the graph first knew it")

	blocks, err := store.FindNodes(ctx, contextgraph.NodeBlock, nil)
	require.NoError(t, err)
	assert.Len(t, blocks, 3, "a second pass replaces rows rather than adding them")
}
