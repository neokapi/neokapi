package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/core/projectdb"
	graphstore "github.com/neokapi/neokapi/host/storage/graph"
	"github.com/neokapi/neokapi/terms"
)

// MaterializeUsesTermGraph rebuilds the project's uses_term / in_collection
// subgraph — the local context graph's first writer — from the project's terms
// over its block cache, and reports the number of edges written.
//
// This is the App entry point, using the memoized project graph handle. The
// shared body is materializeUsesTermGraph; MaterializeUsesTermGraphInDB is the
// same work for an embedding host that holds a project store but not an App.
func (a *App) MaterializeUsesTermGraph(ctx context.Context, root string) (int, error) {
	g, err := a.ProjectGraph(ctx, root)
	if errors.Is(err, ErrNoProjectGraph) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return 0, err
	}
	return materializeUsesTermGraph(ctx, g, db.Terms(), db.BlocksAutocommit())
}

// MaterializeUsesTermGraphInDB rebuilds the uses_term / in_collection subgraph
// for an already-open project store. It is the reuse seam for an embedding host
// that drives extraction directly and holds a project store but not an App (the
// desktop's Re-extract), so the graph can stay in step with the block cache on
// that surface the same way `kapi up` keeps it in step. A store with no
// file-backed SQLite driver (the browser's JSON sidecar) has no graph tables and
// is a no-op.
func MaterializeUsesTermGraphInDB(ctx context.Context, db *projectdb.DB) (int, error) {
	if db == nil || db.Raw() == nil {
		return 0, nil
	}
	g, err := graphstore.NewSQLiteGraphStore(db.Raw())
	if err != nil {
		return 0, fmt.Errorf("materialize uses_term graph: open graph: %w", err)
	}
	return materializeUsesTermGraph(ctx, g, db.Terms(), db.BlocksAutocommit())
}

// materializeUsesTermGraph is the shared rebuild. The subgraph is a pure
// projection of the current terms × blocks, exactly as the block cache is a pure
// projection of the source tree: the labels this writer owns are cleared and
// rebuilt each pass, so a re-extract that renumbers a document leaves no stale
// edge behind (block nodes key on the content key, not a positional id). Concept
// nodes are refreshed rather than purged — they mirror the authored terms store
// and a future terms-hierarchy writer shares that vocabulary.
//
// It must run AFTER extraction has committed its block-write transaction, never
// inside it: the write gate is per handle and not reentrant, and searching the
// blocks here keeps the occurrence FTS index off the block write path.
func materializeUsesTermGraph(ctx context.Context, g *graphstore.SQLiteGraphStore, tb terms.Terminology, blocks blockstore.Store) (int, error) {
	if tb == nil || blocks == nil {
		return 0, nil
	}

	delta, err := occurrence.BuildGraph(ctx, occurrence.Sources{Terms: tb, Blocks: blocks})
	if err != nil {
		return 0, fmt.Errorf("materialize uses_term graph: %w", err)
	}

	// Edges reference nodes, so edges go first. Both deletes and both inserts are
	// separate gated writes; a crash between them leaves the projection empty,
	// which the next pass rebuilds.
	if _, err := g.DeleteEdgesByLabel(ctx, occurrence.EdgeLabelUsesTerm, occurrence.EdgeLabelInCollection); err != nil {
		return 0, fmt.Errorf("materialize uses_term graph: clear edges: %w", err)
	}
	if _, err := g.DeleteNodesByLabel(ctx, occurrence.NodeLabelBlock, occurrence.NodeLabelCollection); err != nil {
		return 0, fmt.Errorf("materialize uses_term graph: clear nodes: %w", err)
	}

	nodes := make([]*coreg.Node, len(delta.Nodes))
	for i := range delta.Nodes {
		nodes[i] = &delta.Nodes[i]
	}
	edges := make([]*coreg.Edge, len(delta.Edges))
	for i := range delta.Edges {
		edges[i] = &delta.Edges[i]
	}
	if err := g.BulkUpsertNodes(ctx, nodes); err != nil {
		return 0, fmt.Errorf("materialize uses_term graph: write nodes: %w", err)
	}
	if err := g.BulkUpsertEdges(ctx, edges); err != nil {
		return 0, fmt.Errorf("materialize uses_term graph: write edges: %w", err)
	}
	return len(delta.Edges), nil
}
