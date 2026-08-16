// Package contextgraph is the identity vocabulary of the context graph: the
// node and edge labels, the scope tuple a node is qualified by, and the id
// scheme that keeps one project's subgraph distinguishable inside a store that
// holds many.
//
// The same vocabulary is written locally into `.kapi/work/store.db` and on the
// server into Postgres. Locally the dimensions are pinned — the store IS one
// project — while the server carries real workspace, project and stream
// identities. Nothing else differs, which is what lets a query written against
// one store run against the other ([C-03]).
//
// # Two kinds of node
//
// The split decides what can be asked:
//
//   - VOCABULARY nodes — concepts, coordinates — are workspace-scoped. One
//     concept is one node however many projects use it, which is what makes
//     "which projects use this concept" a two-hop traversal instead of a join
//     across project boundaries.
//   - INSTANCE nodes — blocks, collections, unit states — carry (project,
//     stream) in their identity. Two projects' `docs` collections are two
//     nodes. Two projects' identical wording is two block nodes carrying the
//     same content key, and "same wording" is a content-key equality query:
//     sharing one node would conflate governance scopes, and an instance sits
//     somewhere.
//
// # Dimensions are fields, not containment edges
//
// Every node carries its scope tuple as properties as well as in its id, so
// slicing a view by project or stream is a filter rather than a traversal.
// There is no project→project edge anywhere: projects relate by co-occurrence
// through the vocabulary they share.
//
// # Identity, and what a rename costs
//
// An id carries the scope tuple, so a scope value that changes changes the id.
// That is safe because every writer clears the scope it is about to write in
// the same pass: a rename is a deterministic re-key, and no row survives under
// the old one. Locally the project dimension is the recipe's project name — the
// only identity a disconnected project has — so a rename re-keys the whole
// projection, which is the rebuild the writer performs every pass anyway.
// Server-side the project dimension is the workspace-issued project id, which a
// rename does not touch, so the display name rides as a node property and the
// rule costs nothing.
//
// Within a scope, identity is durable: a block is its content key ([F-03]),
// not a reader's positional id, so a re-parse that renumbers a document rewrites
// the same rows rather than orphaning them.
//
// [F-03]: https://neokapi.org/docs/contribute/architecture/foundations/f-03-identity
// [C-03]: https://neokapi.org/docs/contribute/architecture/context/c-03-context-store-and-graph
package contextgraph

import "strings"

// Node labels. A label is the node kind, and it decides which dimensions the
// node's scope key carries: vocabulary labels are workspace-only, instance
// labels carry the full tuple.
const (
	// NodeBlock is a unit of source content, identified by its content key
	// within a (project, stream).
	NodeBlock = "block"
	// NodeCollection is a content collection, identified by its label within a
	// (project, stream).
	NodeCollection = "collection"
	// NodeUnitState is one unit's workflow state in one locale variant — the
	// record that blesses a translation at a content hash.
	NodeUnitState = "unit_state"

	// NodeConcept is a terminology concept, workspace-scoped: concept ids are
	// issued by the workspace, and one concept is one node however many
	// projects use it.
	NodeConcept = "concept"
	// NodeCoordinate is a point in the context space — a (profile, channel)
	// pair — workspace-scoped, because governance vocabulary is the workspace's
	// and a project binds to it rather than owning it.
	NodeCoordinate = "coordinate"
)

// Edge labels.
const (
	// EdgeUsesTerm records that a block's text uses a concept's term.
	EdgeUsesTerm = "uses_term"
	// EdgeInCollection records that a block sits in a collection.
	EdgeInCollection = "in_collection"
	// EdgeGovernedBy records that a collection's content is governed at a
	// coordinate. The edge carries the governing profile's validity window, so
	// "what governed here on that date" is answered by the same temporal model
	// the recipe declares.
	EdgeGovernedBy = "governed_by"
	// EdgeBlesses records that a unit-state record blesses a block's
	// translation at a target hash. A translation edited after the decision
	// changes the hash, which is how a stale blessing is visible without
	// re-reading the deliverable.
	EdgeBlesses = "blesses"
)

// Property keys. Every node carries the non-empty components of its scope
// tuple under the first three; the rest are per-label.
//
// An empty dimension is written as an ABSENT key rather than an empty string,
// because a JSON property filter compares against a value and absence is not
// the empty string. Locally that means the scope properties are simply not
// there, which is correct: a disconnected project has no workspace to name.
const (
	PropWorkspace = "workspace"
	PropProject   = "project"
	PropStream    = "stream"

	// PropProjectName is the project's display name where it differs from the
	// identity in the id — the server's case, where a rename updates this
	// property and nothing else.
	PropProjectName = "project_name"

	// PropContentKey is a block's durable content key, carried as a property so
	// "which projects hold this same wording" is an equality query rather than a
	// shared node.
	PropContentKey = "content_key"
	PropDocument   = "document"
	PropBlockID    = "block_id"
	PropCollection = "collection"

	PropConceptID  = "concept_id"
	PropTerm       = "term"
	PropTermLocale = "term_locale"
	PropLocale     = "locale"
	PropCount      = "count"

	PropProfile = "profile"
	PropChannel = "channel"

	PropUnit        = "unit"
	PropVariant     = "variant"
	PropStatus      = "status"
	PropTargetHash  = "target_hash"
	PropReviewState = "review_state"
	// PropContentHash is the decision's BASIS: the hash of the SOURCE wording it
	// blessed. Paired with PropTargetHash it makes "at which basis" answerable by
	// traversal — a blessing carries both halves of what it approved, so a reader
	// can see that the source moved, not only that the translation did.
	PropContentHash = "content_hash"
)

// Scope is the tuple an instance node is qualified by. It is the plan's "scope
// tuple": the dimensions a view slices on, carried as fields.
//
// It is unrelated to graph.Scope, which is the temporal evaluation point a
// validity window is matched against. This one says WHERE a node sits; that one
// says WHEN an edge is in force.
//
// The zero Scope is the local, disconnected project: no workspace, no stream,
// and a project dimension the caller fills from the recipe.
type Scope struct {
	// Workspace is the workspace identity. Empty until a project connects,
	// which is the ordinary local case.
	Workspace string
	// Project is the project identity: the workspace-issued project id on the
	// server, the recipe's project name locally.
	Project string
	// Stream is the stream identity within the project. Empty when the project
	// is not stream-bound.
	Stream string
}

// WorkspaceScope drops the instance dimensions, leaving the scope a vocabulary
// node is qualified by. Two projects in one workspace resolve the same concept
// node through it, which is the whole mechanism behind the cross-project
// questions.
func (s Scope) WorkspaceScope() Scope { return Scope{Workspace: s.Workspace} }

// Key renders the scope as the id segment. It is always three fields, so the
// segment says which dimensions the node is qualified by rather than leaving it
// to be inferred from the label: a vocabulary node's key has two empty fields.
func (s Scope) Key() string {
	return escapeField(s.Workspace) + "/" + escapeField(s.Project) + "/" + escapeField(s.Stream)
}

// Properties returns the non-empty dimensions as node/edge properties.
func (s Scope) Properties() map[string]string {
	props := make(map[string]string, 3)
	s.WriteProperties(props)
	return props
}

// WriteProperties adds the non-empty dimensions to an existing property map.
func (s Scope) WriteProperties(props map[string]string) {
	if props == nil {
		return
	}
	if s.Workspace != "" {
		props[PropWorkspace] = s.Workspace
	}
	if s.Project != "" {
		props[PropProject] = s.Project
	}
	if s.Stream != "" {
		props[PropStream] = s.Stream
	}
}

// Contains reports whether other sits within this scope, treating an empty
// dimension here as "any". It is the filter a query applies when the caller
// pinned some dimensions and left the rest free — the server's explorer slicing
// a workspace view down to one project, and the local surface asserting the one
// project it has.
func (s Scope) Contains(other Scope) bool {
	if s.Workspace != "" && s.Workspace != other.Workspace {
		return false
	}
	if s.Project != "" && s.Project != other.Project {
		return false
	}
	if s.Stream != "" && s.Stream != other.Stream {
		return false
	}
	return true
}

// NodeID is a parsed node id: the kind, where it sits, and what it is within
// that scope.
type NodeID struct {
	Label string
	Scope Scope
	// Local is the identity within the scope: a content key for a block, a
	// label for a collection, a concept id for a concept.
	Local string
}

// String renders the id.
func (n NodeID) String() string { return n.Label + ":" + n.Scope.Key() + ":" + escapeField(n.Local) }

// ParseNodeID reads an id back into its parts. It reports false for anything
// this package did not write, so a traversal over a store that also holds a
// different vocabulary skips what it does not recognise rather than
// misreading it.
func ParseNodeID(id string) (NodeID, bool) {
	label, rest, ok := strings.Cut(id, ":")
	if !ok || label == "" {
		return NodeID{}, false
	}
	scopeKey, local, ok := strings.Cut(rest, ":")
	if !ok {
		return NodeID{}, false
	}
	fields := strings.Split(scopeKey, "/")
	if len(fields) != 3 {
		return NodeID{}, false
	}
	return NodeID{
		Label: label,
		Scope: Scope{
			Workspace: unescapeField(fields[0]),
			Project:   unescapeField(fields[1]),
			Stream:    unescapeField(fields[2]),
		},
		Local: unescapeField(local),
	}, true
}

// BlockNodeID is the graph id of a block, keyed on its content key within the
// scope.
func BlockNodeID(s Scope, contentKey string) string {
	return NodeID{Label: NodeBlock, Scope: s, Local: contentKey}.String()
}

// CollectionNodeID is the graph id of a collection within the scope.
func CollectionNodeID(s Scope, label string) string {
	return NodeID{Label: NodeCollection, Scope: s, Local: label}.String()
}

// UnitStateNodeID is the graph id of one unit's state, in one document, in one
// locale variant.
//
// All three are identity. The variant, because a unit has one state per variant
// and a variant-less key would collapse them. The document, because a unit id is
// unique inside its document and nowhere wider — every page of a prose
// collection carries an `h` and a `p`, so without it two pages are one node and
// the decision recorded last answers for both.
//
// The document and the unit are escaped before they are joined, so the
// separators are the only unescaped ones in the composition and no two
// (document, unit, variant) triples can render the same id.
func UnitStateNodeID(s Scope, document, unit, variant string) string {
	local := escapeField(document) + "/" + escapeField(unit) + "@" + variant
	return NodeID{Label: NodeUnitState, Scope: s, Local: local}.String()
}

// ConceptNodeID is the graph id of a concept. It drops the instance dimensions:
// the concept is the workspace's, and two projects using it reach one node.
func ConceptNodeID(s Scope, conceptID string) string {
	return NodeID{Label: NodeConcept, Scope: s.WorkspaceScope(), Local: conceptID}.String()
}

// CoordinateNodeID is the graph id of a point in the context space. Like a
// concept it is workspace-scoped, which is what lets a workspace hold ONE
// coordinate vocabulary that projects bind to rather than each inventing its
// own.
func CoordinateNodeID(s Scope, profile, channel string) string {
	return NodeID{Label: NodeCoordinate, Scope: s.WorkspaceScope(), Local: coordinateLocal(profile, channel)}.String()
}

// coordinateLocal renders a (profile, channel) pair the way a recipe writes a
// channel reference, so a coordinate node id reads back as the reference that
// named it.
func coordinateLocal(profile, channel string) string {
	if profile == "" {
		return channel
	}
	return profile + "/" + channel
}

// EdgeID keys one relationship. Node ids never contain "|" — every component
// they are built from is escaped — so joining them with it is unambiguous, and
// an edge's id says which nodes it relates without a lookup.
//
// Discriminators are the parts of the relationship that make two edges between
// the same pair distinct (the locale a term was matched in, say). What is NOT a
// discriminator is a match position: two uses of one term in one block are one
// relationship, counted on the edge.
func EdgeID(label, source, target string, discriminators ...string) string {
	var b strings.Builder
	b.WriteString(label)
	b.WriteString("|")
	b.WriteString(source)
	b.WriteString("|")
	b.WriteString(target)
	for _, d := range discriminators {
		b.WriteString("|")
		b.WriteString(escapeField(d))
	}
	return b.String()
}

// escapeField makes a value safe to place in an id: the separators this scheme
// uses ("/", ":", "|") and the escape character itself are percent-encoded, so
// two different tuples can never render to one id.
func escapeField(s string) string {
	if !strings.ContainsAny(s, `%/:|`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '%':
			b.WriteString("%25")
		case '/':
			b.WriteString("%2F")
		case ':':
			b.WriteString("%3A")
		case '|':
			b.WriteString("%7C")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unescapeField reverses escapeField. An unrecognised escape is left verbatim:
// the function reads ids this package wrote, and a value that was never escaped
// must survive the round trip unchanged.
func unescapeField(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) {
			switch strings.ToUpper(s[i : i+3]) {
			case "%25":
				b.WriteByte('%')
				i += 3
				continue
			case "%2F":
				b.WriteByte('/')
				i += 3
				continue
			case "%3A":
				b.WriteByte(':')
				i += 3
				continue
			case "%7C":
				b.WriteByte('|')
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
