package knowledge

import (
	"context"
	"fmt"
	"strings"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// ---------------------------------------------------------------------------
// Engine dependencies (small interfaces the engine defines for itself)
//
// Following "accept interfaces, return structs", the blast-radius engine depends
// only on the narrow slices of the platform stores it actually calls. The real
// PostgreSQL ContentStore, the framework termbase, and the brand store satisfy
// these directly (or via thin adapters); tests inject in-memory fakes and the
// framework in-memory termbase so the whole read side runs without a database.
// ---------------------------------------------------------------------------

// BlockSource is the slice of the content store the engine walks to gather the
// stored blocks a change-set or concept is evaluated against. The real bowrain
// ContentStore satisfies it directly.
type BlockSource interface {
	// ListProjects returns every project; the engine filters to the workspace.
	ListProjects(ctx context.Context) ([]*store.Project, error)
	// ListStreams returns a project's streams, used to confirm a pilot stream
	// exists before querying it (the walk always includes "main").
	ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*store.Stream, error)
	// GetBlocks returns the blocks matching the query (project + stream scoped).
	GetBlocks(ctx context.Context, query store.BlockQuery) ([]*store.StoredBlock, error)
}

// CollectionResolver is an optional capability of a BlockSource: it maps a
// stored block's item to its collection so the blast radius can group by
// collection. When a BlockSource does not implement it (or a lookup fails), the
// engine falls back to grouping blocks by their item name. The real
// ContentStore implements it.
type CollectionResolver interface {
	GetItem(ctx context.Context, projectID, stream, itemName string) (*store.Item, error)
	GetCollection(ctx context.Context, projectID, collectionID string) (*store.Collection, error)
}

// ConceptStore is the slice of the framework termbase the engine reads to build
// the before/after candidate termbases. The framework termbase (in-memory and
// SQLite TBStore) satisfies it.
type ConceptStore interface {
	// GetConcept returns a concept by ID; ok is false when it is absent.
	GetConcept(ctx context.Context, id string) (termbase.Concept, bool, error)
	// RelationsOf returns every relation touching the concept (either
	// direction); a nil scope means no validity filtering.
	RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]termbase.ConceptRelation, error)
}

// ProfileStore is the slice of the brand store the engine reads (and the merge
// path writes) for voice impact. corebrand.BrandStore satisfies it.
type ProfileStore interface {
	// GetProfile returns a voice profile by ID, or a nil profile with a nil
	// error when it is absent (following the brand store's convention).
	GetProfile(ctx context.Context, id string) (*corebrand.VoiceProfile, error)
	// ListProfiles returns the workspace's voice profiles.
	ListProfiles(ctx context.Context, workspaceID string) ([]*corebrand.VoiceProfile, error)
	// UpdateProfile persists a profile (used by the merge path; the read side
	// never calls it).
	UpdateProfile(ctx context.Context, profile *corebrand.VoiceProfile) error
}

// Compile-time proof that the production stores satisfy the engine's interfaces,
// so the read side runs against the real platform without adapters: the bowrain
// ContentStore is a BlockSource and a CollectionResolver, the framework termbase
// store is a ConceptStore, and the brand store is a ProfileStore.
var (
	_ BlockSource        = (store.ContentStore)(nil)
	_ CollectionResolver = (store.ContentStore)(nil)
	_ ConceptStore       = (termbase.TBStore)(nil)
	_ ProfileStore       = (corebrand.BrandStore)(nil)
)

// Engine computes the read-side analytics of the brand knowledge graph: the
// blast radius of a change-set over stored content (EvaluateChangeSet) and the
// where-used footprint of a concept (ConceptUsage). It holds no state of its
// own beyond its store dependencies, so a single engine is safe to reuse across
// requests. The governance Store is retained for the mutation paths (merge,
// pilots) that build on the same engine; the read methods do not require it and
// tolerate a nil store.
type Engine struct {
	blocks   BlockSource
	concepts ConceptStore
	profiles ProfileStore
	store    Store
}

// NewEngine constructs an Engine over the given store slices. store (the
// governance Store) may be nil for read-only use.
func NewEngine(blocks BlockSource, concepts ConceptStore, profiles ProfileStore, store Store) *Engine {
	return &Engine{blocks: blocks, concepts: concepts, profiles: profiles, store: store}
}

// ---------------------------------------------------------------------------
// Candidate builders
//
// A blast radius compares the live ("before") graph and voice profile against
// the draft ("after") that the change-set's ops would produce. The builders
// below produce those "after" states purely in memory, without persisting
// anything — exactly the property the preview needs (AD-021: "nothing is
// persisted by the preview").
// ---------------------------------------------------------------------------

// ApplyOpsToTermbase returns a new in-memory termbase equal to base with the
// concept, term, and relation ops in ops applied — the "after" snapshot a
// change-set would produce. base is treated as read-only: it is deep-copied
// first, so the returned termbase is fully independent and base is never
// mutated. Voice ops are ignored (they apply to a profile, not the termbase);
// callers route them through ApplyVoiceOpsToProfile. Nothing is persisted.
//
// term.* ops set/replace/remove a term on its concept; term.status sets the
// term's status (and optional validity); relation.add/remove adjust relations;
// concept.create/update/delete adjust concepts. An op that references a concept
// absent from base is an error for the mutating term ops (the concept must
// exist to be edited); concept.create inserts a new concept regardless.
func ApplyOpsToTermbase(ctx context.Context, base *termbase.InMemoryTermBase, ops []ChangeSetOp) (*termbase.InMemoryTermBase, error) {
	after, err := cloneInMemoryTermbase(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("clone base termbase: %w", err)
	}
	for _, op := range ops {
		if err := applyTermbaseOp(ctx, after, op); err != nil {
			return nil, err
		}
	}
	return after, nil
}

// applyTermbaseOp applies a single concept/term/relation op to tb. Voice ops and
// op types that do not touch the termbase are no-ops. tb is the minimal
// TermBase write surface (GetConcept/AddConcept/DeleteConcept/AddRelation/
// DeleteRelation), so the same op-application logic drives both the in-memory
// candidate snapshot (ApplyOpsToTermbase) and the live workspace termbase at
// merge time (MergeChangeSet).
func applyTermbaseOp(ctx context.Context, tb termbase.TermBase, op ChangeSetOp) error {
	switch op.Op {
	case OpConceptCreate:
		var p ConceptCreatePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		return tb.AddConcept(ctx, p.Concept)

	case OpConceptUpdate:
		var p ConceptUpdatePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		c, ok, err := tb.GetConcept(ctx, p.ConceptID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s: concept %q not found", op.Op, p.ConceptID)
		}
		if p.Domain != nil {
			c.Domain = *p.Domain
		}
		if p.Definition != nil {
			c.Definition = *p.Definition
		}
		if p.Properties != nil {
			props := make(map[string]string, len(p.Properties))
			for k, v := range p.Properties {
				props[k] = v
			}
			c.Properties = props
		}
		return tb.AddConcept(ctx, c)

	case OpConceptDelete:
		var p ConceptDeletePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if err := tb.DeleteConcept(ctx, p.ConceptID); err != nil && !isNotFound(err) {
			return fmt.Errorf("%s: %w", op.Op, err)
		}
		return nil

	case OpTermAdd:
		var p TermAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		c, err := getConceptForEdit(ctx, tb, op.Op, p.ConceptID)
		if err != nil {
			return err
		}
		c.Terms = append(copyTerms(c.Terms), p.Term)
		return tb.AddConcept(ctx, c)

	case OpTermUpdate:
		var p TermUpdatePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		c, err := getConceptForEdit(ctx, tb, op.Op, p.ConceptID)
		if err != nil {
			return err
		}
		terms := copyTerms(c.Terms)
		idx := findTerm(terms, p.Locale, p.Text)
		if idx < 0 {
			return fmt.Errorf("%s: term %q (%s) not found on concept %q", op.Op, p.Text, p.Locale, p.ConceptID)
		}
		terms[idx] = p.Term
		c.Terms = terms
		return tb.AddConcept(ctx, c)

	case OpTermRemove:
		var p TermRemovePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		c, err := getConceptForEdit(ctx, tb, op.Op, p.ConceptID)
		if err != nil {
			return err
		}
		terms := copyTerms(c.Terms)
		idx := findTerm(terms, p.Locale, p.Text)
		if idx < 0 {
			return fmt.Errorf("%s: term %q (%s) not found on concept %q", op.Op, p.Text, p.Locale, p.ConceptID)
		}
		c.Terms = append(terms[:idx], terms[idx+1:]...)
		return tb.AddConcept(ctx, c)

	case OpTermStatus:
		var p TermStatusPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		c, err := getConceptForEdit(ctx, tb, op.Op, p.ConceptID)
		if err != nil {
			return err
		}
		terms := copyTerms(c.Terms)
		idx := findTerm(terms, p.Locale, p.Text)
		if idx < 0 {
			return fmt.Errorf("%s: term %q (%s) not found on concept %q", op.Op, p.Text, p.Locale, p.ConceptID)
		}
		terms[idx].Status = p.To
		if p.Validity != nil {
			terms[idx].Validity = p.Validity
		}
		c.Terms = terms
		return tb.AddConcept(ctx, c)

	case OpRelationAdd:
		var p RelationAddPayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if err := tb.AddRelation(ctx, p.Relation); err != nil {
			return fmt.Errorf("%s: %w", op.Op, err)
		}
		return nil

	case OpRelationRemove:
		var p RelationRemovePayload
		if err := decodePayload(op, &p); err != nil {
			return err
		}
		if err := tb.DeleteRelation(ctx, p.RelationID); err != nil && !isNotFound(err) {
			return fmt.Errorf("%s: %w", op.Op, err)
		}
		return nil

	case OpVoiceRuleAdd, OpVoiceRuleRemove:
		// Voice ops apply to a profile, not the termbase.
		return nil

	default:
		return fmt.Errorf("unknown op type: %q", op.Op)
	}
}

// getConceptForEdit loads a concept that an op edits, returning a descriptive
// error when it is absent.
func getConceptForEdit(ctx context.Context, tb termbase.TermBase, op OpType, conceptID string) (termbase.Concept, error) {
	c, ok, err := tb.GetConcept(ctx, conceptID)
	if err != nil {
		return termbase.Concept{}, err
	}
	if !ok {
		return termbase.Concept{}, fmt.Errorf("%s: concept %q not found", op, conceptID)
	}
	return c, nil
}

// findTerm returns the index of the term identified by (locale, text) in terms,
// matched case-insensitively on text, or -1 when absent.
func findTerm(terms []termbase.Term, locale model.LocaleID, text string) int {
	for i := range terms {
		if terms[i].Locale == locale && strings.EqualFold(terms[i].Text, text) {
			return i
		}
	}
	return -1
}

// copyTerms returns an independent copy of a term slice so an "after" termbase
// can be mutated without touching the "before" snapshot's backing array.
func copyTerms(terms []termbase.Term) []termbase.Term {
	return append([]termbase.Term(nil), terms...)
}

// isNotFound reports whether err looks like a "not found" error from the
// in-memory termbase (used to make delete ops idempotent against a pruned
// candidate snapshot).
func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// ApplyVoiceOpsToProfile returns a candidate copy of baseline with the
// voice.rule.add / voice.rule.remove ops that target it applied — the "after"
// profile a change-set's voice ops would produce. baseline is deep-copied
// (corebrand.VoiceProfile.Clone), so it is never mutated. Ops whose ProfileID
// does not match baseline.ID are ignored, so a caller may pass the full op list
// and get back only the changes relevant to this profile. A nil baseline yields
// nil.
//
// An add joins the rule to the named vocabulary list (preferred|forbidden|
// competitor), idempotently by term (case-insensitive) like
// corebrand.ApplySuggestedRule; a remove drops the rule from that list by term.
func ApplyVoiceOpsToProfile(baseline *corebrand.VoiceProfile, ops []ChangeSetOp) *corebrand.VoiceProfile {
	if baseline == nil {
		return nil
	}
	cand := baseline.Clone()
	for _, op := range ops {
		switch op.Op {
		case OpVoiceRuleAdd:
			var p VoiceRuleAddPayload
			if decodePayload(op, &p) != nil || p.ProfileID != baseline.ID {
				continue
			}
			addRuleToList(cand, p.List, p.Rule)
		case OpVoiceRuleRemove:
			var p VoiceRuleRemovePayload
			if decodePayload(op, &p) != nil || p.ProfileID != baseline.ID {
				continue
			}
			removeRuleFromList(cand, p.List, p.Term)
		}
	}
	return cand
}

// listFor returns a pointer to the vocabulary list a voice rule belongs to.
func listFor(p *corebrand.VoiceProfile, list VoiceRuleList) *[]corebrand.TermRule {
	switch list {
	case VoiceListPreferred:
		return &p.Vocabulary.PreferredTerms
	case VoiceListForbidden:
		return &p.Vocabulary.ForbiddenTerms
	case VoiceListCompetitor:
		return &p.Vocabulary.CompetitorTerms
	default:
		return nil
	}
}

// addRuleToList appends rule to the named list, idempotently by term: an
// existing rule with the same term (case-insensitive) is updated in place rather
// than duplicated.
func addRuleToList(p *corebrand.VoiceProfile, list VoiceRuleList, rule corebrand.TermRule) {
	lp := listFor(p, list)
	if lp == nil || strings.TrimSpace(rule.Term) == "" {
		return
	}
	for i := range *lp {
		if strings.EqualFold((*lp)[i].Term, rule.Term) {
			(*lp)[i] = rule
			return
		}
	}
	*lp = append(*lp, rule)
}

// removeRuleFromList drops the rule identified by term (case-insensitive) from
// the named list.
func removeRuleFromList(p *corebrand.VoiceProfile, list VoiceRuleList, term string) {
	lp := listFor(p, list)
	if lp == nil || strings.TrimSpace(term) == "" {
		return
	}
	kept := (*lp)[:0]
	for _, r := range *lp {
		if strings.EqualFold(r.Term, term) {
			continue
		}
		kept = append(kept, r)
	}
	*lp = kept
}

// ---------------------------------------------------------------------------
// Termbase snapshot helpers
// ---------------------------------------------------------------------------

// cloneInMemoryTermbase returns a deep, independent copy of src: every concept's
// terms and properties are copied so the clone can be mutated without affecting
// src, and every relation is carried over.
func cloneInMemoryTermbase(ctx context.Context, src *termbase.InMemoryTermBase) (*termbase.InMemoryTermBase, error) {
	dst := termbase.NewInMemoryTermBase()
	if src == nil {
		return dst, nil
	}
	concepts, err := src.Concepts(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range concepts {
		if err := dst.AddConcept(ctx, deepCopyConcept(c)); err != nil {
			return nil, err
		}
	}
	rels, err := src.ListRelations(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, r := range rels {
		if err := dst.AddRelation(ctx, r); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

// deepCopyConcept copies a concept's mutable collection fields (terms,
// properties) so it can be edited independently of any shared backing array.
func deepCopyConcept(c termbase.Concept) termbase.Concept {
	c.Terms = copyTerms(c.Terms)
	if c.Properties != nil {
		props := make(map[string]string, len(c.Properties))
		for k, v := range c.Properties {
			props[k] = v
		}
		c.Properties = props
	}
	return c
}

// buildBeforeTermbase seeds an in-memory termbase with the current state of
// every concept an op touches (and the endpoints of every relation those
// concepts carry), so a before/after diff can be computed without pulling the
// whole workspace graph into memory. Concepts absent from the store are simply
// not seeded (a concept.create op introduces them only on the "after" side).
func (e *Engine) buildBeforeTermbase(ctx context.Context, ops []ChangeSetOp) (*termbase.InMemoryTermBase, error) {
	tb := termbase.NewInMemoryTermBase()
	if e.concepts == nil {
		return tb, nil
	}
	seeded := map[string]bool{}
	for _, id := range touchedConceptIDs(ops) {
		if err := e.seedConcept(ctx, tb, id, seeded); err != nil {
			return nil, err
		}
	}
	// Seed the relations of each seeded concept, pulling in any endpoint
	// concepts they reference so USE_INSTEAD/REPLACED_BY guidance resolves.
	for id := range seeded {
		rels, err := e.concepts.RelationsOf(ctx, id, nil)
		if err != nil {
			return nil, err
		}
		for _, r := range rels {
			if err := e.seedConcept(ctx, tb, r.SourceID, seeded); err != nil {
				return nil, err
			}
			if err := e.seedConcept(ctx, tb, r.TargetID, seeded); err != nil {
				return nil, err
			}
			if err := tb.AddRelation(ctx, r); err != nil {
				return nil, err
			}
		}
	}
	return tb, nil
}

// seedConcept loads a concept by ID into tb once. A missing or empty ID, or a
// concept absent from the store, leaves tb unchanged (but still marks the ID
// seeded so it is not queried again).
func (e *Engine) seedConcept(ctx context.Context, tb *termbase.InMemoryTermBase, id string, seeded map[string]bool) error {
	if id == "" || seeded[id] {
		return nil
	}
	seeded[id] = true
	c, ok, err := e.concepts.GetConcept(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return tb.AddConcept(ctx, deepCopyConcept(c))
}

// touchedConceptIDs returns the concept IDs an op list references: the concept
// of every concept/term/status op, plus both endpoints of every relation.add.
// relation.remove is not included (it names only a relation ID); such relations
// are pulled in via the seeded concepts' RelationsOf.
func touchedConceptIDs(ops []ChangeSetOp) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, op := range ops {
		add(conceptIDOf(op))
		if op.Op == OpRelationAdd {
			var p RelationAddPayload
			if decodePayload(op, &p) == nil {
				add(p.Relation.SourceID)
				add(p.Relation.TargetID)
			}
		}
	}
	return ids
}
