package graph

// Integration tests for SQLGraphStore against a real PostgreSQL container
// (via testutil/pgtest → testcontainers-go). Each test runs in an isolated
// schema, so table-level state does not leak between tests.
//
// These cases are ported from age_test.go (the AGE behavioral contract) and
// extended with the SQL-specific guarantees the task calls out: no-path and
// cyclic-graph termination for ShortestPath, exact Scope/Validity semantics,
// and the Cypher-not-supported escape hatch.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	coreg "github.com/neokapi/neokapi/core/graph"
)

func newSQLTestStore(t *testing.T) *SQLGraphStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	pool := db.Pool()
	require.NotNil(t, pool, "pgtest DB must expose a pgxpool.Pool")
	store := NewSQLGraphStore(pool)
	require.NoError(t, store.EnsureGraph(t.Context()))
	return store
}

// TestSQLImplementsInterface is a compile-time + runtime check that
// SQLGraphStore satisfies core/graph.GraphStore.
func TestSQLImplementsInterface(t *testing.T) {
	var _ coreg.GraphStore = (*SQLGraphStore)(nil)
}

func TestSQLEnsureGraphIdempotent(t *testing.T) {
	store := newSQLTestStore(t)
	// Calling EnsureGraph again must not error (CREATE TABLE IF NOT EXISTS).
	require.NoError(t, store.EnsureGraph(t.Context()))
	require.NoError(t, store.EnsureGraph(t.Context()))
}

func TestSQLNodeCRUD(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	node := &coreg.Node{
		ID:         "test-node-1",
		Label:      "Concept",
		Properties: map[string]string{"name": "integration-test"},
	}
	require.NoError(t, store.CreateNode(ctx, node))
	require.False(t, node.CreatedAt.IsZero(), "CreateNode should stamp CreatedAt")
	require.False(t, node.UpdatedAt.IsZero(), "CreateNode should stamp UpdatedAt")

	got, err := store.GetNode(ctx, "test-node-1")
	require.NoError(t, err)
	assert.Equal(t, "test-node-1", got.ID)
	assert.Equal(t, "Concept", got.Label)
	assert.Equal(t, "integration-test", got.Properties["name"])
	assert.False(t, got.CreatedAt.IsZero())

	node.Properties["name"] = "updated"
	require.NoError(t, store.UpdateNode(ctx, node))

	got, err = store.GetNode(ctx, "test-node-1")
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Properties["name"])

	require.NoError(t, store.DeleteNode(ctx, "test-node-1"))

	_, err = store.GetNode(ctx, "test-node-1")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)
}

func TestSQLNodeNotFoundErrors(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	_, err := store.GetNode(ctx, "missing")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)

	err = store.UpdateNode(ctx, &coreg.Node{ID: "missing", Label: "Concept"})
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)

	err = store.DeleteNode(ctx, "missing")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)
}

func TestSQLEdgeCRUD(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "e-n1", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "e-n2", Label: "Concept", Properties: map[string]string{}}))

	edge := &coreg.Edge{
		ID:         "e-1",
		Source:     "e-n1",
		Target:     "e-n2",
		Label:      "BROADER",
		Properties: map[string]string{"weight": "5"},
	}
	require.NoError(t, store.CreateEdge(ctx, edge))

	got, err := store.GetEdge(ctx, "e-1")
	require.NoError(t, err)
	assert.Equal(t, "BROADER", got.Label)
	assert.Equal(t, "e-n1", got.Source)
	assert.Equal(t, "e-n2", got.Target)
	assert.Equal(t, "5", got.Properties["weight"])
	assert.Nil(t, got.Validity)

	edge.Label = "NARROWER"
	edge.Properties["weight"] = "9"
	require.NoError(t, store.UpdateEdge(ctx, edge))
	got, err = store.GetEdge(ctx, "e-1")
	require.NoError(t, err)
	assert.Equal(t, "NARROWER", got.Label)
	assert.Equal(t, "9", got.Properties["weight"])

	require.NoError(t, store.DeleteEdge(ctx, "e-1"))
	_, err = store.GetEdge(ctx, "e-1")
	require.ErrorIs(t, err, coreg.ErrEdgeNotFound)
}

func TestSQLEdgeNotFoundErrors(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	_, err := store.GetEdge(ctx, "missing")
	require.ErrorIs(t, err, coreg.ErrEdgeNotFound)

	err = store.UpdateEdge(ctx, &coreg.Edge{ID: "missing", Source: "a", Target: "b", Label: "X"})
	require.ErrorIs(t, err, coreg.ErrEdgeNotFound)

	err = store.DeleteEdge(ctx, "missing")
	require.ErrorIs(t, err, coreg.ErrEdgeNotFound)
}

func TestSQLEdgeValidityRoundTrip(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "v-n1", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "v-n2", Label: "Concept", Properties: map[string]string{}}))

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	edge := &coreg.Edge{
		ID: "v-e1", Source: "v-n1", Target: "v-n2", Label: "RELATED",
		Properties: map[string]string{},
		Validity: &coreg.Validity{
			ValidFrom: &from, ValidTo: &to,
			Tags: map[string]string{"market": "us", "channel": "web"},
		},
	}
	require.NoError(t, store.CreateEdge(ctx, edge))

	got, err := store.GetEdge(ctx, "v-e1")
	require.NoError(t, err)
	require.NotNil(t, got.Validity)
	require.NotNil(t, got.Validity.ValidFrom)
	require.NotNil(t, got.Validity.ValidTo)
	assert.True(t, from.Equal(*got.Validity.ValidFrom))
	assert.True(t, to.Equal(*got.Validity.ValidTo))
	assert.Equal(t, "us", got.Validity.Tags["market"])
	assert.Equal(t, "web", got.Validity.Tags["channel"])

	// An edge with an all-empty (non-nil) Validity collapses back to nil on
	// read, matching the SQLite reference.
	empty := &coreg.Edge{
		ID: "v-e2", Source: "v-n1", Target: "v-n2", Label: "RELATED",
		Properties: map[string]string{}, Validity: &coreg.Validity{},
	}
	require.NoError(t, store.CreateEdge(ctx, empty))
	got, err = store.GetEdge(ctx, "v-e2")
	require.NoError(t, err)
	assert.Nil(t, got.Validity)
}

func TestSQLNeighbors(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	// A -BROADER-> B -BROADER-> C
	for _, id := range []string{"nb-a", "nb-b", "nb-c"} {
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: id, Label: "Concept", Properties: map[string]string{"name": id}}))
	}
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "nb-e1", Source: "nb-a", Target: "nb-b", Label: "BROADER", Properties: map[string]string{}}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "nb-e2", Source: "nb-b", Target: "nb-c", Label: "BROADER", Properties: map[string]string{}}))

	// Outgoing neighbours of A → [B]
	nbrs, err := store.Neighbors(ctx, "nb-a", coreg.Outgoing, "BROADER")
	require.NoError(t, err)
	require.Len(t, nbrs, 1)
	assert.Equal(t, "nb-b", nbrs[0].ID)

	// Incoming neighbours of B → [A]
	nbrs, err = store.Neighbors(ctx, "nb-b", coreg.Incoming, "BROADER")
	require.NoError(t, err)
	require.Len(t, nbrs, 1)
	assert.Equal(t, "nb-a", nbrs[0].ID)

	// Outgoing neighbours of B → [C]
	nbrs, err = store.Neighbors(ctx, "nb-b", coreg.Outgoing, "BROADER")
	require.NoError(t, err)
	require.Len(t, nbrs, 1)
	assert.Equal(t, "nb-c", nbrs[0].ID)

	// Both directions on B → {A, C}
	nbrs, err = store.Neighbors(ctx, "nb-b", coreg.Both)
	require.NoError(t, err)
	require.Len(t, nbrs, 2)
	ids := map[string]bool{}
	for _, n := range nbrs {
		ids[n.ID] = true
	}
	assert.True(t, ids["nb-a"] && ids["nb-c"])

	// Label filter that matches nothing → empty.
	nbrs, err = store.Neighbors(ctx, "nb-a", coreg.Outgoing, "NARROWER")
	require.NoError(t, err)
	assert.Empty(t, nbrs)

	// EdgesOf A outgoing → e1
	edges, err := store.EdgesOf(ctx, "nb-a", coreg.Outgoing)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, "nb-e1", edges[0].ID)

	// EdgesOf B both → e1 + e2
	edges, err = store.EdgesOf(ctx, "nb-b", coreg.Both)
	require.NoError(t, err)
	assert.Len(t, edges, 2)
}

func TestSQLNeighborsScoped(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "sc-n1", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "sc-n2", Label: "Concept", Properties: map[string]string{}}))

	now := time.Now().UTC()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	farPast := now.Add(-48 * time.Hour)

	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "sc-e1", Source: "sc-n1", Target: "sc-n2", Label: "RELATED",
		Properties: map[string]string{},
		Validity:   &coreg.Validity{ValidFrom: &past, ValidTo: &future, Tags: map[string]string{"market": "us"}},
	}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "sc-e2", Source: "sc-n1", Target: "sc-n2", Label: "BROADER",
		Properties: map[string]string{},
		Validity:   &coreg.Validity{ValidFrom: &farPast, ValidTo: &past},
	}))

	// In-window + matching market tag → the active edge yields sc-n2.
	scope := coreg.Scope{At: now, Tags: map[string]string{"market": "us"}}
	nbrs, err := store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, scope)
	require.NoError(t, err)
	require.Len(t, nbrs, 1)
	assert.Equal(t, "sc-n2", nbrs[0].ID)

	// Wrong market tag → no active edges → no neighbours.
	wrong := coreg.Scope{At: now, Tags: map[string]string{"market": "eu"}}
	nbrs, err = store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, wrong)
	require.NoError(t, err)
	assert.Empty(t, nbrs)

	// A scope in the far past (before either window opens) → no neighbours.
	beforeAll := coreg.Scope{At: now.Add(-72 * time.Hour)}
	nbrs, err = store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, beforeAll)
	require.NoError(t, err)
	assert.Empty(t, nbrs)

	// A scope inside the expired edge's window (untagged) → sc-n2 via sc-e2.
	inExpired := coreg.Scope{At: now.Add(-36 * time.Hour)}
	nbrs, err = store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, inExpired)
	require.NoError(t, err)
	require.Len(t, nbrs, 1)
	assert.Equal(t, "sc-n2", nbrs[0].ID)
}

func TestSQLFindNodesScoped(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	now := time.Now().UTC()
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)

	// n1 has an active edge; n2 has an expired edge; n3 has no edges.
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "fs-n1", Label: "Concept", Properties: map[string]string{"g": "x"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "fs-n2", Label: "Concept", Properties: map[string]string{"g": "x"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "fs-n3", Label: "Concept", Properties: map[string]string{"g": "x"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "fs-t", Label: "Concept", Properties: map[string]string{}}))

	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "fs-e1", Source: "fs-n1", Target: "fs-t", Label: "RELATED",
		Properties: map[string]string{}, Validity: &coreg.Validity{ValidFrom: &past, ValidTo: &future},
	}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{
		ID: "fs-e2", Source: "fs-n2", Target: "fs-t", Label: "RELATED",
		Properties: map[string]string{}, Validity: &coreg.Validity{ValidFrom: &now, ValidTo: &now}, // empty half-open window → never active
	}))

	// Scoped find at now over label=Concept, g=x: n1 (active edge) and n3
	// (no edges) match; n2 (only an inactive edge) does not.
	got, err := store.FindNodesScoped(ctx, "Concept", map[string]string{"g": "x"}, coreg.ScopeAt(now))
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, n := range got {
		ids[n.ID] = true
	}
	assert.True(t, ids["fs-n1"], "node with active edge should match")
	assert.True(t, ids["fs-n3"], "node with no edges should match")
	assert.False(t, ids["fs-n2"], "node whose only edge is inactive should not match")
}

func TestSQLFindNodes(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "find-n1", Label: "Concept", Properties: map[string]string{"domain": "medical"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "find-n2", Label: "Concept", Properties: map[string]string{"domain": "legal"}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "find-n3", Label: "Term", Properties: map[string]string{"domain": "medical"}}))

	// Concept + domain=medical → only n1
	found, err := store.FindNodes(ctx, "Concept", map[string]string{"domain": "medical"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "find-n1", found[0].ID)

	// All Concept nodes → n1 + n2 (not the Term node)
	found, err = store.FindNodes(ctx, "Concept", nil)
	require.NoError(t, err)
	assert.Len(t, found, 2)

	// Non-matching property value → empty
	found, err = store.FindNodes(ctx, "Concept", map[string]string{"domain": "finance"})
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestSQLFindEdges(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "fe-a", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "fe-b", Label: "Concept", Properties: map[string]string{}}))

	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "fe-e1", Source: "fe-a", Target: "fe-b", Label: "RELATED", Properties: map[string]string{"kind": "synonym"}}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "fe-e2", Source: "fe-a", Target: "fe-b", Label: "RELATED", Properties: map[string]string{"kind": "antonym"}}))

	found, err := store.FindEdges(ctx, "RELATED", map[string]string{"kind": "synonym"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "fe-e1", found[0].ID)

	found, err = store.FindEdges(ctx, "RELATED", nil)
	require.NoError(t, err)
	assert.Len(t, found, 2)
}

func TestSQLBulkCreate(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	nodes := []*coreg.Node{
		{ID: "bulk-n1", Label: "Term", Properties: map[string]string{"text": "hello"}},
		{ID: "bulk-n2", Label: "Term", Properties: map[string]string{"text": "world"}},
		{ID: "bulk-n3", Label: "Term", Properties: map[string]string{"text": "test"}},
	}
	require.NoError(t, store.BulkCreateNodes(ctx, nodes))
	for _, n := range nodes {
		got, err := store.GetNode(ctx, n.ID)
		require.NoError(t, err)
		assert.Equal(t, n.Label, got.Label)
		assert.Equal(t, n.Properties["text"], got.Properties["text"])
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

	// Empty bulk is a no-op.
	require.NoError(t, store.BulkCreateNodes(ctx, nil))
	require.NoError(t, store.BulkCreateEdges(ctx, nil))
}

func TestSQLBulkCreateNodesAtomic(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "dup", Label: "Concept", Properties: map[string]string{}}))

	// A batch containing a duplicate primary key must fail wholesale and leave
	// none of its (other) rows behind — the batch runs in one transaction.
	nodes := []*coreg.Node{
		{ID: "atomic-ok", Label: "Concept", Properties: map[string]string{}},
		{ID: "dup", Label: "Concept", Properties: map[string]string{}}, // PK collision
	}
	err := store.BulkCreateNodes(ctx, nodes)
	require.Error(t, err)

	_, err = store.GetNode(ctx, "atomic-ok")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound, "failed bulk insert must roll back the whole batch")
}

func TestSQLShortestPath(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	// A -> B -> C -> D
	for _, id := range []string{"sp-a", "sp-b", "sp-c", "sp-d"} {
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: id, Label: "Concept", Properties: map[string]string{}}))
	}
	for _, e := range []struct{ id, src, tgt string }{
		{"sp-e1", "sp-a", "sp-b"},
		{"sp-e2", "sp-b", "sp-c"},
		{"sp-e3", "sp-c", "sp-d"},
	} {
		require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: e.id, Source: e.src, Target: e.tgt, Label: "BROADER", Properties: map[string]string{}}))
	}

	path, err := store.ShortestPath(ctx, "sp-a", "sp-d", 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Equal(t, 3, path.Len())
	require.Len(t, path.Nodes, 4)
	assert.Equal(t, "sp-a", path.Nodes[0].ID)
	assert.Equal(t, "sp-d", path.Nodes[3].ID)

	// Same source and target → a zero-length path (one node, no edges).
	path, err = store.ShortestPath(ctx, "sp-a", "sp-a", 5)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Equal(t, 0, path.Len())
	assert.Len(t, path.Nodes, 1)
}

func TestSQLShortestPathNoPath(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "np-a", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "np-b", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "np-island", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "np-e1", Source: "np-a", Target: "np-b", Label: "BROADER", Properties: map[string]string{}}))

	// np-island is disconnected → unreachable → nil path.
	path, err := store.ShortestPath(ctx, "np-a", "np-island", 5)
	require.NoError(t, err)
	assert.Nil(t, path)

	// Reachable but beyond maxDepth → nil path. Build a-b-c-d chain, depth 1.
	require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "np-c", Label: "Concept", Properties: map[string]string{}}))
	require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "np-e2", Source: "np-b", Target: "np-c", Label: "BROADER", Properties: map[string]string{}}))
	path, err = store.ShortestPath(ctx, "np-a", "np-c", 1)
	require.NoError(t, err)
	assert.Nil(t, path, "target beyond maxDepth should be unreachable")
}

// TestSQLShortestPathCycleTerminates proves the recursive CTE terminates on a
// cyclic graph (the visited-node array prevents revisiting) and still returns
// the shortest path.
func TestSQLShortestPathCycleTerminates(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	// 4-cycle A -> B -> C -> D -> A, plus a pendant E hanging off C. Traversal
	// is undirected (source = node OR target = node), so both routes to E
	// (A-B-C-E and A-D-C-E) are length 3 — the pendant cannot be reached
	// without crossing the cycle, which a non-cycle-safe walk would loop on.
	for _, id := range []string{"cy-a", "cy-b", "cy-c", "cy-d", "cy-e"} {
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: id, Label: "Concept", Properties: map[string]string{}}))
	}
	for _, e := range []struct{ id, src, tgt string }{
		{"cy-e1", "cy-a", "cy-b"},
		{"cy-e2", "cy-b", "cy-c"},
		{"cy-e3", "cy-c", "cy-d"},
		{"cy-e4", "cy-d", "cy-a"}, // closes the 4-cycle
		{"cy-e5", "cy-c", "cy-e"}, // pendant
	} {
		require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: e.id, Source: e.src, Target: e.tgt, Label: "REL", Properties: map[string]string{}}))
	}

	// Must terminate (not loop forever on the cycle) and find a length-3 path.
	done := make(chan struct{})
	var path *coreg.Path
	var perr error
	go func() {
		path, perr = store.ShortestPath(ctx, "cy-a", "cy-e", 10)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("ShortestPath did not terminate on a cyclic graph")
	}
	require.NoError(t, perr)
	require.NotNil(t, path)
	assert.Equal(t, 3, path.Len())
	require.Len(t, path.Nodes, 4)
	assert.Equal(t, "cy-a", path.Nodes[0].ID)
	assert.Equal(t, "cy-e", path.Nodes[path.Len()].ID)

	// A shortest path within a pure cycle back to A is unreachable (a simple
	// path cannot revisit A), so A->A over the cycle is the zero-length path.
	self, err := store.ShortestPath(ctx, "cy-a", "cy-a", 10)
	require.NoError(t, err)
	require.NotNil(t, self)
	assert.Equal(t, 0, self.Len())
}

func TestSQLCypherNotSupported(t *testing.T) {
	store := newSQLTestStore(t)
	ctx := t.Context()

	_, err := store.CypherQuery(ctx, "MATCH (n) RETURN n", nil)
	require.ErrorIs(t, err, coreg.ErrCypherNotSupported)

	err = store.CypherExec(ctx, "CREATE (n)", nil)
	require.ErrorIs(t, err, coreg.ErrCypherNotSupported)
}

func TestSQLClose(t *testing.T) {
	store := newSQLTestStore(t)
	// Close is a no-op (pool owned by the caller) and must not error or close
	// the shared pool — subsequent reads still work.
	require.NoError(t, store.Close())
	_, err := store.GetNode(t.Context(), "whatever")
	require.ErrorIs(t, err, coreg.ErrNodeNotFound)
}
