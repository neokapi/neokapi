package host

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/contextgraph"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
	graphstore "github.com/neokapi/neokapi/host/storage/graph"
	"github.com/neokapi/neokapi/terms"
)

// ProjectScope is the scope tuple a project's context-graph rows are written
// under.
//
// Locally the dimensions are pinned: the store IS one project, so there is no
// workspace to name and no stream to distinguish. The project dimension is the
// recipe's name — the only identity a disconnected project has — and it is
// carried anyway, because the ids have to be the qualified scheme for a local
// subgraph and a server one to describe the same world. A project that names
// itself nothing leaves the dimension empty, which is the honest answer rather
// than a fabricated id.
func ProjectScope(proj *project.KapiProject) contextgraph.Scope {
	if proj == nil {
		return contextgraph.Scope{}
	}
	return contextgraph.Scope{Project: proj.Name}
}

// MaterializeContextGraph rebuilds the project's context graph — term
// occurrences, collection membership, the coordinates governing them, and the
// unit-state records blessing their translations — and reports the number of
// edges written.
//
// This is the App entry point, using the memoized project graph handle. The
// shared body is materializeContextGraph; MaterializeContextGraphInDB is the
// same work for an embedding host that holds a project store but not an App.
func (a *App) MaterializeContextGraph(ctx context.Context, root string, proj *project.KapiProject) (int, error) {
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
	return materializeContextGraph(ctx, g, ProjectScope(proj), proj, db.Terms(), db.BlocksAutocommit(), db.Work())
}

// MaterializeContextGraphInDB rebuilds the context graph for an already-open
// project store. It is the reuse seam for an embedding host that drives
// extraction directly and holds a project store but not an App (the desktop's
// Re-extract), so the graph can stay in step with the block cache on that
// surface the same way `kapi up` keeps it in step. A store with no file-backed
// SQLite driver (the browser's JSON sidecar) has no graph tables and is a no-op.
func MaterializeContextGraphInDB(ctx context.Context, db *projectdb.DB, proj *project.KapiProject) (int, error) {
	if db == nil || db.Raw() == nil {
		return 0, nil
	}
	g, err := graphstore.NewSQLiteGraphStore(db.Raw())
	if err != nil {
		return 0, fmt.Errorf("materialize context graph: open graph: %w", err)
	}
	return materializeContextGraph(ctx, g, ProjectScope(proj), proj, db.Terms(), db.BlocksAutocommit(), db.Work())
}

// ExtractToProjectStore is the one extract-into-the-cache path. It rebuilds the
// block set from the resolved content (project.ExtractToBlockStore) and then the
// context graph projected from it, in that order and on the same store, so no
// surface can refresh the blocks and leave the graph, and every usage count read
// from it, describing the previous extraction. `kapi up` takes it on source
// drift and the desktop's Re-extract takes it on demand.
//
// The graph rebuild is best-effort, exactly as the drift stamps are: the graph
// is a projection the next pass rebuilds, so a failed rebuild is a warning on
// the stats rather than a failed extraction, and it must never fail a converge
// over a derived index.
func (a *App) ExtractToProjectStore(
	ctx context.Context,
	reg *registry.FormatRegistry,
	root string,
	pctx *project.ProjectContext,
	files []project.ResolvedFile,
) (project.ExtractStats, error) {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return project.ExtractStats{}, err
	}
	store := db.Blocks()
	if store == nil {
		return project.ExtractStats{}, fmt.Errorf("extract into the block cache: %w", projectdb.ErrNoStore)
	}
	// Session-transactional, not autocommit: extraction purges the prior block
	// set and refills it, and a half-applied purge is a project with no blocks.
	stats, err := project.ExtractToBlockStore(ctx, reg, pctx, store, db, files)
	if err != nil {
		return stats, err
	}
	// The block set just changed, so the graph it projects is stale. This runs
	// AFTER the extraction transaction commits (the write gate is per handle and
	// not reentrant) and searches the freshly written blocks, keeping the
	// occurrence FTS index off the block write path.
	if _, gerr := a.MaterializeContextGraph(ctx, root, pctx.Project); gerr != nil {
		stats.Warnings = append(stats.Warnings, fmt.Sprintf(
			"could not rebuild the context graph: %v. `kapi context` navigation and term usage counts may be stale until the next extract", gerr))
	}
	return stats, nil
}

// materializeContextGraph is the shared rebuild. The subgraph is a pure
// projection of the current recipe × terms × blocks × unit state, exactly as the
// block cache is a pure projection of the source tree: the labels this writer
// owns are cleared and rebuilt each pass.
//
// The clear is also what makes a re-key free. Node ids carry the scope tuple, so
// renaming the project changes every id this writer produces; because the pass
// deletes what it owns before writing, the rows under the old name go with it
// and nothing is left orphaned. The same holds for a re-parse that renumbers a
// document, which does not even re-key: block nodes are their content, not a
// positional id.
//
// Concept nodes are refreshed rather than purged: they mirror the authored
// terms store, and they are workspace vocabulary a future terms-hierarchy writer
// shares. Coordinate nodes ARE purged, because here the recipe is the whole of
// the vocabulary — a profile deleted from it is a point the project no longer
// occupies, and nothing else in this store has an opinion. The server, whose
// coordinate vocabulary belongs to a workspace rather than to any one project,
// leaves them alone for exactly the same reason.
//
// It must run AFTER extraction has committed its block-write transaction, never
// inside it: the write gate is per handle and not reentrant, and searching the
// blocks here keeps the occurrence FTS index off the block write path.
func materializeContextGraph(
	ctx context.Context,
	g *graphstore.SQLiteGraphStore,
	scope contextgraph.Scope,
	proj *project.KapiProject,
	tb terms.Terminology,
	blocks blockstore.Store,
	work *state.WorkStore,
) (int, error) {
	if tb == nil || blocks == nil {
		return 0, nil
	}

	delta, err := occurrence.BuildGraph(ctx, occurrence.Sources{Terms: tb, Blocks: blocks}, scope)
	if err != nil {
		return 0, fmt.Errorf("materialize context graph: %w", err)
	}

	governance := governanceDelta(scope, proj)
	delta.Nodes = append(delta.Nodes, governance.Nodes...)
	delta.Edges = append(delta.Edges, governance.Edges...)

	blessings, err := blessingDelta(ctx, scope, blocks, work)
	if err != nil {
		return 0, fmt.Errorf("materialize context graph: %w", err)
	}
	delta.Nodes = append(delta.Nodes, blessings.Nodes...)
	delta.Edges = append(delta.Edges, blessings.Edges...)

	// Edges reference nodes, so edges go first. Both deletes and both inserts are
	// separate gated writes; a crash between them leaves the projection empty,
	// which the next pass rebuilds.
	if _, err := g.DeleteEdgesByLabel(ctx,
		contextgraph.EdgeUsesTerm, contextgraph.EdgeInCollection,
		contextgraph.EdgeGovernedBy, contextgraph.EdgeBlesses); err != nil {
		return 0, fmt.Errorf("materialize context graph: clear edges: %w", err)
	}
	if _, err := g.DeleteNodesByLabel(ctx,
		contextgraph.NodeBlock, contextgraph.NodeCollection,
		contextgraph.NodeCoordinate, contextgraph.NodeUnitState); err != nil {
		return 0, fmt.Errorf("materialize context graph: clear nodes: %w", err)
	}

	nodes := dedupeNodes(delta.Nodes)
	edges := make([]*coreg.Edge, len(delta.Edges))
	for i := range delta.Edges {
		edges[i] = &delta.Edges[i]
	}
	if err := g.BulkUpsertNodes(ctx, nodes); err != nil {
		return 0, fmt.Errorf("materialize context graph: write nodes: %w", err)
	}
	if err := g.BulkUpsertEdges(ctx, edges); err != nil {
		return 0, fmt.Errorf("materialize context graph: write edges: %w", err)
	}
	return len(delta.Edges), nil
}

// dedupeNodes collapses nodes contributed by more than one builder — a block
// reached both by a term occurrence and by a unit-state record is one node —
// keeping the richest description of each and a deterministic order.
func dedupeNodes(in []coreg.Node) []*coreg.Node {
	byID := make(map[string]coreg.Node, len(in))
	for _, n := range in {
		if prev, ok := byID[n.ID]; ok && len(prev.Properties) >= len(n.Properties) {
			continue
		}
		byID[n.ID] = n
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*coreg.Node, len(ids))
	for i, id := range ids {
		n := byID[id]
		out[i] = &n
	}
	return out
}

// governanceDelta builds the coordinate nodes the recipe declares and the
// governed_by edges binding each named collection to the point that governs it.
//
// The points come from resolution, not from the literal `channel:` strings: a
// collection that declares nothing is governed at the project's default point,
// which is a real place its content sits even though the recipe names no
// coordinate for it. Each edge carries the governing profile's declared window,
// so "what governed this collection in March" is answered by the same half-open
// temporal model the recipe writes.
func governanceDelta(scope contextgraph.Scope, proj *project.KapiProject) *occurrence.GraphDelta {
	d := &occurrence.GraphDelta{}
	if proj == nil {
		return d
	}
	seen := map[string]bool{}
	for _, c := range proj.Collections {
		if c.Name == "" {
			// A point is resolved by collection name, so a bare entry has
			// nothing to bind.
			continue
		}
		// The as-declared view: every binding, its window carried onto the edge
		// rather than applied here. A materialization that dropped an expired
		// binding would make the graph unable to answer what governed last year.
		rc, err := proj.ResolveGovernance(c.Name)
		if err != nil {
			// A channel that does not resolve is a recipe error the loader
			// already reports; the projection simply has nothing to place.
			continue
		}
		ref := rc.Ref()
		coord := contextgraph.CoordinateNode(scope, ref.Profile, ref.Channel)
		if !seen[coord.ID] {
			seen[coord.ID] = true
			d.Nodes = append(d.Nodes, coord)
		}
		label := project.CollectionLabel(c.Name)
		d.Nodes = append(d.Nodes, contextgraph.CollectionNode(scope, label))
		d.Edges = append(d.Edges, contextgraph.GovernedByEdge(scope, label, ref.Profile, ref.Channel, rc.Validity))
	}
	return d
}

// blessingDelta builds the unit-state nodes the working set holds and the
// blesses edges binding each to the block it decided.
//
// The join is (document, unit): a decision records the document it was made in
// and the block's structural key within it, which is the same pair the review
// path and the sync protocol address a unit by. A record whose document or unit
// no longer resolves to a block still gets its node — the decision exists — but
// no edge, because there is nothing to point at. That is the honest reading: an
// edge asserts a relationship, and inventing one for a unit whose block is gone
// would make a deleted block look governed.
func blessingDelta(ctx context.Context, scope contextgraph.Scope, blocks blockstore.Store, work *state.WorkStore) (*occurrence.GraphDelta, error) {
	d := &occurrence.GraphDelta{}
	if work == nil || blocks == nil {
		return d, nil
	}
	units, err := work.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read unit state: %w", err)
	}
	if len(units) == 0 {
		return d, nil
	}

	byDocUnit, err := blocksByDocumentAndID(ctx, blocks)
	if err != nil {
		return nil, err
	}
	// A decision names its document by identity; a stored block names its
	// document by path, because the block store places a block by where it was
	// read from. The two meet here, so the key is turned back into the address
	// before the join — the alternative being a graph that silently loses every
	// blesses-edge the moment a document has a key at all.
	docPaths, err := work.DocumentPaths(ctx)
	if err != nil {
		return nil, fmt.Errorf("read document paths: %w", err)
	}

	seenBlocks := map[string]bool{}
	for _, u := range units {
		variant, err := u.Variant.MarshalText()
		if err != nil {
			return nil, fmt.Errorf("render variant of unit %q: %w", u.Unit, err)
		}
		gu := contextgraph.UnitState{
			Document:    u.Scope,
			Unit:        u.Unit,
			Variant:     string(variant),
			Status:      string(u.Status),
			ReviewState: u.Decision.ReviewState,
			TargetHash:  u.TargetHash,
			ContentHash: u.ContentHash,
		}
		d.Nodes = append(d.Nodes, contextgraph.UnitStateNode(scope, gu))

		document := u.Scope
		if p, known := docPaths[document]; known && p != "" {
			document = p
		}
		b, ok := byDocUnit[documentUnitKey(document, u.Unit)]
		if !ok {
			continue
		}
		if !seenBlocks[b.Hash] {
			seenBlocks[b.Hash] = true
			d.Nodes = append(d.Nodes, contextgraph.BlockNode(scope, contextgraph.Block{
				ContentKey: b.Hash,
				BlockID:    b.ID,
				Document:   b.Properties.File,
			}))
		}
		d.Edges = append(d.Edges, contextgraph.BlessesEdge(scope, gu, b.Hash))
	}
	return d, nil
}

// blocksByDocumentAndID indexes the block cache by the pair a unit-state record
// names its block with. Where two blocks in one document share a structural key
// the first in store order wins, so a rebuild resolves the same way twice.
func blocksByDocumentAndID(ctx context.Context, blocks blockstore.Store) (map[string]*blockstore.Block, error) {
	sess, err := blocks.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open block store session: %w", err)
	}
	defer func() { _ = sess.Rollback() }()

	index := map[string]*blockstore.Block{}
	for b, err := range sess.Blocks(blockstore.BlockFilter{}) {
		if err != nil {
			return nil, fmt.Errorf("stream blocks: %w", err)
		}
		if b == nil || b.ID == "" {
			continue
		}
		key := documentUnitKey(b.Properties.File, b.ID)
		if _, taken := index[key]; taken {
			continue
		}
		index[key] = b
	}
	return index, nil
}

// documentUnitKey normalizes the (document, unit) pair both sides spell. The
// document is a path, and a path written with the platform's separator and one
// written with slashes name the same file.
func documentUnitKey(document, unit string) string {
	if document == "" {
		return "\x00" + unit
	}
	return path.Clean(filepath.ToSlash(document)) + "\x00" + unit
}
