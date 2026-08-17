package contextgraph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
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

// ConceptUse is one recorded use of a concept: one block's text, in one
// language, naming the term that was used and how often.
//
// It is what the uses_term edge holds, read back. The graph records the
// relationship, not the passage — a reader who wants the words asks the content
// store for the block this names.
type ConceptUse struct {
	Scope       Scope  `json:"scope"`
	ContentKey  string `json:"content_key"`
	Collection  string `json:"collection,omitempty"`
	Document    string `json:"document,omitempty"`
	BlockID     string `json:"block_id,omitempty"`
	Locale      string `json:"locale,omitempty"`
	Term        string `json:"term,omitempty"`
	TermLocale  string `json:"term_locale,omitempty"`
	Status      string `json:"status,omitempty"`
	Discouraged bool   `json:"discouraged,omitempty"`
	Occurrences int    `json:"occurrences"`
}

// TermUse is one of a concept's terms as one project actually uses it.
//
// A concept holds several spellings, and which of them a project reaches for is
// the whole of "how does the concept stand here": a project using the preferred
// term and a project using the deprecated one both use the concept.
type TermUse struct {
	Term       string `json:"term"`
	TermLocale string `json:"term_locale,omitempty"`
	// Status is the term's lifecycle status as the edge recorded it, resolved
	// under the window the answer was asked at.
	Status      string `json:"status,omitempty"`
	Discouraged bool   `json:"discouraged,omitempty"`
	Blocks      int    `json:"blocks"`
	Occurrences int    `json:"occurrences"`
}

// ProjectUse is one project's share of a concept's use: how much of it sits
// there, in which collections, and which of the concept's terms were used.
//
// ProjectsUsingConcept answers WHICH projects; this answers how much and which
// words, off the same two hops — the counts a reader wants are already on the
// edges that walk crosses, so asking for them costs nothing extra.
type ProjectUse struct {
	Scope       Scope    `json:"scope"`
	Blocks      int      `json:"blocks"`
	Occurrences int      `json:"occurrences"`
	Collections []string `json:"collections,omitempty"`
	// Status and Discouraged are how the concept stands HERE: a project that
	// reached for a discouraged spelling is discouraged in this project, whatever
	// the workspace list says about the concept in general. Where the project
	// used several spellings, the discouraged one decides — a rule a writer is
	// breaking outranks a word they may keep using.
	Status      string       `json:"status,omitempty"`
	Discouraged bool         `json:"discouraged,omitempty"`
	Terms       []TermUse    `json:"terms,omitempty"`
	Uses        []ConceptUse `json:"uses,omitempty"`
}

// UsesByProject answers the workspace question with its numbers attached: which
// projects use this concept, how much, where, and in which words.
//
// It is ProjectsUsingConcept's traversal — in over the workspace-scoped concept
// node's uses_term edges — reading each edge rather than only the project it
// arrived from. The stream is dropped from the rollup for the same reason it is
// dropped there (a project that uses a concept on one stream uses it) and kept
// on each use, which names a place.
//
// at is the point the answer is resolved at — an instant, and the validity tags
// that go with it — so a term whose window has not opened, or whose market this
// is not, drops out of the project's answer. That is what makes "is it
// discouraged" answerable as "is it discouraged HERE": the standing is a
// property of the concept at a coordinate, never of the word. The zero instant
// is the as-declared view: every use, window unapplied.
func UsesByProject(ctx context.Context, r EdgeReader, filter Scope, conceptID string, at graph.Scope) ([]ProjectUse, error) {
	if r == nil {
		return nil, errors.New("contextgraph: no graph store")
	}
	conceptNode := ConceptNodeID(filter, conceptID)
	usesEdges, err := r.EdgesOf(ctx, conceptNode, graph.Incoming, EdgeUsesTerm)
	if err != nil {
		return nil, fmt.Errorf("contextgraph: uses_term edges of %q: %w", conceptID, err)
	}

	agg := newProjectUseAggregator()
	for _, e := range usesEdges {
		parsed, ok := ParseNodeID(e.Source)
		if !ok || parsed.Label != NodeBlock || !filter.Contains(parsed.Scope) {
			continue
		}
		if !at.At.IsZero() && !e.Validity.Matches(at) {
			continue
		}
		agg.add(parsed, e)
	}
	return agg.rollup(), nil
}

// projectUseAggregator folds uses_term edges into one row per project.
type projectUseAggregator struct {
	order   []Scope
	byScope map[Scope]*ProjectUse
	blocks  map[Scope]map[string]bool
	colls   map[Scope]map[string]bool
	terms   map[Scope]map[string]*TermUse
	// termBlocks counts distinct blocks per term, which is not the number of
	// uses: one block using a term four times is one block.
	termBlocks map[Scope]map[string]map[string]bool
}

func newProjectUseAggregator() *projectUseAggregator {
	return &projectUseAggregator{
		byScope:    map[Scope]*ProjectUse{},
		blocks:     map[Scope]map[string]bool{},
		colls:      map[Scope]map[string]bool{},
		terms:      map[Scope]map[string]*TermUse{},
		termBlocks: map[Scope]map[string]map[string]bool{},
	}
}

func (a *projectUseAggregator) add(block NodeID, e *graph.Edge) {
	project := Scope{Workspace: block.Scope.Workspace, Project: block.Scope.Project}
	row, ok := a.byScope[project]
	if !ok {
		row = &ProjectUse{Scope: project}
		a.byScope[project] = row
		a.order = append(a.order, project)
		a.blocks[project] = map[string]bool{}
		a.colls[project] = map[string]bool{}
		a.terms[project] = map[string]*TermUse{}
		a.termBlocks[project] = map[string]map[string]bool{}
	}

	count := edgeCount(e)
	row.Occurrences += count
	if !a.blocks[project][block.Local] {
		a.blocks[project][block.Local] = true
		row.Blocks++
	}

	collection := e.Properties[PropCollection]
	if collection != "" && !a.colls[project][collection] {
		a.colls[project][collection] = true
		row.Collections = append(row.Collections, collection)
	}

	term := e.Properties[PropTerm]
	status := e.Properties[PropTermStatus]
	discouraged := model.TermStatus(status).Discouraged()
	if term != "" {
		key := term + "\x00" + e.Properties[PropTermLocale]
		tu, seen := a.terms[project][key]
		if !seen {
			tu = &TermUse{
				Term:        term,
				TermLocale:  e.Properties[PropTermLocale],
				Status:      status,
				Discouraged: discouraged,
			}
			a.terms[project][key] = tu
			a.termBlocks[project][key] = map[string]bool{}
		}
		tu.Occurrences += count
		if !a.termBlocks[project][key][block.Local] {
			a.termBlocks[project][key][block.Local] = true
			tu.Blocks++
		}
	}

	row.Uses = append(row.Uses, ConceptUse{
		Scope:       block.Scope,
		ContentKey:  block.Local,
		Collection:  collection,
		Document:    e.Properties[PropDocument],
		BlockID:     e.Properties[PropBlockID],
		Locale:      e.Properties[PropLocale],
		Term:        term,
		TermLocale:  e.Properties[PropTermLocale],
		Status:      status,
		Discouraged: discouraged,
		Occurrences: count,
	})
}

// rollup renders the aggregate in a stable order: an answer a reader compares
// against yesterday's has to come back the same way twice.
func (a *projectUseAggregator) rollup() []ProjectUse {
	out := make([]ProjectUse, 0, len(a.order))
	for _, project := range a.order {
		row := a.byScope[project]
		row.Terms = make([]TermUse, 0, len(a.terms[project]))
		for _, tu := range a.terms[project] {
			row.Terms = append(row.Terms, *tu)
		}
		sort.Slice(row.Terms, func(i, j int) bool {
			if row.Terms[i].Term != row.Terms[j].Term {
				return row.Terms[i].Term < row.Terms[j].Term
			}
			return row.Terms[i].TermLocale < row.Terms[j].TermLocale
		})
		sort.Strings(row.Collections)
		sort.Slice(row.Uses, func(i, j int) bool { return lessUse(row.Uses[i], row.Uses[j]) })
		row.Status, row.Discouraged = standing(row.Terms)
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope.Key() < out[j].Scope.Key() })
	return out
}

// standing reduces the terms one project used to how the concept stands there.
//
// A discouraged spelling decides, because that is the finding a writer has to
// act on: a project using both "sign in" and "log in" is a project still saying
// "log in". Where nothing is discouraged and every spelling agrees, that shared
// status is the standing; where they disagree, the project has no single
// standing and says so by leaving it empty rather than picking one.
func standing(uses []TermUse) (status string, discouraged bool) {
	for _, u := range uses {
		if u.Discouraged {
			return u.Status, true
		}
	}
	for _, u := range uses {
		if u.Status == "" {
			continue
		}
		if status == "" {
			status = u.Status
			continue
		}
		if status != u.Status {
			return "", false
		}
	}
	return status, false
}

func lessUse(a, b ConceptUse) bool {
	if a.Document != b.Document {
		return a.Document < b.Document
	}
	if a.BlockID != b.BlockID {
		return a.BlockID < b.BlockID
	}
	if a.ContentKey != b.ContentKey {
		return a.ContentKey < b.ContentKey
	}
	if a.Locale != b.Locale {
		return a.Locale < b.Locale
	}
	return a.Term < b.Term
}

// edgeCount reads an edge's occurrence count. An edge exists because there was
// a use, so an absent or unreadable count is one rather than none.
func edgeCount(e *graph.Edge) int {
	n, err := strconv.Atoi(e.Properties[PropCount])
	if err != nil || n < 1 {
		return 1
	}
	return n
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
	// ContentHash is the basis the decision blessed: the source hash. A blessing
	// whose basis differs from the block's current source is stale, whatever its
	// status says.
	ContentHash string `json:"content_hash,omitempty"`
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
			Scope:       parsed.Scope,
			ContentKey:  contentKey,
			Unit:        e.Properties[PropUnit],
			Variant:     e.Properties[PropVariant],
			Status:      e.Properties[PropStatus],
			TargetHash:  e.Properties[PropTargetHash],
			ContentHash: e.Properties[PropContentHash],
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
