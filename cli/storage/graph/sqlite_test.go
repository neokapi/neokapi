package graph

import (
	"context"
	"testing"
	"time"

	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteGraphStore {
	t.Helper()
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	store, err := NewSQLiteGraphStore(db)
	require.NoError(t, err)
	return store
}

func TestNodeCRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	node := &coreg.Node{
		ID:         "n1",
		Label:      "Concept",
		Properties: map[string]string{"domain": "tech"},
	}

	// Create
	require.NoError(t, store.CreateNode(ctx, node))
	assert.False(t, node.CreatedAt.IsZero())

	// Get
	got, err := store.GetNode(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "n1", got.ID)
	assert.Equal(t, "Concept", got.Label)
	assert.Equal(t, "tech", got.Properties["domain"])

	// Update
	node.Label = "Term"
	node.Properties["domain"] = "medical"
	require.NoError(t, store.UpdateNode(ctx, node))
	got, err = store.GetNode(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "Term", got.Label)
	assert.Equal(t, "medical", got.Properties["domain"])

	// Delete
	require.NoError(t, store.DeleteNode(ctx, "n1"))
	_, err = store.GetNode(ctx, "n1")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)
}

func TestNodeNotFound(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	_, err := store.GetNode(ctx, "nonexistent")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)

	require.ErrorIs(t, store.UpdateNode(ctx, &coreg.Node{ID: "nonexistent"}), coreg.ErrNodeNotFound)
	require.ErrorIs(t, store.DeleteNode(ctx, "nonexistent"), coreg.ErrNodeNotFound)
}

func TestEdgeCRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// Create nodes first
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "Concept"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "Concept"}))

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	edge := &coreg.Edge{
		ID:         "e1",
		Source:     "a",
		Target:     "b",
		Label:      coreg.LabelBroader,
		Properties: map[string]string{"weight": "1"},
		Validity: &coreg.Validity{
			ValidFrom: &from,
			ValidTo:   &to,
			Tags:      map[string]string{"version": "v2"},
		},
	}

	// Create
	require.NoError(t, store.CreateEdge(ctx, edge))

	// Get
	got, err := store.GetEdge(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "a", got.Source)
	assert.Equal(t, "b", got.Target)
	assert.Equal(t, coreg.LabelBroader, got.Label)
	require.NotNil(t, got.Validity)
	assert.Equal(t, "v2", got.Validity.Tags["version"])

	// Update
	edge.Label = coreg.LabelRelated
	require.NoError(t, store.UpdateEdge(ctx, edge))
	got, err = store.GetEdge(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, coreg.LabelRelated, got.Label)

	// Delete
	require.NoError(t, store.DeleteEdge(ctx, "e1"))
	_, err = store.GetEdge(ctx, "e1")
	require.ErrorIs(t, err, coreg.ErrEdgeNotFound)
}

func TestEdgeNotFound(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	_, err := store.GetEdge(ctx, "nonexistent")
	require.ErrorIs(t, err, coreg.ErrEdgeNotFound)

	require.ErrorIs(t, store.DeleteEdge(ctx, "nonexistent"), coreg.ErrEdgeNotFound)
}

func TestFindNodes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "n1", Label: "Concept", Properties: map[string]string{"domain": "tech"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "n2", Label: "Concept", Properties: map[string]string{"domain": "medical"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "n3", Label: "Term", Properties: map[string]string{"domain": "tech"}}))

	// Find by label
	nodes, err := store.FindNodes(ctx, "Concept", nil)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// Find by label + properties
	nodes, err = store.FindNodes(ctx, "Concept", map[string]string{"domain": "tech"})
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, "n1", nodes[0].ID)
}

func TestFindEdges(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "C"}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e1", Source: "a", Target: "b", Label: coreg.LabelBroader, Properties: map[string]string{"w": "1"}}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e2", Source: "b", Target: "a", Label: coreg.LabelRelated, Properties: map[string]string{"w": "2"}}))

	edges, err := store.FindEdges(ctx, coreg.LabelBroader, nil)
	require.NoError(t, err)
	assert.Len(t, edges, 1)
	assert.Equal(t, "e1", edges[0].ID)

	edges, err = store.FindEdges(ctx, coreg.LabelBroader, map[string]string{"w": "1"})
	require.NoError(t, err)
	assert.Len(t, edges, 1)
}

func TestNeighbors(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "c", Label: "C"}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e1", Source: "a", Target: "b", Label: coreg.LabelBroader}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e2", Source: "c", Target: "a", Label: coreg.LabelRelated}))

	// Outgoing from a
	nodes, err := store.Neighbors(ctx, "a", coreg.Outgoing)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, "b", nodes[0].ID)

	// Incoming to a
	nodes, err = store.Neighbors(ctx, "a", coreg.Incoming)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, "c", nodes[0].ID)

	// Both directions from a
	nodes, err = store.Neighbors(ctx, "a", coreg.Both)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// With label filter
	nodes, err = store.Neighbors(ctx, "a", coreg.Both, coreg.LabelBroader)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, "b", nodes[0].ID)
}

func TestNeighborsScoped(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "c", Label: "C"}))

	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	// Edge a->b valid Jan-Jun 2025
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "e1", Source: "a", Target: "b", Label: coreg.LabelBroader,
		Validity: &coreg.Validity{ValidFrom: &from, ValidTo: &to},
	}))
	// Edge a->c with no validity (always active)
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "e2", Source: "a", Target: "c", Label: coreg.LabelRelated,
	}))

	// Scope in range: both neighbors
	scope := coreg.ScopeAt(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC))
	nodes, err := store.NeighborsScoped(ctx, "a", coreg.Outgoing, scope)
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// Scope out of range: only c (unbounded edge)
	scope = coreg.ScopeAt(time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC))
	nodes, err = store.NeighborsScoped(ctx, "a", coreg.Outgoing, scope)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, "c", nodes[0].ID)
}

func TestEdgesOf(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "C"}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e1", Source: "a", Target: "b", Label: coreg.LabelBroader}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e2", Source: "b", Target: "a", Label: coreg.LabelRelated}))

	edges, err := store.EdgesOf(ctx, "a", coreg.Outgoing)
	require.NoError(t, err)
	assert.Len(t, edges, 1)
	assert.Equal(t, "e1", edges[0].ID)

	edges, err = store.EdgesOf(ctx, "a", coreg.Both)
	require.NoError(t, err)
	assert.Len(t, edges, 2)
}

func TestShortestPath(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	// a -> b -> c
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "c", Label: "C"}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e1", Source: "a", Target: "b", Label: coreg.LabelBroader}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "e2", Source: "b", Target: "c", Label: coreg.LabelBroader}))

	path, err := store.ShortestPath(ctx, "a", "c", 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Len(t, path.Nodes, 3)
	assert.Len(t, path.Edges, 2)

	// No path
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "d", Label: "C"}))
	path, err = store.ShortestPath(ctx, "a", "d", 5)
	require.NoError(t, err)
	assert.Nil(t, path)
}

func TestBulkCreate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	nodes := []*coreg.Node{
		{ID: "n1", Label: "C", Properties: map[string]string{"k": "1"}},
		{ID: "n2", Label: "C", Properties: map[string]string{"k": "2"}},
		{ID: "n3", Label: "C", Properties: map[string]string{"k": "3"}},
	}
	require.NoError(t, store.BulkCreateNodes(ctx, nodes))

	for _, n := range nodes {
		got, err := store.GetNode(ctx, n.ID)
		require.NoError(t, err)
		assert.Equal(t, n.Properties["k"], got.Properties["k"])
	}

	edges := []*coreg.Edge{
		{ID: "e1", Source: "n1", Target: "n2", Label: coreg.LabelBroader},
		{ID: "e2", Source: "n2", Target: "n3", Label: coreg.LabelRelated},
	}
	require.NoError(t, store.BulkCreateEdges(ctx, edges))

	for _, e := range edges {
		got, err := store.GetEdge(ctx, e.ID)
		require.NoError(t, err)
		assert.Equal(t, e.Label, got.Label)
	}
}

func TestCypherNotSupported(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	_, err := store.CypherQuery(ctx, "MATCH (n) RETURN n", nil)
	require.ErrorIs(t, err, coreg.ErrCypherNotSupported)

	err = store.CypherExec(ctx, "CREATE (n:Foo)", nil)
	require.ErrorIs(t, err, coreg.ErrCypherNotSupported)
}

func TestEdgeWithNilValidity(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "a", Label: "C"}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "b", Label: "C"}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "e1", Source: "a", Target: "b", Label: coreg.LabelBroader,
	}))

	got, err := store.GetEdge(ctx, "e1")
	require.NoError(t, err)
	assert.Nil(t, got.Validity)
}
