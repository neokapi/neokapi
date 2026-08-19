package graph

import (
	"context"
	"errors"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	blockwire "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/core/contextgraph"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/occurrence"
	fwproject "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/venue"
	fwterms "github.com/neokapi/neokapi/terms"
)

// The server's context-graph writer.
//
// It is the same subgraph a project writes locally, with the dimensions free:
// the ids come from the same constructors (core/contextgraph), so the two
// stores hold one vocabulary rather than two that resemble each other.
// Connecting a project widens the scope of what can be asked; it does not
// change the question.
//
// What differs is only what the dimensions carry. Locally the scope is one
// project and the graph is its whole content; here the scope names a real
// workspace, project and stream inside a graph that spans every tenant, so the
// rebuild clears only what it is entitled to.

// ProjectSubgraph names one project's stream and the stores its subgraph
// projects from.
type ProjectSubgraph struct {
	// Content is the server's content store — the blocks, items, collections
	// and decisions this project holds.
	Content platstore.ContentStore
	// Terms is the workspace's terminology. The concepts it lists become the
	// workspace vocabulary the projects' blocks attach to.
	Terms fwterms.Terminology

	// WorkspaceID, ProjectID and Stream are the scope tuple's values. All three
	// are required: an unscoped write into a shared graph could clear another
	// tenant's rows.
	WorkspaceID string
	ProjectID   string
	Stream      string
	// ProjectName is the project's display name, carried as a property. It is
	// deliberately not part of any id, which is why a rename here costs
	// nothing.
	ProjectName string
}

// Scope is the tuple this subgraph's instance nodes are qualified by.
func (p ProjectSubgraph) Scope() contextgraph.Scope {
	return contextgraph.Scope{Workspace: p.WorkspaceID, Project: p.ProjectID, Stream: p.Stream}
}

func (p ProjectSubgraph) validate() error {
	switch {
	case p.Content == nil:
		return errors.New("materialize context graph: no content store")
	case p.WorkspaceID == "":
		return errors.New("materialize context graph: no workspace")
	case p.ProjectID == "":
		return errors.New("materialize context graph: no project")
	case p.Stream == "":
		return errors.New("materialize context graph: no stream")
	}
	return nil
}

// MaterializeProjectContextGraph rebuilds one project stream's subgraph and
// reports the number of edges written.
//
// The pass is a projection, as it is locally: it clears the labels it owns
// WITHIN ITS SCOPE and rewrites them, so a block, collection or decision that no
// longer projects leaves nothing behind, and another project's rows are never
// touched. Concept and coordinate nodes are workspace vocabulary and are
// refreshed rather than purged — a project stream is in no position to decide
// that the workspace has stopped holding a concept.
func MaterializeProjectContextGraph(ctx context.Context, g *SQLGraphStore, in ProjectSubgraph) (int, error) {
	if g == nil {
		return 0, nil
	}
	if err := in.validate(); err != nil {
		return 0, err
	}
	scope := in.Scope()

	delta := &occurrence.GraphDelta{}
	if in.Terms != nil {
		blocks, err := blockwire.Open(in.Content, in.ProjectID, in.Stream)
		if err != nil {
			return 0, fmt.Errorf("materialize context graph: open blocks: %w", err)
		}
		occ, err := occurrence.BuildGraph(ctx, occurrence.Sources{Terms: in.Terms, Blocks: blocks}, scope)
		if err != nil {
			return 0, fmt.Errorf("materialize context graph: %w", err)
		}
		delta.Nodes = append(delta.Nodes, occ.Nodes...)
		delta.Edges = append(delta.Edges, occ.Edges...)
	}

	governance, err := governanceSubgraph(ctx, scope, in)
	if err != nil {
		return 0, err
	}
	delta.Nodes = append(delta.Nodes, governance.Nodes...)
	delta.Edges = append(delta.Edges, governance.Edges...)

	blessings, err := blessingSubgraph(ctx, scope, in)
	if err != nil {
		return 0, err
	}
	delta.Nodes = append(delta.Nodes, blessings.Nodes...)
	delta.Edges = append(delta.Edges, blessings.Edges...)

	scopeProps := scope.Properties()
	if _, err := g.DeleteEdgesByLabelInScope(ctx, scopeProps,
		contextgraph.EdgeUsesTerm, contextgraph.EdgeInCollection,
		contextgraph.EdgeGovernedBy, contextgraph.EdgeBlesses); err != nil {
		return 0, fmt.Errorf("materialize context graph: clear edges: %w", err)
	}
	if _, err := g.DeleteNodesByLabelInScope(ctx, scopeProps,
		contextgraph.NodeBlock, contextgraph.NodeCollection, contextgraph.NodeUnitState); err != nil {
		return 0, fmt.Errorf("materialize context graph: clear nodes: %w", err)
	}

	nodes := dedupeNodes(delta.Nodes, in.ProjectName)
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

// dedupeNodes collapses nodes contributed by more than one builder, keeping the
// richest description of each, and stamps the project's display name on the
// instance nodes. The name is a property precisely so a rename is an update
// here rather than a re-key of every id.
func dedupeNodes(in []coreg.Node, projectName string) []*coreg.Node {
	byID := make(map[string]coreg.Node, len(in))
	order := make([]string, 0, len(in))
	for _, n := range in {
		if prev, ok := byID[n.ID]; ok {
			if len(prev.Properties) >= len(n.Properties) {
				continue
			}
			byID[n.ID] = n
			continue
		}
		byID[n.ID] = n
		order = append(order, n.ID)
	}
	out := make([]*coreg.Node, 0, len(order))
	for _, id := range order {
		n := byID[id]
		if projectName != "" && n.Properties[contextgraph.PropProject] != "" {
			n.Properties[contextgraph.PropProjectName] = projectName
		}
		out = append(out, &n)
	}
	return out
}

// governanceSubgraph builds the coordinate nodes this project's collections sit
// at, and the governed_by edges binding them.
//
// The point comes off the collection's own recorded context — the coordinates
// the recipe pushed — because that is what the project declared. A collection
// that declared none is governed at the project's default point, which is a real
// place its content sits.
func governanceSubgraph(ctx context.Context, scope contextgraph.Scope, in ProjectSubgraph) (*occurrence.GraphDelta, error) {
	d := &occurrence.GraphDelta{}
	collections, err := in.Content.ListCollections(ctx, in.ProjectID, in.Stream)
	if err != nil {
		return nil, fmt.Errorf("materialize context graph: list collections: %w", err)
	}
	seen := map[string]bool{}
	for _, c := range collections {
		if c == nil || c.Name == "" {
			continue
		}
		profile := c.Context[fwproject.ProductAxis]
		channel := c.Context[fwproject.ChannelAxis]
		coord := contextgraph.CoordinateNode(scope, profile, channel)
		if !seen[coord.ID] {
			seen[coord.ID] = true
			d.Nodes = append(d.Nodes, coord)
		}
		d.Nodes = append(d.Nodes, contextgraph.CollectionNode(scope, c.Name))
		d.Edges = append(d.Edges, contextgraph.GovernedByEdge(scope, c.Name, profile, channel, nil))
	}
	return d, nil
}

// blessedBlock is what the (item, unit) join needs of a block: its identity in
// the graph and the content key a decision blesses. Deliberately not a
// *venue.StoredBlock — one entry per decision, three strings each, rather than
// a retained copy of every block the project holds.
type blessedBlock struct {
	sourceID    string
	itemName    string
	contentHash string
}

// blessingSubgraph builds the unit-state nodes the decision ledger holds and the
// blesses edges binding each to the block it decided.
//
// The join is (item, unit): a decision records the item whose identity namespace
// its unit lives in, which is the server's spelling of the same pair a local
// record names its block with. A decision whose block this store has never held
// keeps its node and gets no edge — the decision is a fact, the relationship is
// not.
func blessingSubgraph(ctx context.Context, scope contextgraph.Scope, in ProjectSubgraph) (*occurrence.GraphDelta, error) {
	d := &occurrence.GraphDelta{}
	decisions, ok := in.Content.(platstore.DecisionStore)
	if !ok {
		return d, nil
	}
	records, err := decisions.ListUnitDecisions(ctx, in.ProjectID, in.Stream)
	if err != nil {
		return nil, fmt.Errorf("materialize context graph: list decisions: %w", err)
	}
	if len(records) == 0 {
		return d, nil
	}

	// Only the blocks a decision names are of interest, and only three of
	// their fields. Reading the corpus to find them — and holding every block
	// that came back — is what OOM-killed the server on a project of no
	// remarkable size, so the walk is batched and keeps the join key and the
	// content hash rather than the block they came from.
	wanted := make(map[string]struct{}, len(records))
	for _, r := range records {
		wanted[r.ItemName+"\x00"+r.Unit] = struct{}{}
	}

	index := make(map[string]blessedBlock, len(wanted))
	err = platstore.EachBlockBatch(ctx, in.Content,
		platstore.BlockQuery{ProjectID: in.ProjectID, Stream: in.Stream},
		platstore.DefaultBlockBatch,
		func(batch []*venue.StoredBlock) error {
			for _, sb := range batch {
				if sb == nil || sb.SourceID == "" {
					continue
				}
				key := sb.ItemName + "\x00" + sb.SourceID
				if _, ok := wanted[key]; !ok {
					continue
				}
				if _, taken := index[key]; taken {
					continue
				}
				index[key] = blessedBlock{
					sourceID:    sb.SourceID,
					itemName:    sb.ItemName,
					contentHash: sb.ContentHash,
				}
			}
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("materialize context graph: read blocks: %w", err)
	}

	seenBlocks := map[string]bool{}
	for _, r := range records {
		u := contextgraph.UnitState{
			Document:    r.ItemName,
			Unit:        r.Unit,
			Variant:     r.Variant,
			Status:      r.Status,
			ReviewState: r.ReviewState,
			TargetHash:  r.TargetHash,
			ContentHash: r.ContentHash,
		}
		d.Nodes = append(d.Nodes, contextgraph.UnitStateNode(scope, u))

		sb, found := index[r.ItemName+"\x00"+r.Unit]
		if !found || sb.contentHash == "" {
			continue
		}
		if !seenBlocks[sb.contentHash] {
			seenBlocks[sb.contentHash] = true
			d.Nodes = append(d.Nodes, contextgraph.BlockNode(scope, contextgraph.Block{
				ContentKey: sb.contentHash,
				BlockID:    sb.sourceID,
				Document:   sb.itemName,
			}))
		}
		d.Edges = append(d.Edges, contextgraph.BlessesEdge(scope, u, sb.contentHash))
	}
	return d, nil
}
