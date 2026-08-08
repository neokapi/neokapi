package graph

import (
	"testing"

	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertNodeIsIdempotent(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	n := &coreg.Node{ID: "block:h1", Label: "block", Properties: map[string]string{"document": "a.md"}}
	require.NoError(t, store.UpsertNode(ctx, n))
	first, err := store.GetNode(ctx, "block:h1")
	require.NoError(t, err)

	// Re-upsert with changed properties: same id refreshes rather than colliding,
	// and the original created_at survives.
	n2 := &coreg.Node{ID: "block:h1", Label: "block", Properties: map[string]string{"document": "b.md"}}
	require.NoError(t, store.UpsertNode(ctx, n2))
	second, err := store.GetNode(ctx, "block:h1")
	require.NoError(t, err)
	assert.Equal(t, "b.md", second.Properties["document"])
	assert.Equal(t, first.CreatedAt.UTC(), second.CreatedAt.UTC(), "created_at is preserved across upsert")

	nodes, err := store.FindNodes(ctx, "block", nil)
	require.NoError(t, err)
	assert.Len(t, nodes, 1, "upsert never duplicates an id")
}

func TestBulkUpsertNodesAndEdgesIdempotent(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	nodes := []*coreg.Node{
		{ID: "block:h1", Label: "block"},
		{ID: "concept:c1", Label: "concept"},
		{ID: "collection:docs", Label: "collection"},
	}
	edges := []*coreg.Edge{
		{ID: "uses_term:h1:c1:", Source: "block:h1", Target: "concept:c1", Label: "uses_term"},
		{ID: "in_collection:h1:docs", Source: "block:h1", Target: "collection:docs", Label: "in_collection"},
	}

	require.NoError(t, store.BulkUpsertNodes(ctx, nodes))
	require.NoError(t, store.BulkUpsertEdges(ctx, edges))

	// A second identical pass must not error on primary-key collisions.
	require.NoError(t, store.BulkUpsertNodes(ctx, nodes))
	require.NoError(t, store.BulkUpsertEdges(ctx, edges))

	usesTerm, err := store.FindEdges(ctx, "uses_term", nil)
	require.NoError(t, err)
	assert.Len(t, usesTerm, 1)

	blocks, err := store.FindNodes(ctx, "block", nil)
	require.NoError(t, err)
	assert.Len(t, blocks, 1)
}

func TestDeleteByLabelClearsProjection(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	require.NoError(t, store.BulkUpsertNodes(ctx, []*coreg.Node{
		{ID: "block:h1", Label: "block"},
		{ID: "concept:c1", Label: "concept"},
		{ID: "collection:docs", Label: "collection"},
	}))
	require.NoError(t, store.BulkUpsertEdges(ctx, []*coreg.Edge{
		{ID: "uses_term:h1:c1:", Source: "block:h1", Target: "concept:c1", Label: "uses_term"},
		{ID: "in_collection:h1:docs", Source: "block:h1", Target: "collection:docs", Label: "in_collection"},
	}))

	// Edges first (they reference nodes), then the nodes the writer owns. The
	// concept node is left untouched — a different label.
	nEdges, err := store.DeleteEdgesByLabel(ctx, "uses_term", "in_collection")
	require.NoError(t, err)
	assert.Equal(t, int64(2), nEdges)
	nNodes, err := store.DeleteNodesByLabel(ctx, "block", "collection")
	require.NoError(t, err)
	assert.Equal(t, int64(2), nNodes)

	remaining, err := store.FindNodes(ctx, "concept", nil)
	require.NoError(t, err)
	assert.Len(t, remaining, 1, "labels the writer does not own survive the purge")

	edges, err := store.FindEdges(ctx, "uses_term", nil)
	require.NoError(t, err)
	assert.Empty(t, edges)
}
