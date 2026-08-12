package contextgraph

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/graph"
)

// EdgeReader and NodeFinder are the read surface these queries traverse — narrow
// slices of graph.GraphStore that the local SQLite store and the server's
// Postgres store both satisfy. The queries below are written once against them,
// which is what "the same query shapes answer locally and on the server" means
// concretely: not two implementations agreeing by convention, but one
// implementation.
type EdgeReader interface {
	EdgesOf(ctx context.Context, nodeID string, direction graph.Direction, labels ...string) ([]*graph.Edge, error)
}

// NodeFinder finds nodes by label and property equality — the shape a dimension
// filter takes, since dimensions are fields.
type NodeFinder interface {
	FindNodes(ctx context.Context, label string, properties map[string]string) ([]*graph.Node, error)
}

// Reader is both halves, which is what a real store is.
type Reader interface {
	EdgeReader
	NodeFinder
}

// Use is one place a concept is used: a block, where it sits, and the
// collection it belongs to.
type Use struct {
	ConceptID  string `json:"concept_id"`
	Scope      Scope  `json:"scope"`
	ContentKey string `json:"content_key"`
	Collection string `json:"collection,omitempty"`
}

// Uses answers "term → blocks → collection" by traversal: in over the concept
// node's uses_term edges to the blocks that use it, then out over each block's
// in_collection edges.
//
// filter pins the dimensions the caller cares about; an empty dimension is
// "any". Its workspace also selects the concept node, so a local project
// (workspace empty) reaches its own vocabulary and a connected one reaches the
// workspace's. Pinning the project is how the same call serves a project-scoped
// surface, and leaving it free is how it serves a workspace rollup.
//
// This is the query the blocks×terms join cannot express on its own: the join
// reads a block's collection as a column, while here a collection is a node the
// traversal reaches, so the same walk extends to relationships a two-table join
// has no shape for. For one concept in one project it returns the same set as
// the join, which is what makes the two answers checkable against each other.
func Uses(ctx context.Context, r EdgeReader, filter Scope, conceptID string) ([]Use, error) {
	if r == nil {
		return nil, errors.New("contextgraph: no graph store")
	}
	conceptNode := ConceptNodeID(filter, conceptID)
	usesEdges, err := r.EdgesOf(ctx, conceptNode, graph.Incoming, EdgeUsesTerm)
	if err != nil {
		return nil, fmt.Errorf("contextgraph: uses_term edges of %q: %w", conceptID, err)
	}

	seen := map[string]bool{}
	var out []Use
	add := func(s Scope, contentKey, collection string) {
		key := s.Key() + "\x00" + contentKey + "\x00" + collection
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Use{ConceptID: conceptID, Scope: s, ContentKey: contentKey, Collection: collection})
	}

	blocksDone := map[string]bool{}
	for _, ue := range usesEdges {
		blockNode := ue.Source
		if blocksDone[blockNode] {
			continue
		}
		blocksDone[blockNode] = true
		parsed, ok := ParseNodeID(blockNode)
		if !ok || parsed.Label != NodeBlock || !filter.Contains(parsed.Scope) {
			continue
		}

		collEdges, err := r.EdgesOf(ctx, blockNode, graph.Outgoing, EdgeInCollection)
		if err != nil {
			return nil, fmt.Errorf("contextgraph: in_collection edges of %q: %w", blockNode, err)
		}
		if len(collEdges) == 0 {
			add(parsed.Scope, parsed.Local, "")
			continue
		}
		for _, ce := range collEdges {
			target, ok := ParseNodeID(ce.Target)
			if !ok || target.Label != NodeCollection {
				continue
			}
			add(parsed.Scope, parsed.Local, target.Local)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope.Key() < out[j].Scope.Key()
		}
		if out[i].ContentKey != out[j].ContentKey {
			return out[i].ContentKey < out[j].ContentKey
		}
		return out[i].Collection < out[j].Collection
	})
	return out, nil
}

// ProjectsUsingConcept answers the workspace question: which projects use this
// concept.
//
// It is two hops through one workspace-scoped concept node, which is exactly
// why the concept is not per-project. There is no project→project edge to
// follow and there never will be — projects relate by co-occurrence through the
// vocabulary they share, so the answer is the distinct scopes on the far side
// of the concept's uses_term edges. The stream dimension is dropped: a project
// that uses a concept on one stream uses it.
func ProjectsUsingConcept(ctx context.Context, r EdgeReader, filter Scope, conceptID string) ([]Scope, error) {
	if r == nil {
		return nil, errors.New("contextgraph: no graph store")
	}
	conceptNode := ConceptNodeID(filter, conceptID)
	usesEdges, err := r.EdgesOf(ctx, conceptNode, graph.Incoming, EdgeUsesTerm)
	if err != nil {
		return nil, fmt.Errorf("contextgraph: uses_term edges of %q: %w", conceptID, err)
	}
	seen := map[Scope]bool{}
	var out []Scope
	for _, ue := range usesEdges {
		parsed, ok := ParseNodeID(ue.Source)
		if !ok || parsed.Label != NodeBlock || !filter.Contains(parsed.Scope) {
			continue
		}
		project := Scope{Workspace: parsed.Scope.Workspace, Project: parsed.Scope.Project}
		if seen[project] {
			continue
		}
		seen[project] = true
		out = append(out, project)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out, nil
}

// Placement is one collection at one coordinate.
type Placement struct {
	Scope      Scope  `json:"scope"`
	Collection string `json:"collection"`
	Profile    string `json:"profile,omitempty"`
	Channel    string `json:"channel,omitempty"`
}

// CollectionsAtCoordinate answers "what is governed here": the collections
// bound to one point in the context space, across every project the filter
// leaves free.
//
// at.At selects the instant governance is resolved at, so a binding whose
// profile window has closed drops out — the same half-open window the recipe
// declares. The zero instant is the as-declared view: every binding, window
// unapplied.
func CollectionsAtCoordinate(ctx context.Context, r EdgeReader, filter Scope, profile, channel string, at graph.Scope) ([]Placement, error) {
	if r == nil {
		return nil, errors.New("contextgraph: no graph store")
	}
	node := CoordinateNodeID(filter, profile, channel)
	edges, err := r.EdgesOf(ctx, node, graph.Incoming, EdgeGovernedBy)
	if err != nil {
		return nil, fmt.Errorf("contextgraph: governed_by edges of %q: %w", node, err)
	}
	var out []Placement
	for _, e := range edges {
		parsed, ok := ParseNodeID(e.Source)
		if !ok || parsed.Label != NodeCollection || !filter.Contains(parsed.Scope) {
			continue
		}
		if !at.At.IsZero() && !e.Validity.Matches(at) {
			continue
		}
		out = append(out, Placement{
			Scope:      parsed.Scope,
			Collection: parsed.Local,
			Profile:    e.Properties[PropProfile],
			Channel:    e.Properties[PropChannel],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope.Key() < out[j].Scope.Key()
		}
		return out[i].Collection < out[j].Collection
	})
	return out, nil
}

// Blessing is one unit-state record's hold on one block.
type Blessing struct {
	Scope      Scope  `json:"scope"`
	ContentKey string `json:"content_key"`
	Unit       string `json:"unit"`
	Variant    string `json:"variant,omitempty"`
	Status     string `json:"status,omitempty"`
	TargetHash string `json:"target_hash,omitempty"`
}

// BlessingsOfBlock answers "which decision covers this unit, at which basis":
// the unit-state records holding one block, one per locale variant.
func BlessingsOfBlock(ctx context.Context, r EdgeReader, s Scope, contentKey string) ([]Blessing, error) {
	if r == nil {
		return nil, errors.New("contextgraph: no graph store")
	}
	node := BlockNodeID(s, contentKey)
	edges, err := r.EdgesOf(ctx, node, graph.Incoming, EdgeBlesses)
	if err != nil {
		return nil, fmt.Errorf("contextgraph: blesses edges of %q: %w", node, err)
	}
	var out []Blessing
	for _, e := range edges {
		parsed, ok := ParseNodeID(e.Source)
		if !ok || parsed.Label != NodeUnitState {
			continue
		}
		out = append(out, Blessing{
			Scope:      parsed.Scope,
			ContentKey: contentKey,
			Unit:       e.Properties[PropUnit],
			Variant:    e.Properties[PropVariant],
			Status:     e.Properties[PropStatus],
			TargetHash: e.Properties[PropTargetHash],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Unit != out[j].Unit {
			return out[i].Unit < out[j].Unit
		}
		return out[i].Variant < out[j].Variant
	})
	return out, nil
}

// BlocksWithContentKey answers the cross-project "same wording" question: every
// block node carrying one content key, wherever it sits.
//
// It is a property equality query rather than a traversal, because block nodes
// are per-project by design. Two projects holding identical wording hold two
// nodes; making them one would say the two instances are governed together,
// which they are not.
func BlocksWithContentKey(ctx context.Context, r NodeFinder, filter Scope, contentKey string) ([]Scope, error) {
	if r == nil {
		return nil, errors.New("contextgraph: no graph store")
	}
	nodes, err := r.FindNodes(ctx, NodeBlock, map[string]string{PropContentKey: contentKey})
	if err != nil {
		return nil, fmt.Errorf("contextgraph: blocks with content key %q: %w", contentKey, err)
	}
	var out []Scope
	seen := map[Scope]bool{}
	for _, n := range nodes {
		parsed, ok := ParseNodeID(n.ID)
		if !ok || parsed.Label != NodeBlock || !filter.Contains(parsed.Scope) {
			continue
		}
		if seen[parsed.Scope] {
			continue
		}
		seen[parsed.Scope] = true
		out = append(out, parsed.Scope)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out, nil
}
