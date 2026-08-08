package occurrence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/graph"
)

// The local context graph's first materialized relationship: which blocks use
// which concepts, and which collections those blocks sit in. Three node kinds
// and two edge kinds, all keyed on durable identity so a re-extract that
// renumbers a document rewrites the same rows rather than orphaning them:
//
//	(block) ──uses_term──▶ (concept)
//	(block) ──in_collection──▶ (collection)
//
// A block node keys on its content key (AD-036), a concept node on its concept
// id, a collection node on its label. The occurrence coordinate — which locale's
// text matched, at which position — is edge data, not node identity, because the
// same content used in two places is one block.
const (
	// NodeLabelBlock is a unit of source content, identified by its content key.
	NodeLabelBlock = "block"
	// NodeLabelConcept is a terminology concept, identified by its concept id.
	NodeLabelConcept = "concept"
	// NodeLabelCollection is a content collection, identified by its label.
	NodeLabelCollection = "collection"

	// EdgeLabelUsesTerm records that a block's text uses a concept's term.
	EdgeLabelUsesTerm = "uses_term"
	// EdgeLabelInCollection records that a block sits in a collection.
	EdgeLabelInCollection = "in_collection"

	blockNodePrefix      = "block:"
	conceptNodePrefix    = "concept:"
	collectionNodePrefix = "collection:"
)

// BlockNodeID is the graph id of the block with the given content key.
func BlockNodeID(contentKey string) string { return blockNodePrefix + contentKey }

// ConceptNodeID is the graph id of the concept with the given id.
func ConceptNodeID(conceptID string) string { return conceptNodePrefix + conceptID }

// CollectionNodeID is the graph id of the collection with the given label.
func CollectionNodeID(collection string) string { return collectionNodePrefix + collection }

// UsesTermEdgeID keys one block-uses-concept relationship. Locale is part of the
// identity — an English term standing in a Norwegian target is a different
// finding from the same term in the source — but the match position is not: two
// uses of one term in one block are one relationship, counted, not two edges.
func UsesTermEdgeID(contentKey, conceptID, locale string) string {
	return EdgeLabelUsesTerm + ":" + contentKey + ":" + conceptID + ":" + locale
}

// InCollectionEdgeID keys one block-in-collection relationship.
func InCollectionEdgeID(contentKey, collection string) string {
	return EdgeLabelInCollection + ":" + contentKey + ":" + collection
}

func contentKeyOf(blockNodeID string) string { return strings.TrimPrefix(blockNodeID, blockNodePrefix) }
func collectionOf(collectionNodeID string) string {
	return strings.TrimPrefix(collectionNodeID, collectionNodePrefix)
}

// Edge is the graph relationship this occurrence contributes to: the block uses
// the concept, in the occurrence's locale. Its id drops the match position, so
// every use of one term in one block folds onto one relationship — BuildGraph
// counts them into the edge's `count` property rather than emitting an edge per
// position.
func (o Occurrence) Edge() graph.Edge {
	return graph.Edge{
		ID:     UsesTermEdgeID(o.BlockHash, o.ConceptID, o.Locale),
		Source: BlockNodeID(o.BlockHash),
		Target: ConceptNodeID(o.ConceptID),
		Label:  EdgeLabelUsesTerm,
		Properties: map[string]string{
			"term":        o.Term,
			"term_locale": o.TermLocale,
			"locale":      o.Locale,
			"collection":  o.Collection,
			"document":    o.Document,
			"block_id":    o.BlockID,
		},
	}
}

// GraphDelta is the set of nodes and edges a materialization pass produces. The
// caller upserts it into the graph store; nothing here touches a database, so
// the shape is testable without one.
type GraphDelta struct {
	Nodes []graph.Node
	Edges []graph.Edge
}

// BuildGraph computes the uses_term / in_collection subgraph for every concept
// in the terms store over the project's blocks. It reuses Find's matching
// semantics exactly — one Find per concept — so the graph can never disagree
// with the live occurrence query about what counts as a use.
//
// The delta is deterministic: nodes and edges come back in id order, and repeat
// uses of one term in one block fold onto one counted edge.
func BuildGraph(ctx context.Context, src Sources) (*GraphDelta, error) {
	if src.Terms == nil {
		return nil, errors.New("occurrence: no terms store")
	}
	concepts, err := src.Terms.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("occurrence: list concepts: %w", err)
	}

	agg := newGraphAggregator()
	for _, c := range concepts {
		res, err := Find(ctx, src, Query{Subject: c.ID})
		if errors.Is(err, ErrUnknownSubject) {
			// A concept with no terms resolves to nothing to search for.
			continue
		}
		if err != nil {
			return nil, err
		}
		// A readable concept-node label — the first term's text — so a graph
		// dump is legible without a second lookup. A hint, not identity.
		termHint := ""
		if len(c.Terms) > 0 {
			termHint = c.Terms[0].Text
		}
		agg.concept(c.ID, termHint)
		for _, occ := range res.Occurrences {
			agg.occurrence(occ)
		}
	}
	return agg.delta(), nil
}

// graphAggregator folds occurrences into a deduplicated node/edge set.
type graphAggregator struct {
	nodes     map[string]graph.Node
	usesEdges map[string]*graph.Edge
	inColl    map[string]graph.Edge
}

func newGraphAggregator() *graphAggregator {
	return &graphAggregator{
		nodes:     map[string]graph.Node{},
		usesEdges: map[string]*graph.Edge{},
		inColl:    map[string]graph.Edge{},
	}
}

func (g *graphAggregator) concept(id, termHint string) {
	nid := ConceptNodeID(id)
	props := map[string]string{}
	if termHint != "" {
		props["term"] = termHint
	}
	g.nodes[nid] = graph.Node{ID: nid, Label: NodeLabelConcept, Properties: props}
}

func (g *graphAggregator) occurrence(o Occurrence) {
	blockID := BlockNodeID(o.BlockHash)
	if _, ok := g.nodes[blockID]; !ok {
		g.nodes[blockID] = graph.Node{
			ID:    blockID,
			Label: NodeLabelBlock,
			Properties: map[string]string{
				"document": o.Document,
				"block_id": o.BlockID,
			},
		}
	}
	// The concept node already exists from BuildGraph's concept() call.

	edgeID := UsesTermEdgeID(o.BlockHash, o.ConceptID, o.Locale)
	if e, ok := g.usesEdges[edgeID]; ok {
		e.Properties["count"] = strconv.Itoa(atoiOr(e.Properties["count"], 0) + 1)
	} else {
		e := o.Edge()
		e.Properties["count"] = "1"
		g.usesEdges[edgeID] = &e
	}

	if o.Collection != "" {
		collID := CollectionNodeID(o.Collection)
		if _, ok := g.nodes[collID]; !ok {
			g.nodes[collID] = graph.Node{ID: collID, Label: NodeLabelCollection, Properties: map[string]string{}}
		}
		inID := InCollectionEdgeID(o.BlockHash, o.Collection)
		if _, ok := g.inColl[inID]; !ok {
			g.inColl[inID] = graph.Edge{
				ID: inID, Source: blockID, Target: collID, Label: EdgeLabelInCollection,
				Properties: map[string]string{},
			}
		}
	}
}

func (g *graphAggregator) delta() *GraphDelta {
	d := &GraphDelta{}
	for _, n := range g.nodes {
		d.Nodes = append(d.Nodes, n)
	}
	for _, e := range g.usesEdges {
		d.Edges = append(d.Edges, *e)
	}
	for _, e := range g.inColl {
		d.Edges = append(d.Edges, e)
	}
	sort.Slice(d.Nodes, func(i, j int) bool { return d.Nodes[i].ID < d.Nodes[j].ID })
	sort.Slice(d.Edges, func(i, j int) bool { return d.Edges[i].ID < d.Edges[j].ID })
	return d
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// EdgeReader is the read surface UsesInCollections traverses: the edges incident
// to a node, filtered by direction and label. It is a narrow slice of
// graph.GraphStore, which any backend satisfies, so the traversal is testable
// without a database.
type EdgeReader interface {
	EdgesOf(ctx context.Context, nodeID string, direction graph.Direction, labels ...string) ([]*graph.Edge, error)
}

// CollectionUse is one (block, collection) pair reached by traversing the graph
// out from a concept.
type CollectionUse struct {
	ConceptID  string `json:"concept_id"`
	BlockHash  string `json:"block_hash"`
	Collection string `json:"collection"`
}

// UsesInCollections answers "term → blocks → collection" by graph traversal:
// from the concept node, in over its uses_term edges to the blocks that use it,
// then out over each block's in_collection edges to the collections it sits in.
//
// This is the query the blocks×terms join cannot express on its own — the join
// reads a block's collection as a column, while here a collection is a node the
// traversal reaches, so the same walk extends to relationships (co-occurrence
// across a collection, a concept's blast radius) a two-table join has no shape
// for. For a single concept it returns the same (block, collection) set as the
// join, which is what makes the two answers checkable against each other.
func UsesInCollections(ctx context.Context, g EdgeReader, conceptID string) ([]CollectionUse, error) {
	if g == nil {
		return nil, errors.New("occurrence: no graph store")
	}
	conceptNode := ConceptNodeID(conceptID)
	usesEdges, err := g.EdgesOf(ctx, conceptNode, graph.Incoming, EdgeLabelUsesTerm)
	if err != nil {
		return nil, fmt.Errorf("occurrence: uses_term edges of %q: %w", conceptID, err)
	}

	seen := map[string]bool{}
	var out []CollectionUse
	add := func(contentKey, collection string) {
		key := contentKey + "\x00" + collection
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, CollectionUse{ConceptID: conceptID, BlockHash: contentKey, Collection: collection})
	}

	blocksDone := map[string]bool{}
	for _, ue := range usesEdges {
		blockNode := ue.Source
		if blocksDone[blockNode] {
			continue
		}
		blocksDone[blockNode] = true
		contentKey := contentKeyOf(blockNode)

		collEdges, err := g.EdgesOf(ctx, blockNode, graph.Outgoing, EdgeLabelInCollection)
		if err != nil {
			return nil, fmt.Errorf("occurrence: in_collection edges of %q: %w", blockNode, err)
		}
		if len(collEdges) == 0 {
			add(contentKey, "")
			continue
		}
		for _, ce := range collEdges {
			add(contentKey, collectionOf(ce.Target))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].BlockHash != out[j].BlockHash {
			return out[i].BlockHash < out[j].BlockHash
		}
		return out[i].Collection < out[j].Collection
	})
	return out, nil
}
