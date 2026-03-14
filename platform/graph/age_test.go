//go:build integration

package graph

// Integration tests for AGEGraphStore — requires Apache AGE in Docker.
// Run with: go test -tags integration ./graph/... -v

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	coreg "github.com/neokapi/neokapi/core/graph"
)

func setupTestStore(t *testing.T) *AGEGraphStore {
	t.Helper()

	connStr := os.Getenv("AGE_TEST_DSN")
	if connStr == "" {
		t.Skip("AGE_TEST_DSN not set; skipping AGE integration test")
	}

	ctx := context.Background()
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
	ctx := context.Background()

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
	ctx := context.Background()

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
