package contextgraph

import (
	"strconv"

	"github.com/neokapi/neokapi/core/graph"
)

// The constructors below are the whole of "the server writes the same
// subgraph". Neither side assembles a node or an edge by hand: the local
// materializer reads the project store, the server reads Postgres, and both
// hand what they read to these functions. A shape that drifted would have to
// drift here, in one place, for both.

// Block describes one block, as a graph node's worth of it.
type Block struct {
	// ContentKey is the block's durable identity within its project.
	ContentKey string
	// BlockID is the block's structural name inside its document.
	BlockID string
	// Document is the file the block was read from, project-relative.
	Document string
	// Collection is the collection the block sits in.
	Collection string
}

// BlockNode builds the node for one block. The content key is both the node's
// local identity and a property: the id places the block in its project, the
// property lets a query find the same wording in another one.
func BlockNode(s Scope, b Block) graph.Node {
	props := s.Properties()
	props[PropContentKey] = b.ContentKey
	putIf(props, PropBlockID, b.BlockID)
	putIf(props, PropDocument, b.Document)
	putIf(props, PropCollection, b.Collection)
	return graph.Node{ID: BlockNodeID(s, b.ContentKey), Label: NodeBlock, Properties: props}
}

// CollectionNode builds the node for one collection.
func CollectionNode(s Scope, label string) graph.Node {
	props := s.Properties()
	props[PropCollection] = label
	return graph.Node{ID: CollectionNodeID(s, label), Label: NodeCollection, Properties: props}
}

// ConceptNode builds the node for one concept. termHint is the concept's first
// term, carried so a graph dump is legible without a second lookup — a hint,
// never identity.
func ConceptNode(s Scope, conceptID, termHint string) graph.Node {
	props := s.WorkspaceScope().Properties()
	props[PropConceptID] = conceptID
	putIf(props, PropTerm, termHint)
	return graph.Node{ID: ConceptNodeID(s, conceptID), Label: NodeConcept, Properties: props}
}

// CoordinateNode builds the node for one point in the context space. A point
// with no profile is the project's default point, which is a real place content
// sits at even though the recipe names nothing.
func CoordinateNode(s Scope, profile, channel string) graph.Node {
	props := s.WorkspaceScope().Properties()
	putIf(props, PropProfile, profile)
	putIf(props, PropChannel, channel)
	return graph.Node{ID: CoordinateNodeID(s, profile, channel), Label: NodeCoordinate, Properties: props}
}

// UnitState describes one unit's workflow state in one locale variant.
type UnitState struct {
	// Unit is the unit identity — the block's structural key, as the state
	// record and the deliverable both address it.
	Unit string
	// Variant is the locale (and optional tone/channel) the state applies to.
	Variant string
	// Status is the target ladder position the record records.
	Status string
	// ReviewState is the workflow decision recorded for the unit.
	ReviewState string
	// TargetHash is the content hash of the translation the record blesses.
	TargetHash string
}

// UnitStateNode builds the node for one unit-state record.
func UnitStateNode(s Scope, u UnitState) graph.Node {
	props := s.Properties()
	props[PropUnit] = u.Unit
	putIf(props, PropVariant, u.Variant)
	putIf(props, PropStatus, u.Status)
	putIf(props, PropReviewState, u.ReviewState)
	putIf(props, PropTargetHash, u.TargetHash)
	return graph.Node{ID: UnitStateNodeID(s, u.Unit, u.Variant), Label: NodeUnitState, Properties: props}
}

// UsesTerm describes one block-uses-concept relationship.
type UsesTerm struct {
	ContentKey string
	ConceptID  string
	// Term and TermLocale are the term as the terms store holds it. They are not
	// necessarily how the matched text spells it: matching folds case.
	Term       string
	TermLocale string
	// Locale is the language of the text the term was found in — empty for the
	// block's own source text.
	Locale     string
	Collection string
	Document   string
	BlockID    string
	// Count is how many times the term occurs in that text. Zero renders as one:
	// an edge exists because there was a use.
	Count int
}

// UsesTermEdge builds the block→concept edge. Locale is part of the identity —
// an English term standing in a Norwegian target is a different finding from the
// same term in the source — but the match position is not: repeated uses of one
// term in one block fold onto one counted edge.
func UsesTermEdge(s Scope, u UsesTerm) graph.Edge {
	props := s.Properties()
	putIf(props, PropTerm, u.Term)
	putIf(props, PropTermLocale, u.TermLocale)
	putIf(props, PropLocale, u.Locale)
	putIf(props, PropCollection, u.Collection)
	putIf(props, PropDocument, u.Document)
	putIf(props, PropBlockID, u.BlockID)
	props[PropCount] = strconv.Itoa(max(u.Count, 1))
	source := BlockNodeID(s, u.ContentKey)
	target := ConceptNodeID(s, u.ConceptID)
	return graph.Edge{
		ID:         EdgeID(EdgeUsesTerm, source, target, u.Locale),
		Source:     source,
		Target:     target,
		Label:      EdgeUsesTerm,
		Properties: props,
	}
}

// InCollectionEdge builds the block→collection edge.
func InCollectionEdge(s Scope, contentKey, collection string) graph.Edge {
	props := s.Properties()
	props[PropCollection] = collection
	source := BlockNodeID(s, contentKey)
	target := CollectionNodeID(s, collection)
	return graph.Edge{
		ID:         EdgeID(EdgeInCollection, source, target),
		Source:     source,
		Target:     target,
		Label:      EdgeInCollection,
		Properties: props,
	}
}

// GovernedByEdge builds the collection→coordinate edge: the point the
// collection's content is governed at, carrying the governing profile's
// declared window so "what governed here in March" is the same temporal query
// the recipe's validity model already answers.
func GovernedByEdge(s Scope, collection, profile, channel string, validity *graph.Validity) graph.Edge {
	props := s.Properties()
	props[PropCollection] = collection
	putIf(props, PropProfile, profile)
	putIf(props, PropChannel, channel)
	source := CollectionNodeID(s, collection)
	target := CoordinateNodeID(s, profile, channel)
	return graph.Edge{
		ID:         EdgeID(EdgeGovernedBy, source, target),
		Source:     source,
		Target:     target,
		Label:      EdgeGovernedBy,
		Properties: props,
		Validity:   validity,
	}
}

// BlessesEdge builds the unit-state→block edge: which record blesses which
// block, at which target hash. The hash is edge data rather than identity, so a
// re-translation moves the same edge rather than accumulating one per revision
// — and comparing it against the deliverable is how a stale blessing shows.
func BlessesEdge(s Scope, u UnitState, contentKey string) graph.Edge {
	props := s.Properties()
	props[PropUnit] = u.Unit
	putIf(props, PropVariant, u.Variant)
	putIf(props, PropTargetHash, u.TargetHash)
	putIf(props, PropStatus, u.Status)
	putIf(props, PropReviewState, u.ReviewState)
	source := UnitStateNodeID(s, u.Unit, u.Variant)
	target := BlockNodeID(s, contentKey)
	return graph.Edge{
		ID:         EdgeID(EdgeBlesses, source, target),
		Source:     source,
		Target:     target,
		Label:      EdgeBlesses,
		Properties: props,
	}
}

func putIf(props map[string]string, key, value string) {
	if value != "" {
		props[key] = value
	}
}
