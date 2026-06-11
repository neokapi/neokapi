//go:build integration

package graph

// Integration tests for AGEGraphStore — requires Apache AGE in Docker.
// Run with: go test -tags integration ./graph/... -v

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreg "github.com/neokapi/neokapi/core/graph"
)

func setupTestStore(t *testing.T) *AGEGraphStore {
	t.Helper()

	connStr := os.Getenv("AGE_TEST_DSN")
	if connStr == "" {
		t.Skip("AGE_TEST_DSN not set; skipping AGE integration test")
	}

	ctx := t.Context()
	config, err := pgxpool.ParseConfig(connStr)
	require.NoError(t, err)

	config.AfterConnect = AfterConnect

	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	store := NewAGEGraphStore(pool)
	require.NoError(t, store.EnsureGraph(ctx))

	return store
}

func TestAGENodeCRUD(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	node := &coreg.Node{
		ID:         "test-node-1",
		Label:      "Concept",
		Properties: map[string]string{"name": "integration-test"},
	}

	require.NoError(t, store.CreateNode(ctx, node))

	got, err := store.GetNode(ctx, "test-node-1")
	require.NoError(t, err)
	require.Equal(t, "test-node-1", got.ID)
	require.Equal(t, "Concept", got.Label)

	node.Properties["name"] = "updated"
	require.NoError(t, store.UpdateNode(ctx, node))

	require.NoError(t, store.DeleteNode(ctx, "test-node-1"))

	_, err = store.GetNode(ctx, "test-node-1")
	require.Error(t, err)
}

func TestAGEEdgeCRUD(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	n1 := &coreg.Node{ID: "edge-test-n1", Label: "Concept", Properties: map[string]string{}}
	n2 := &coreg.Node{ID: "edge-test-n2", Label: "Concept", Properties: map[string]string{}}
	require.NoError(t, store.CreateNode(ctx, n1))
	require.NoError(t, store.CreateNode(ctx, n2))
	t.Cleanup(func() {
		_ = store.DeleteNode(ctx, "edge-test-n1")
		_ = store.DeleteNode(ctx, "edge-test-n2")
	})

	edge := &coreg.Edge{
		ID:         "edge-test-e1",
		Source:     "edge-test-n1",
		Target:     "edge-test-n2",
		Label:      "BROADER",
		Properties: map[string]string{},
	}
	require.NoError(t, store.CreateEdge(ctx, edge))

	got, err := store.GetEdge(ctx, "edge-test-e1")
	require.NoError(t, err)
	require.Equal(t, "BROADER", got.Label)

	require.NoError(t, store.DeleteEdge(ctx, "edge-test-e1"))
}

func TestAGENeighbors(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	// Create a small graph: A -BROADER-> B -BROADER-> C
	a := &coreg.Node{ID: "nb-a", Label: "Concept", Properties: map[string]string{"name": "A"}}
	b := &coreg.Node{ID: "nb-b", Label: "Concept", Properties: map[string]string{"name": "B"}}
	c := &coreg.Node{ID: "nb-c", Label: "Concept", Properties: map[string]string{"name": "C"}}
	require.NoError(t, store.CreateNode(ctx, a))
	require.NoError(t, store.CreateNode(ctx, b))
	require.NoError(t, store.CreateNode(ctx, c))
	t.Cleanup(func() {
		_ = store.DeleteEdge(ctx, "nb-e1")
		_ = store.DeleteEdge(ctx, "nb-e2")
		_ = store.DeleteNode(ctx, "nb-a")
		_ = store.DeleteNode(ctx, "nb-b")
		_ = store.DeleteNode(ctx, "nb-c")
	})

	e1 := &coreg.Edge{ID: "nb-e1", Source: "nb-a", Target: "nb-b", Label: "BROADER", Properties: map[string]string{}}
	e2 := &coreg.Edge{ID: "nb-e2", Source: "nb-b", Target: "nb-c", Label: "BROADER", Properties: map[string]string{}}
	require.NoError(t, store.CreateEdge(ctx, e1))
	require.NoError(t, store.CreateEdge(ctx, e2))

	// Outgoing neighbors of A should be [B]
	neighbors, err := store.Neighbors(ctx, "nb-a", coreg.Outgoing, "BROADER")
	require.NoError(t, err)
	require.Len(t, neighbors, 1)
	assert.Equal(t, "nb-b", neighbors[0].ID)

	// Incoming neighbors of B should be [A]
	neighbors, err = store.Neighbors(ctx, "nb-b", coreg.Incoming, "BROADER")
	require.NoError(t, err)
	require.Len(t, neighbors, 1)
	assert.Equal(t, "nb-a", neighbors[0].ID)

	// Outgoing neighbors of B should be [C]
	neighbors, err = store.Neighbors(ctx, "nb-b", coreg.Outgoing, "BROADER")
	require.NoError(t, err)
	require.Len(t, neighbors, 1)
	assert.Equal(t, "nb-c", neighbors[0].ID)

	// EdgesOf A outgoing should return e1
	edges, err := store.EdgesOf(ctx, "nb-a", coreg.Outgoing)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, "BROADER", edges[0].Label)
}

func TestAGENeighborsScoped(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	n1 := &coreg.Node{ID: "sc-n1", Label: "Concept", Properties: map[string]string{}}
	n2 := &coreg.Node{ID: "sc-n2", Label: "Concept", Properties: map[string]string{}}
	require.NoError(t, store.CreateNode(ctx, n1))
	require.NoError(t, store.CreateNode(ctx, n2))

	now := time.Now().UTC()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	farPast := now.Add(-48 * time.Hour)

	// Active edge: valid now
	activeEdge := &coreg.Edge{
		ID: "sc-e1", Source: "sc-n1", Target: "sc-n2", Label: "RELATED",
		Properties: map[string]string{},
		Validity:   &coreg.Validity{ValidFrom: &past, ValidTo: &future, Tags: map[string]string{"market": "us"}},
	}
	// Expired edge
	expiredEdge := &coreg.Edge{
		ID: "sc-e2", Source: "sc-n1", Target: "sc-n2", Label: "BROADER",
		Properties: map[string]string{},
		Validity:   &coreg.Validity{ValidFrom: &farPast, ValidTo: &past},
	}
	require.NoError(t, store.CreateEdge(ctx, activeEdge))
	require.NoError(t, store.CreateEdge(ctx, expiredEdge))
	t.Cleanup(func() {
		_ = store.DeleteEdge(ctx, "sc-e1")
		_ = store.DeleteEdge(ctx, "sc-e2")
		_ = store.DeleteNode(ctx, "sc-n1")
		_ = store.DeleteNode(ctx, "sc-n2")
	})

	// Scoped query: only active edge should match
	scope := coreg.Scope{At: now, Tags: map[string]string{"market": "us"}}
	neighbors, err := store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, scope)
	require.NoError(t, err)
	require.Len(t, neighbors, 1)
	assert.Equal(t, "sc-n2", neighbors[0].ID)

	// Wrong market tag: no results
	wrongScope := coreg.Scope{At: now, Tags: map[string]string{"market": "eu"}}
	neighbors, err = store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, wrongScope)
	require.NoError(t, err)
	assert.Empty(t, neighbors)
}

func TestAGEBulkCreate(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	nodes := []*coreg.Node{
		{ID: "bulk-n1", Label: "Term", Properties: map[string]string{"text": "hello"}},
		{ID: "bulk-n2", Label: "Term", Properties: map[string]string{"text": "world"}},
		{ID: "bulk-n3", Label: "Term", Properties: map[string]string{"text": "test"}},
	}
	require.NoError(t, store.BulkCreateNodes(ctx, nodes))
	t.Cleanup(func() {
		_ = store.DeleteEdge(ctx, "bulk-e1")
		_ = store.DeleteEdge(ctx, "bulk-e2")
		_ = store.DeleteNode(ctx, "bulk-n1")
		_ = store.DeleteNode(ctx, "bulk-n2")
		_ = store.DeleteNode(ctx, "bulk-n3")
	})

	// Verify all created
	for _, n := range nodes {
		got, err := store.GetNode(ctx, n.ID)
		require.NoError(t, err)
		assert.Equal(t, n.Label, got.Label)
	}

	edges := []*coreg.Edge{
		{ID: "bulk-e1", Source: "bulk-n1", Target: "bulk-n2", Label: "RELATED", Properties: map[string]string{}},
		{ID: "bulk-e2", Source: "bulk-n2", Target: "bulk-n3", Label: "RELATED", Properties: map[string]string{}},
	}
	require.NoError(t, store.BulkCreateEdges(ctx, edges))

	for _, e := range edges {
		got, err := store.GetEdge(ctx, e.ID)
		require.NoError(t, err)
		assert.Equal(t, e.Label, got.Label)
	}
}

func TestAGEFindNodes(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	n1 := &coreg.Node{ID: "find-n1", Label: "Concept", Properties: map[string]string{"domain": "medical"}}
	n2 := &coreg.Node{ID: "find-n2", Label: "Concept", Properties: map[string]string{"domain": "legal"}}
	n3 := &coreg.Node{ID: "find-n3", Label: "Term", Properties: map[string]string{"domain": "medical"}}
	require.NoError(t, store.CreateNode(ctx, n1))
	require.NoError(t, store.CreateNode(ctx, n2))
	require.NoError(t, store.CreateNode(ctx, n3))
	t.Cleanup(func() {
		_ = store.DeleteNode(ctx, "find-n1")
		_ = store.DeleteNode(ctx, "find-n2")
		_ = store.DeleteNode(ctx, "find-n3")
	})

	// Find Concept nodes with domain=medical
	found, err := store.FindNodes(ctx, "Concept", map[string]string{"domain": "medical"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "find-n1", found[0].ID)

	// Find all Concept nodes (no property filter)
	found, err = store.FindNodes(ctx, "Concept", nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(found), 2)
}

func TestAGEShortestPath(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	// A -> B -> C -> D
	for _, id := range []string{"sp-a", "sp-b", "sp-c", "sp-d"} {
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{
			ID: id, Label: "Concept", Properties: map[string]string{},
		}))
	}
	for _, e := range []struct{ id, src, tgt string }{
		{"sp-e1", "sp-a", "sp-b"},
		{"sp-e2", "sp-b", "sp-c"},
		{"sp-e3", "sp-c", "sp-d"},
	} {
		require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
			ID: e.id, Source: e.src, Target: e.tgt, Label: "BROADER", Properties: map[string]string{},
		}))
	}
	t.Cleanup(func() {
		for _, id := range []string{"sp-e1", "sp-e2", "sp-e3"} {
			_ = store.DeleteEdge(ctx, id)
		}
		for _, id := range []string{"sp-a", "sp-b", "sp-c", "sp-d"} {
			_ = store.DeleteNode(ctx, id)
		}
	})

	path, err := store.ShortestPath(ctx, "sp-a", "sp-d", 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Equal(t, 3, path.Len())
	assert.Len(t, path.Nodes, 4)
}

func TestAGECypherQuery(t *testing.T) {
	store := setupTestStore(t)
	ctx := t.Context()

	// Cypher escape hatch should work on AGE
	n := &coreg.Node{ID: "cypher-n1", Label: "Concept", Properties: map[string]string{"name": "test"}}
	require.NoError(t, store.CreateNode(ctx, n))
	t.Cleanup(func() { _ = store.DeleteNode(ctx, "cypher-n1") })

	nodes, err := store.CypherQuery(ctx,
		"MATCH (n:Concept) WHERE n.node_id = 'cypher-n1' RETURN n", nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "cypher-n1", nodes[0].ID)
}
