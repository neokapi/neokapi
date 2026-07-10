package graph

// Epic 014 — GraphStore backend parity harness.
//
// A single behavioral contract suite (graphContractSuite) is run against BOTH
// graph backends to prove they behave identically for the operations bowrain
// actually uses (node/edge CRUD, neighbours, scoped traversal, bounded shortest
// path, bulk create, find):
//
//   - TestSQLGraphStoreContract — the DEFAULT backend. Runs against standard
//     PostgreSQL (postgres:16-alpine) via the pgtest testcontainer. This is the
//     arm exercised by ordinary CI/local runs.
//   - TestAGEGraphStoreContract — the opt-in Apache AGE backend. Gated on an
//     AGE_TEST_DSN pointing at an AGE-enabled PostgreSQL; skipped otherwise so
//     the default run does not require the apache/age image.
//
// The suite deliberately omits the two operations that legitimately diverge
// between backends (documented in NOTES.md): CypherQuery/CypherExec (an
// AGE-only escape hatch — the SQL backend returns ErrCypherNotSupported, which
// is asserted separately in TestSQLGraphStoreCypherUnsupported) and
// FindNodesScoped (a no-op filter on the AGE backend today).

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	coreg "github.com/neokapi/neokapi/core/graph"
)

// storeFactory returns a fresh, empty GraphStore scoped to the subtest t. The
// factory is called once per subtest so every case starts from a pristine graph
// (isolated schema for SQL, a freshly (re)created graph for AGE).
type storeFactory func(t *testing.T) coreg.GraphStore

// TestSQLGraphStoreContract runs the shared contract against the plain-SQL
// backend on standard PostgreSQL — the default backend for dev and RDS.
func TestSQLGraphStoreContract(t *testing.T) {
	graphContractSuite(t, func(t *testing.T) coreg.GraphStore {
		db := pgtest.NewTestDB(t) // skips when Docker/-short and no BOWRAIN_TEST_POSTGRES_URL
		pool := db.Pool()
		require.NotNil(t, pool, "pgtest must expose a pgxpool for the SQL graph store")
		store := NewSQLGraphStore(pool)
		require.NoError(t, store.EnsureGraph(t.Context()))
		return store
	})
}

// TestAGEGraphStoreContract runs the same shared contract against the Apache
// AGE backend when an AGE-capable PostgreSQL is supplied via AGE_TEST_DSN. This
// arm proves SQL and AGE are interchangeable; it is skipped by default.
func TestAGEGraphStoreContract(t *testing.T) {
	dsn := os.Getenv("AGE_TEST_DSN")
	if dsn == "" {
		t.Skip("AGE_TEST_DSN not set; skipping AGE parity arm (SQL is the default backend)")
	}
	graphContractSuite(t, func(t *testing.T) coreg.GraphStore {
		ctx := t.Context()
		config, err := pgxpool.ParseConfig(dsn)
		require.NoError(t, err)
		config.AfterConnect = AfterConnect
		pool, err := pgxpool.NewWithConfig(ctx, config)
		require.NoError(t, err)
		t.Cleanup(pool.Close)
		// Fresh graph per subtest for isolation (ignore "does not exist" on first run).
		_, _ = pool.Exec(ctx, `SELECT ag_catalog.drop_graph('bowrain_graph', true)`)
		store := NewAGEGraphStore(pool)
		require.NoError(t, store.EnsureGraph(ctx))
		return store
	})
}

// TestSQLGraphStoreCypherUnsupported pins the one intentional divergence: the
// SQL backend does not implement the Cypher escape hatch.
func TestSQLGraphStoreCypherUnsupported(t *testing.T) {
	db := pgtest.NewTestDB(t)
	pool := db.Pool()
	require.NotNil(t, pool)
	store := NewSQLGraphStore(pool)
	require.NoError(t, store.EnsureGraph(t.Context()))

	_, err := store.CypherQuery(t.Context(), "MATCH (n) RETURN n", nil)
	require.ErrorIs(t, err, coreg.ErrCypherNotSupported)
	require.ErrorIs(t, store.CypherExec(t.Context(), "CREATE (n)", nil), coreg.ErrCypherNotSupported)
}

// graphContractSuite is the shared behavioral contract. It mirrors the AGE
// integration cases (age_test.go) so both backends are held to one spec.
func graphContractSuite(t *testing.T, newStore storeFactory) {
	t.Run("NodeCRUD", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

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
		require.Equal(t, "integration-test", got.Properties["name"])

		node.Properties["name"] = "updated"
		require.NoError(t, store.UpdateNode(ctx, node))
		got, err = store.GetNode(ctx, "test-node-1")
		require.NoError(t, err)
		require.Equal(t, "updated", got.Properties["name"])

		require.NoError(t, store.DeleteNode(ctx, "test-node-1"))
		_, err = store.GetNode(ctx, "test-node-1")
		require.ErrorIs(t, err, coreg.ErrNodeNotFound)
	})

	t.Run("EdgeCRUD", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

		n1 := &coreg.Node{ID: "edge-test-n1", Label: "Concept", Properties: map[string]string{}}
		n2 := &coreg.Node{ID: "edge-test-n2", Label: "Concept", Properties: map[string]string{}}
		require.NoError(t, store.CreateNode(ctx, n1))
		require.NoError(t, store.CreateNode(ctx, n2))

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
		require.Equal(t, "edge-test-n1", got.Source)
		require.Equal(t, "edge-test-n2", got.Target)

		require.NoError(t, store.DeleteEdge(ctx, "edge-test-e1"))
		_, err = store.GetEdge(ctx, "edge-test-e1")
		require.ErrorIs(t, err, coreg.ErrEdgeNotFound)
	})

	t.Run("Neighbors", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

		// A -BROADER-> B -BROADER-> C
		for _, n := range []*coreg.Node{
			{ID: "nb-a", Label: "Concept", Properties: map[string]string{"name": "A"}},
			{ID: "nb-b", Label: "Concept", Properties: map[string]string{"name": "B"}},
			{ID: "nb-c", Label: "Concept", Properties: map[string]string{"name": "C"}},
		} {
			require.NoError(t, store.CreateNode(ctx, n))
		}
		require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "nb-e1", Source: "nb-a", Target: "nb-b", Label: "BROADER", Properties: map[string]string{}}))
		require.NoError(t, store.CreateEdge(ctx, &coreg.Edge{ID: "nb-e2", Source: "nb-b", Target: "nb-c", Label: "BROADER", Properties: map[string]string{}}))

		neighbors, err := store.Neighbors(ctx, "nb-a", coreg.Outgoing, "BROADER")
		require.NoError(t, err)
		require.Len(t, neighbors, 1)
		assert.Equal(t, "nb-b", neighbors[0].ID)

		neighbors, err = store.Neighbors(ctx, "nb-b", coreg.Incoming, "BROADER")
		require.NoError(t, err)
		require.Len(t, neighbors, 1)
		assert.Equal(t, "nb-a", neighbors[0].ID)

		neighbors, err = store.Neighbors(ctx, "nb-b", coreg.Outgoing, "BROADER")
		require.NoError(t, err)
		require.Len(t, neighbors, 1)
		assert.Equal(t, "nb-c", neighbors[0].ID)

		edges, err := store.EdgesOf(ctx, "nb-a", coreg.Outgoing)
		require.NoError(t, err)
		require.Len(t, edges, 1)
		assert.Equal(t, "BROADER", edges[0].Label)
	})

	t.Run("NeighborsScoped", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "sc-n1", Label: "Concept", Properties: map[string]string{}}))
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "sc-n2", Label: "Concept", Properties: map[string]string{}}))

		now := time.Now().UTC()
		past := now.Add(-24 * time.Hour)
		future := now.Add(24 * time.Hour)
		farPast := now.Add(-48 * time.Hour)

		activeEdge := &coreg.Edge{
			ID: "sc-e1", Source: "sc-n1", Target: "sc-n2", Label: "RELATED",
			Properties: map[string]string{},
			Validity:   &coreg.Validity{ValidFrom: &past, ValidTo: &future, Tags: map[string]string{"market": "us"}},
		}
		expiredEdge := &coreg.Edge{
			ID: "sc-e2", Source: "sc-n1", Target: "sc-n2", Label: "BROADER",
			Properties: map[string]string{},
			Validity:   &coreg.Validity{ValidFrom: &farPast, ValidTo: &past},
		}
		require.NoError(t, store.CreateEdge(ctx, activeEdge))
		require.NoError(t, store.CreateEdge(ctx, expiredEdge))

		scope := coreg.Scope{At: now, Tags: map[string]string{"market": "us"}}
		neighbors, err := store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, scope)
		require.NoError(t, err)
		require.Len(t, neighbors, 1)
		assert.Equal(t, "sc-n2", neighbors[0].ID)

		wrongScope := coreg.Scope{At: now, Tags: map[string]string{"market": "eu"}}
		neighbors, err = store.NeighborsScoped(ctx, "sc-n1", coreg.Outgoing, wrongScope)
		require.NoError(t, err)
		assert.Empty(t, neighbors)
	})

	t.Run("BulkCreate", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

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
	})

	t.Run("FindNodes", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "find-n1", Label: "Concept", Properties: map[string]string{"domain": "medical"}}))
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "find-n2", Label: "Concept", Properties: map[string]string{"domain": "legal"}}))
		require.NoError(t, store.CreateNode(ctx, &coreg.Node{ID: "find-n3", Label: "Term", Properties: map[string]string{"domain": "medical"}}))

		found, err := store.FindNodes(ctx, "Concept", map[string]string{"domain": "medical"})
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, "find-n1", found[0].ID)

		found, err = store.FindNodes(ctx, "Concept", nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(found), 2)
	})

	t.Run("ShortestPath", func(t *testing.T) {
		ctx := t.Context()
		store := newStore(t)

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
		assert.Len(t, path.Nodes, 4)
	})
}
