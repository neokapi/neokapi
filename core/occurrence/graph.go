package occurrence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/neokapi/neokapi/core/contextgraph"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/terms"
)

// The context graph's first materialized relationship: which blocks use which
// concepts, and which collections those blocks sit in.
//
//	(block) ──uses_term──▶ (concept)
//	(block) ──in_collection──▶ (collection)
//
// Identity is contextgraph's: blocks and collections are qualified by the scope
// they sit in, the concept is the workspace's. The occurrence coordinate —
// which locale's text matched, at which position — is edge data, not node
// identity, because the same content used in two places is one block.

// Standing is how a term stands: the status it carries and the window it
// carries it in.
//
// It is deliberately not a field on Occurrence. A use is a fact about the text
// — the word is there, at these offsets — while the standing is the terms
// store's ruling about the word, which can change without a character of
// content moving. The graph carries both because "which projects use this" and
// "and should they" are one question at workspace reach, and BuildGraph joins
// them at the one place it has both to hand.
type Standing struct {
	Status   string
	Validity *graph.Validity
}

// Edge is the graph relationship this occurrence contributes to: the block uses
// the concept, in this term, in the occurrence's locale, within the given scope.
// Its id drops the match position, so every use of one term in one block folds
// onto one relationship — BuildGraph counts them into the edge's `count`
// property rather than emitting an edge per position.
func (o Occurrence) Edge(scope contextgraph.Scope, standing Standing) graph.Edge {
	return contextgraph.UsesTermEdge(scope, contextgraph.UsesTerm{
		ContentKey: o.BlockHash,
		ConceptID:  o.ConceptID,
		Term:       o.Term,
		TermLocale: o.TermLocale,
		Status:     standing.Status,
		Validity:   standing.Validity,
		Locale:     o.Locale,
		Collection: o.Collection,
		Document:   o.Document,
		BlockID:    o.BlockID,
	})
}

// GraphDelta is the set of nodes and edges a materialization pass produces. The
// caller upserts it into the graph store; nothing here touches a database, so
// the shape is testable without one.
type GraphDelta struct {
	Nodes []graph.Node
	Edges []graph.Edge
}

// BuildGraph computes the uses_term / in_collection subgraph for every concept
// in the terms store over the project's blocks, at the given scope. It reuses
// Find's matching semantics exactly — one Find per concept — so the graph can
// never disagree with the live occurrence query about what counts as a use.
//
// The delta is deterministic: nodes and edges come back in id order, and repeat
// uses of one term in one block fold onto one counted edge.
func BuildGraph(ctx context.Context, src Sources, scope contextgraph.Scope) (*GraphDelta, error) {
	if src.Terms == nil {
		return nil, errors.New("occurrence: no terms store")
	}
	concepts, err := src.Terms.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("occurrence: list concepts: %w", err)
	}

	agg := newGraphAggregator(scope)
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
		standings := standingsOf(c)
		for _, occ := range res.Occurrences {
			agg.occurrence(occ, standings[termKey(occ.Term, occ.TermLocale)])
		}
	}
	return agg.delta(), nil
}

// graphAggregator folds occurrences into a deduplicated node/edge set.
type graphAggregator struct {
	scope     contextgraph.Scope
	nodes     map[string]graph.Node
	usesEdges map[string]*graph.Edge
	usesCount map[string]int
	inColl    map[string]graph.Edge
}

func newGraphAggregator(scope contextgraph.Scope) *graphAggregator {
	return &graphAggregator{
		scope:     scope,
		nodes:     map[string]graph.Node{},
		usesEdges: map[string]*graph.Edge{},
		usesCount: map[string]int{},
		inColl:    map[string]graph.Edge{},
	}
}

func (g *graphAggregator) concept(id, termHint string) {
	n := contextgraph.ConceptNode(g.scope, id, termHint)
	g.nodes[n.ID] = n
}

// standingsOf indexes a concept's terms by the (text, locale) pair an
// occurrence names them with, so the edge for a use carries the ruling on the
// exact spelling that was used rather than the concept's first one.
func standingsOf(c terms.Concept) map[string]Standing {
	out := make(map[string]Standing, len(c.Terms))
	for _, t := range c.Terms {
		out[termKey(t.Text, string(t.Locale))] = Standing{
			Status:   string(t.Status),
			Validity: t.Validity,
		}
	}
	return out
}

func termKey(text, locale string) string { return text + "\x00" + locale }

func (g *graphAggregator) occurrence(o Occurrence, standing Standing) {
	blockID := contextgraph.BlockNodeID(g.scope, o.BlockHash)
	if _, ok := g.nodes[blockID]; !ok {
		g.nodes[blockID] = contextgraph.BlockNode(g.scope, contextgraph.Block{
			ContentKey: o.BlockHash,
			BlockID:    o.BlockID,
			Document:   o.Document,
			Collection: o.Collection,
		})
	}
	// The concept node already exists from BuildGraph's concept() call.

	e := o.Edge(g.scope, standing)
	g.usesCount[e.ID]++
	if _, ok := g.usesEdges[e.ID]; !ok {
		g.usesEdges[e.ID] = &e
	}

	if o.Collection != "" {
		coll := contextgraph.CollectionNode(g.scope, o.Collection)
		if _, ok := g.nodes[coll.ID]; !ok {
			g.nodes[coll.ID] = coll
		}
		in := contextgraph.InCollectionEdge(g.scope, o.BlockHash, o.Collection)
		if _, ok := g.inColl[in.ID]; !ok {
			g.inColl[in.ID] = in
		}
	}
}

func (g *graphAggregator) delta() *GraphDelta {
	d := &GraphDelta{}
	for _, n := range g.nodes {
		d.Nodes = append(d.Nodes, n)
	}
	for id, e := range g.usesEdges {
		e.Properties[contextgraph.PropCount] = strconv.Itoa(g.usesCount[id])
		d.Edges = append(d.Edges, *e)
	}
	for _, e := range g.inColl {
		d.Edges = append(d.Edges, e)
	}
	sort.Slice(d.Nodes, func(i, j int) bool { return d.Nodes[i].ID < d.Nodes[j].ID })
	sort.Slice(d.Edges, func(i, j int) bool { return d.Edges[i].ID < d.Edges[j].ID })
	return d
}
