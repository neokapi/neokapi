package termbase

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// DefaultMaxConcepts is the default maximum number of concepts in an InMemoryTermBase.
// A value of 0 means unlimited.
const DefaultMaxConcepts = 0

// InMemoryTermBase is a thread-safe, in-memory implementation of TermBase
// with normalized and fuzzy term matching.
type InMemoryTermBase struct {
	mu          sync.RWMutex
	concepts    []Concept
	byID        map[string]int // concept ID → index
	maxConcepts int            // 0 = unlimited
}

// InMemoryTermBaseOption configures an InMemoryTermBase instance.
type InMemoryTermBaseOption func(*InMemoryTermBase)

// WithMaxConcepts sets the maximum number of concepts. When the limit is reached,
// the oldest concept is evicted to make room. A value of 0 means unlimited.
func WithMaxConcepts(max int) InMemoryTermBaseOption {
	return func(tb *InMemoryTermBase) {
		tb.maxConcepts = max
	}
}

// NewInMemoryTermBase creates a new empty in-memory termbase.
func NewInMemoryTermBase(opts ...InMemoryTermBaseOption) *InMemoryTermBase {
	tb := &InMemoryTermBase{
		byID:        make(map[string]int),
		maxConcepts: DefaultMaxConcepts,
	}
	for _, opt := range opts {
		opt(tb)
	}
	return tb
}

// AddConcept inserts or updates a concept.
func (tb *InMemoryTermBase) AddConcept(concept Concept) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if concept.ID == "" {
		return errors.New("concept ID is required")
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
		tb.concepts[idx] = concept
		return nil
	}

	// Evict the oldest concept if we've reached the size limit.
	if tb.maxConcepts > 0 && len(tb.concepts) >= tb.maxConcepts {
		tb.evictOldest()
	}

	tb.byID[concept.ID] = len(tb.concepts)
	tb.concepts = append(tb.concepts, concept)
	return nil
}

// evictOldest removes the first (oldest) concept. Caller must hold tb.mu.
func (tb *InMemoryTermBase) evictOldest() {
	if len(tb.concepts) == 0 {
		return
	}
	oldest := tb.concepts[0]
	delete(tb.byID, oldest.ID)

	// Shift concepts and update index map.
	copy(tb.concepts, tb.concepts[1:])
	tb.concepts = tb.concepts[:len(tb.concepts)-1]
	for id, idx := range tb.byID {
		tb.byID[id] = idx - 1
	}
}

// MaxConcepts returns the configured maximum concept count (0 = unlimited).
func (tb *InMemoryTermBase) MaxConcepts() int {
	return tb.maxConcepts
}

// GetConcept retrieves a concept by ID.
func (tb *InMemoryTermBase) GetConcept(id string) (Concept, bool) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	idx, exists := tb.byID[id]
	if !exists {
		return Concept{}, false
	}
	return tb.concepts[idx], true
}

// DeleteConcept removes a concept by ID.
func (tb *InMemoryTermBase) DeleteConcept(id string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	idx, exists := tb.byID[id]
	if !exists {
		return fmt.Errorf("concept not found: %s", id)
	}

	lastIdx := len(tb.concepts) - 1
	if idx != lastIdx {
		tb.concepts[idx] = tb.concepts[lastIdx]
		tb.byID[tb.concepts[idx].ID] = idx
	}
	tb.concepts = tb.concepts[:lastIdx]
	delete(tb.byID, id)

	return nil
}

// Lookup finds terms matching the source text.
func (tb *InMemoryTermBase) Lookup(sourceText string, opts LookupOptions) []TermMatch {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if sourceText == "" {
		return nil
	}

	opts = ApplyLookupDefaults(opts)
	modeEnabled := MatchModesEnabled(opts.MatchModes)
	normalizedSource := NormalizeTerm(sourceText)
	var matches []TermMatch

	for _, concept := range tb.concepts {
		if !matchesDomain(concept, opts.Domains) {
			continue
		}
		if !matchesSource(concept, opts.SourceFilter) {
			continue
		}

		for _, term := range concept.Terms {
			if term.Locale != opts.SourceLocale {
				continue
			}
			if !MatchesStatus(term.Status, opts.StatusFilter) {
				continue
			}

			// Try exact match.
			if modeEnabled[model.MatchStrategyExact] {
				if matchesTerm(sourceText, term.Text, opts.CaseSensitive) {
					matches = append(matches, TermMatch{
						Concept:   concept,
						Term:      term,
						Score:     1.0,
						MatchType: model.MatchStrategyExact,
					})
					continue
				}
			}

			// Try normalized match.
			if modeEnabled[model.MatchStrategyNormalized] {
				if NormalizeTerm(term.Text) == normalizedSource {
					matches = append(matches, TermMatch{
						Concept:   concept,
						Term:      term,
						Score:     0.95,
						MatchType: model.MatchStrategyNormalized,
					})
					continue
				}
			}

			// Try fuzzy match.
			if modeEnabled[model.MatchStrategyFuzzy] {
				score := sievepen.LevenshteinRatio(normalizedSource, NormalizeTerm(term.Text))
				if score >= opts.MinScore {
					matches = append(matches, TermMatch{
						Concept:   concept,
						Term:      term,
						Score:     score,
						MatchType: model.MatchStrategyFuzzy,
					})
				}
			}
		}
	}

	// Sort by score descending.
	slices.SortFunc(matches, func(a, b TermMatch) int {
		return cmp.Compare(b.Score, a.Score)
	})

	return matches
}

// LookupAll finds all terms that appear in the given text.
func (tb *InMemoryTermBase) LookupAll(sourceText string, opts LookupOptions) []TermMatch {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if sourceText == "" {
		return nil
	}

	opts = ApplyLookupDefaults(opts)
	var matches []TermMatch
	lowerSource := strings.ToLower(sourceText)

	for _, concept := range tb.concepts {
		if !matchesDomain(concept, opts.Domains) {
			continue
		}
		if !matchesSource(concept, opts.SourceFilter) {
			continue
		}

		for _, term := range concept.Terms {
			if term.Locale != opts.SourceLocale {
				continue
			}
			if !MatchesStatus(term.Status, opts.StatusFilter) {
				continue
			}

			termText := term.Text
			searchIn := sourceText
			searchFor := termText
			if !opts.CaseSensitive {
				searchIn = lowerSource
				searchFor = strings.ToLower(termText)
			}

			// Find all occurrences of this term in the text.
			offset := 0
			for {
				idx := strings.Index(searchIn[offset:], searchFor)
				if idx < 0 {
					break
				}
				pos := offset + idx
				matches = append(matches, TermMatch{
					Concept:   concept,
					Term:      term,
					Score:     1.0,
					MatchType: model.MatchStrategyExact,
					Position:  model.TextRange{Start: pos, End: pos + len(searchFor)},
				})
				offset = pos + len(searchFor)
			}
		}
	}

	// Sort by position in text.
	slices.SortFunc(matches, func(a, b TermMatch) int {
		if c := cmp.Compare(a.Position.Start, b.Position.Start); c != 0 {
			return c
		}
		// Longer matches first for overlapping terms.
		return cmp.Compare(b.Position.End, a.Position.End)
	})

	return matches
}

// Search performs a case-insensitive text search across terms and definitions.
func (tb *InMemoryTermBase) Search(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	lowerQuery := strings.ToLower(query)
	var matched []Concept

	for _, concept := range tb.concepts {
		if matchesConcept(concept, lowerQuery, sourceLocale, targetLocale) {
			matched = append(matched, concept)
		}
	}

	total := len(matched)
	if offset >= total {
		return nil, total
	}
	end := min(offset+limit, total)
	return matched[offset:end], total
}

// Count returns the total number of concepts.
func (tb *InMemoryTermBase) Count() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.concepts)
}

// Concepts returns a copy of all concepts.
func (tb *InMemoryTermBase) Concepts() []Concept {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	out := make([]Concept, len(tb.concepts))
	copy(out, tb.concepts)
	return out
}

// Close releases resources. For InMemoryTermBase, this is a no-op.
func (tb *InMemoryTermBase) Close() error {
	return nil
}

// --- helpers ---

// ApplyLookupDefaults fills unset LookupOptions fields with their defaults: a
// non-positive MinScore becomes 0.8.
func ApplyLookupDefaults(opts LookupOptions) LookupOptions {
	if opts.MinScore <= 0 {
		opts.MinScore = 0.8
	}
	return opts
}

// MatchModesEnabled returns the set of enabled match strategies. An empty modes
// slice enables all of exact, normalized, and fuzzy matching.
func MatchModesEnabled(modes []model.MatchStrategy) map[model.MatchStrategy]bool {
	if len(modes) == 0 {
		return map[model.MatchStrategy]bool{
			model.MatchStrategyExact:      true,
			model.MatchStrategyNormalized: true,
			model.MatchStrategyFuzzy:      true,
		}
	}
	m := make(map[model.MatchStrategy]bool)
	for _, mode := range modes {
		m[mode] = true
	}
	return m
}

func matchesSource(c Concept, filter []TermSource) bool {
	if len(filter) == 0 {
		return true
	}
	source := c.Source
	if source == "" {
		source = TermSourceTerminology
	}
	for _, f := range filter {
		if f == source {
			return true
		}
	}
	return false
}

func matchesDomain(c Concept, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	for _, d := range domains {
		if strings.EqualFold(c.Domain, d) {
			return true
		}
	}
	return false
}

// MatchesStatus reports whether status is in the filter. An empty filter
// matches any status.
func MatchesStatus(status model.TermStatus, filter []model.TermStatus) bool {
	if len(filter) == 0 {
		return true
	}
	return slices.Contains(filter, status)
}

func matchesTerm(text, term string, caseSensitive bool) bool {
	if caseSensitive {
		return text == term
	}
	return strings.EqualFold(text, term)
}

// NormalizeTerm normalizes a term for comparison by applying Unicode NFC
// normalization, lowercasing, trimming whitespace, and collapsing internal
// whitespace to single spaces.
func NormalizeTerm(s string) string {
	s = norm.NFC.String(s)
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
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
