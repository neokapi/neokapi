package terms

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
)

// DefaultMaxConcepts is the default maximum number of concepts in an InMemoryStore.
// A value of 0 means unlimited.
const DefaultMaxConcepts = 0

// InMemoryStore is a thread-safe, in-memory implementation of Terminology
// with normalized and fuzzy term matching.
type InMemoryStore struct {
	mu          sync.RWMutex
	concepts    []Concept
	byID        map[string]int // concept ID → index
	relations   []ConceptRelation
	relByID     map[string]int // relation ID → index
	maxConcepts int            // 0 = unlimited
}

// InMemoryStoreOption configures an InMemoryStore instance.
type InMemoryStoreOption func(*InMemoryStore)

// WithMaxConcepts sets the maximum number of concepts. When the limit is reached,
// the oldest concept is evicted to make room. A value of 0 means unlimited.
func WithMaxConcepts(max int) InMemoryStoreOption {
	return func(tb *InMemoryStore) {
		tb.maxConcepts = max
	}
}

// NewInMemoryStore creates a new empty in-memory terms.
func NewInMemoryStore(opts ...InMemoryStoreOption) *InMemoryStore {
	tb := &InMemoryStore{
		byID:        make(map[string]int),
		relByID:     make(map[string]int),
		maxConcepts: DefaultMaxConcepts,
	}
	for _, opt := range opts {
		opt(tb)
	}
	return tb
}

// AddConcept inserts or updates a concept.
func (tb *InMemoryStore) AddConcept(_ context.Context, concept Concept) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if concept.ID == "" {
		return errors.New("concept ID is required")
	}
	if err := validateConceptTermStatuses(concept); err != nil {
		return err
	}
	concept = NormalizedConcept(concept)

	// As in the SQLite store: an unstamped concept the store already holds word
	// for word is not a change.
	if idx, exists := tb.byID[concept.ID]; exists && concept.UpdatedAt.IsZero() && SameConcept(tb.concepts[idx], concept) {
		return nil
	}

	now := time.Now()
	if concept.CreatedAt.IsZero() {
		concept.CreatedAt = now
	}
	if concept.UpdatedAt.IsZero() {
		concept.UpdatedAt = now
	}

	if idx, exists := tb.byID[concept.ID]; exists {
		concept.CreatedAt = tb.concepts[idx].CreatedAt // preserve original creation time
		tb.concepts[idx] = cloneConcept(concept)
		return nil
	}

	// Evict the oldest concept if we've reached the size limit.
	if tb.maxConcepts > 0 && len(tb.concepts) >= tb.maxConcepts {
		tb.deleteConceptAt(0)
	}

	tb.byID[concept.ID] = len(tb.concepts)
	tb.concepts = append(tb.concepts, cloneConcept(concept))
	return nil
}

func (tb *InMemoryStore) deleteConceptAt(index int) {
	id := tb.concepts[index].ID
	delete(tb.byID, id)
	tb.concepts = slices.Delete(tb.concepts, index, index+1)
	for i := index; i < len(tb.concepts); i++ {
		tb.byID[tb.concepts[i].ID] = i
	}
	tb.deleteRelationsOf(id)
}

// deleteRelationsOf removes all relations touching the concept, mirroring the
// SQLite backend's ON DELETE CASCADE. Caller must hold tb.mu.
func (tb *InMemoryStore) deleteRelationsOf(conceptID string) {
	kept := tb.relations[:0]
	for _, rel := range tb.relations {
		if rel.SourceID == conceptID || rel.TargetID == conceptID {
			delete(tb.relByID, rel.ID)
			continue
		}
		kept = append(kept, rel)
	}
	clear(tb.relations[len(kept):])
	tb.relations = kept
	for i, rel := range tb.relations {
		tb.relByID[rel.ID] = i
	}
}

// MaxConcepts returns the configured maximum concept count (0 = unlimited).
func (tb *InMemoryStore) MaxConcepts() int {
	return tb.maxConcepts
}

// GetConcept retrieves a concept by ID.
func (tb *InMemoryStore) GetConcept(_ context.Context, id string) (Concept, bool, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	idx, exists := tb.byID[id]
	if !exists {
		return Concept{}, false, nil
	}
	return cloneConcept(tb.concepts[idx]), true, nil
}

// DeleteConcept removes a concept by ID.
func (tb *InMemoryStore) DeleteConcept(_ context.Context, id string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	idx, exists := tb.byID[id]
	if !exists {
		return fmt.Errorf("concept not found: %s", id)
	}

	tb.deleteConceptAt(idx)
	return nil
}

// AddRelation inserts or updates (by ID) a relation between two concepts.
func (tb *InMemoryStore) AddRelation(_ context.Context, rel ConceptRelation) error {
	if err := ValidateRelation(rel); err != nil {
		return err
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	if _, ok := tb.byID[rel.SourceID]; !ok {
		return fmt.Errorf("source concept not found: %s", rel.SourceID)
	}
	if _, ok := tb.byID[rel.TargetID]; !ok {
		return fmt.Errorf("target concept not found: %s", rel.TargetID)
	}

	if rel.CreatedAt.IsZero() {
		rel.CreatedAt = time.Now()
	}

	rel.Validity = cloneValidity(rel.Validity)
	if idx, exists := tb.relByID[rel.ID]; exists {
		rel.CreatedAt = tb.relations[idx].CreatedAt // preserve original creation time
		tb.relations[idx] = rel
		return nil
	}

	tb.relByID[rel.ID] = len(tb.relations)
	tb.relations = append(tb.relations, rel)
	return nil
}

// DeleteRelation removes a relation by ID.
func (tb *InMemoryStore) DeleteRelation(_ context.Context, id string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	idx, exists := tb.relByID[id]
	if !exists {
		return fmt.Errorf("relation not found: %s", id)
	}

	lastIdx := len(tb.relations) - 1
	if idx != lastIdx {
		tb.relations[idx] = tb.relations[lastIdx]
		tb.relByID[tb.relations[idx].ID] = idx
	}
	tb.relations = slices.Delete(tb.relations, lastIdx, lastIdx+1)
	delete(tb.relByID, id)

	return nil
}

// RelationsOf returns all relations touching the concept, in either direction,
// filtered by the validity scope when one is given.
func (tb *InMemoryStore) RelationsOf(_ context.Context, conceptID string, scope *graph.Scope) ([]ConceptRelation, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var out []ConceptRelation
	for _, rel := range tb.relations {
		if rel.SourceID != conceptID && rel.TargetID != conceptID {
			continue
		}
		if !MatchesScope(rel.Validity, scope) {
			continue
		}
		rel.Validity = cloneValidity(rel.Validity)
		out = append(out, rel)
	}
	sortRelations(out)
	return out, nil
}

// ListRelations returns all relations, filtered by the validity scope when one
// is given.
func (tb *InMemoryStore) ListRelations(_ context.Context, scope *graph.Scope) ([]ConceptRelation, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var out []ConceptRelation
	for _, rel := range tb.relations {
		if !MatchesScope(rel.Validity, scope) {
			continue
		}
		rel.Validity = cloneValidity(rel.Validity)
		out = append(out, rel)
	}
	sortRelations(out)
	return out, nil
}

// sortRelations orders relations deterministically by ID.
func sortRelations(rels []ConceptRelation) {
	slices.SortFunc(rels, func(a, b ConceptRelation) int {
		return cmp.Compare(a.ID, b.ID)
	})
}

// Lookup finds terms matching the source text.
func (tb *InMemoryStore) Lookup(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	matches, err := lookupTiers(ctx, sourceText, opts, func(mode model.MatchStrategy, key string, opts LookupOptions) ([]TermCandidate, error) {
		var rows []TermCandidate
		for c, t := range tb.lookupTerms(opts) {
			switch mode {
			case model.MatchStrategyExact:
				if !matchesTerm(key, t.Text, opts.CaseSensitive) && !declaresFormOf(t, key, opts.CaseSensitive) {
					continue
				}
			case model.MatchStrategyNormalized:
				if NormalizeTerm(t.Text) != key {
					continue
				}
			}
			rows = append(rows, TermCandidate{ConceptID: c.ID, Term: t})
		}
		return rows, nil
	}, func(_ context.Context, id string) (Concept, error) {
		return tb.concepts[tb.byID[id]], nil
	})
	return cloneMatches(matches), err
}

// LookupAll finds occurrences of the eligible terms in the source text.
func (tb *InMemoryStore) LookupAll(_ context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	opts = ApplyLookupDefaults(opts)
	var candidates []LocaleTerm
	for c, t := range tb.lookupTerms(opts) {
		candidates = append(candidates, LocaleTerm{Concept: c, Term: t})
	}
	return cloneMatches(LookupAllTiered(sourceText, opts, candidates)), nil
}

func (tb *InMemoryStore) lookupTerms(opts LookupOptions) iter.Seq2[Concept, Term] {
	return func(yield func(Concept, Term) bool) {
		for _, c := range tb.concepts {
			if !opts.includesConcept(c.ID) || !matchesDomain(c, opts.Domains) || !matchesSource(c, opts.SourceFilter) {
				continue
			}
			if opts.ProjectScope == ProjectScopeOnly && c.ProjectID != opts.ProjectID ||
				opts.ProjectScope == ProjectScopeExclude && c.ProjectID == opts.ProjectID {
				continue
			}
			for _, t := range c.Terms {
				if t.Locale == opts.SourceLocale && MatchesStatus(t.Status, opts.StatusFilter) && !yield(c, t) {
					return
				}
			}
		}
	}
}

func cloneMatches(matches []TermMatch) []TermMatch {
	for i := range matches {
		matches[i].Concept = cloneConcept(matches[i].Concept)
		matches[i].Term = cloneTerm(matches[i].Term)
	}
	return matches
}

func cloneConcept(c Concept) Concept {
	c.Properties = maps.Clone(c.Properties)
	c.Terms = slices.Clone(c.Terms)
	for i, t := range c.Terms {
		c.Terms[i] = cloneTerm(t)
	}
	return c
}

func cloneTerm(t Term) Term {
	t.Forms = slices.Clone(t.Forms)
	t.Validity = cloneValidity(t.Validity)
	return t
}

func cloneValidity(v *graph.Validity) *graph.Validity {
	if v == nil {
		return nil
	}
	copy := *v
	copy.Tags = maps.Clone(v.Tags)
	if v.ValidFrom != nil {
		copy.ValidFrom = new(*v.ValidFrom)
	}
	if v.ValidTo != nil {
		copy.ValidTo = new(*v.ValidTo)
	}
	return &copy
}

// Search performs a case-insensitive text search across terms and definitions.
func (tb *InMemoryStore) Search(_ context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	sourceLocale, targetLocale = model.NormalizeLocale(sourceLocale), model.NormalizeLocale(targetLocale)
	lowerQuery := strings.ToLower(query)
	var matched []Concept

	for _, concept := range tb.concepts {
		if matchesConcept(concept, lowerQuery, sourceLocale, targetLocale) {
			matched = append(matched, concept)
		}
	}

	total := len(matched)
	if offset >= total {
		return nil, total, nil
	}
	page := matched[max(offset, 0):]
	if limit >= 0 {
		page = page[:min(limit, len(page))]
	}
	for i := range page {
		page[i] = cloneConcept(page[i])
	}
	return page, total, nil
}

// Count returns the total number of concepts.
func (tb *InMemoryStore) Count(_ context.Context) (int, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.concepts), nil
}

// Concepts returns a copy of all concepts.
func (tb *InMemoryStore) Concepts(_ context.Context) ([]Concept, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	out := make([]Concept, len(tb.concepts))
	for i, concept := range tb.concepts {
		out[i] = cloneConcept(concept)
	}
	return out, nil
}

// Close releases resources. For InMemoryStore, this is a no-op.
func (tb *InMemoryStore) Close() error {
	return nil
}

func matchesConcept(c Concept, lowerQuery string, sourceLocale, targetLocale model.LocaleID) bool {
	if lowerQuery == "" && sourceLocale == "" && targetLocale == "" {
		return true
	}

	// Check locale filters.
	if sourceLocale != "" || targetLocale != "" {
		hasSource, hasTarget := false, false
		for _, t := range c.Terms {
			if sourceLocale != "" && t.Locale == sourceLocale {
				hasSource = true
			}
			if targetLocale != "" && t.Locale == targetLocale {
				hasTarget = true
			}
		}
		if sourceLocale != "" && !hasSource {
			return false
		}
		if targetLocale != "" && !hasTarget {
			return false
		}
	}

	if lowerQuery == "" {
		return true
	}

	// Check definition.
	if strings.Contains(strings.ToLower(c.Definition), lowerQuery) {
		return true
	}
	// Check domain.
	if strings.Contains(strings.ToLower(c.Domain), lowerQuery) {
		return true
	}
	// Check term texts.
	for _, t := range c.Terms {
		if strings.Contains(strings.ToLower(t.Text), lowerQuery) {
			return true
		}
	}
	return false
}
