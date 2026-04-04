package sievepen

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/text/unicode/norm"
)

// DefaultMaxEntries is the default maximum number of entries in an InMemoryTM.
// A value of 0 means unlimited.
const DefaultMaxEntries = 0

// normalizedEntry caches the pre-computed normalized text forms for a TMEntry,
// avoiding repeated NormalizeText calls during lookup scans.
type normalizedEntry struct {
	entry      TMEntry
	plainNorm  string // NormalizeText(entry.SourceText())
	structNorm string // NormalizeText(entry.SourceStructural())
	genNorm    string // NormalizeText(entry.SourceGeneralized())
}

// InMemoryTM is a thread-safe, in-memory implementation of TranslationMemory
// with content-aware tiered matching (generalized → structural → plain).
type InMemoryTM struct {
	mu         sync.RWMutex
	entries    []normalizedEntry
	byID       map[string]int // maps entry ID to index in entries slice
	maxEntries int            // 0 = unlimited
}

// InMemoryTMOption configures an InMemoryTM instance.
type InMemoryTMOption func(*InMemoryTM)

// WithMaxEntries sets the maximum number of entries. When the limit is reached,
// the oldest entry is evicted to make room. A value of 0 means unlimited.
func WithMaxEntries(max int) InMemoryTMOption {
	return func(tm *InMemoryTM) {
		tm.maxEntries = max
	}
}

// NewInMemoryTM creates a new empty in-memory translation memory.
func NewInMemoryTM(opts ...InMemoryTMOption) *InMemoryTM {
	tm := &InMemoryTM{
		byID:       make(map[string]int),
		maxEntries: DefaultMaxEntries,
	}
	for _, opt := range opts {
		opt(tm)
	}
	return tm
}

// Add inserts or updates an entry in the translation memory.
func (tm *InMemoryTM) Add(entry TMEntry) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry.ID == "" {
		return errors.New("entry ID is required")
	}
	if entry.Source == nil {
		return errors.New("entry source Fragment is required")
	}

	ne := normalizedEntry{
		entry:      entry,
		plainNorm:  NormalizeText(entry.SourceText()),
		structNorm: NormalizeText(entry.SourceStructural()),
		genNorm:    NormalizeText(entry.SourceGeneralized()),
	}

	if _, exists := tm.byID[entry.ID]; exists {
		idx := tm.byID[entry.ID]
		tm.entries[idx] = ne
		return nil
	}

	// Evict the oldest entry if we've reached the size limit.
	if tm.maxEntries > 0 && len(tm.entries) >= tm.maxEntries {
		tm.evictOldest()
	}

	tm.byID[entry.ID] = len(tm.entries)
	tm.entries = append(tm.entries, ne)
	return nil
}

// evictOldest removes the first (oldest) entry. Caller must hold tm.mu.
func (tm *InMemoryTM) evictOldest() {
	if len(tm.entries) == 0 {
		return
	}
	oldest := tm.entries[0]
	delete(tm.byID, oldest.entry.ID)

	// Shift entries and update index map.
	copy(tm.entries, tm.entries[1:])
	tm.entries = tm.entries[:len(tm.entries)-1]
	for id, idx := range tm.byID {
		tm.byID[id] = idx - 1
	}
}

// MaxEntries returns the configured maximum entry count (0 = unlimited).
func (tm *InMemoryTM) MaxEntries() int {
	return tm.maxEntries
}

// Lookup searches for matches using tiered matching. The source Block's
// entity annotations are used to compute the generalized key.
func (tm *InMemoryTM) Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if source == nil {
		return nil, nil
	}

	opts = ApplyDefaults(opts)
	frag := source.FirstFragment()
	if frag == nil {
		return nil, nil
	}

	// Compute lookup keys from the source block.
	plainKey := NormalizeText(frag.Text())
	structKey := NormalizeText(frag.StructuralText())
	generalKey := NormalizeText(frag.GeneralizedText())

	// Extract entity annotations from the block for adaptation computation.
	entityAnnotations := ExtractEntityAnnotations(source)

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts), nil
}

// LookupText searches for matches using plain text only.
// This always uses plain-mode matching, returning MatchExact/MatchFuzzy types.
func (tm *InMemoryTM) LookupText(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	opts = ApplyDefaults(opts)
	opts.MatchModes = []MatchMode{MatchModePlain}
	normalizedSource := NormalizeText(source)

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.tieredLookup(normalizedSource, normalizedSource, normalizedSource, nil, sourceLocale, targetLocale, opts), nil
}

// tieredLookup performs the 6-tier matching pipeline.
// It uses pre-computed normalized text from normalizedEntry to avoid
// calling NormalizeText on every stored entry during each lookup.
func (tm *InMemoryTM) tieredLookup(plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) []TMMatch {
	var matches []TMMatch
	seen := make(map[string]bool) // track entry IDs to avoid duplicates

	modeEnabled := MatchModesEnabled(opts.MatchModes)

	// Tier 1: generalized exact
	if modeEnabled[MatchModeGeneralized] {
		for i := range tm.entries {
			ne := &tm.entries[i]
			if !matchesLocale(ne.entry, sourceLocale, targetLocale) {
				continue
			}
			if ne.genNorm == generalKey {
				if !seen[ne.entry.ID] {
					seen[ne.entry.ID] = true
					adaptations := ComputeEntityAdaptations(ne.entry, entityAnnotations)
					matches = append(matches, TMMatch{
						Entry:             ne.entry,
						Score:             1.0,
						MatchType:         MatchGeneralizedExact,
						EntityAdaptations: adaptations,
					})
				}
			}
		}
	}

	// Tier 2: structural exact
	if modeEnabled[MatchModeStructural] {
		for i := range tm.entries {
			ne := &tm.entries[i]
			if !matchesLocale(ne.entry, sourceLocale, targetLocale) || seen[ne.entry.ID] {
				continue
			}
			if ne.structNorm == structKey {
				seen[ne.entry.ID] = true
				matches = append(matches, TMMatch{
					Entry:     ne.entry,
					Score:     1.0,
					MatchType: MatchStructuralExact,
				})
			}
		}
	}

	// Tier 3: plain exact
	if modeEnabled[MatchModePlain] {
		for i := range tm.entries {
			ne := &tm.entries[i]
			if !matchesLocale(ne.entry, sourceLocale, targetLocale) || seen[ne.entry.ID] {
				continue
			}
			if ne.plainNorm == plainKey {
				seen[ne.entry.ID] = true
				matches = append(matches, TMMatch{
					Entry:     ne.entry,
					Score:     1.0,
					MatchType: MatchExact,
				})
			}
		}
	}

	// If we have exact matches at or above threshold, return early.
	if len(matches) > 0 && opts.MinScore >= 1.0 {
		return LimitResults(matches, opts.MaxResults)
	}

	// Tier 4: generalized fuzzy
	if modeEnabled[MatchModeGeneralized] {
		for i := range tm.entries {
			ne := &tm.entries[i]
			if !matchesLocale(ne.entry, sourceLocale, targetLocale) || seen[ne.entry.ID] {
				continue
			}
			score := LevenshteinRatio(generalKey, ne.genNorm)
			if score >= opts.MinScore {
				seen[ne.entry.ID] = true
				adaptations := ComputeEntityAdaptations(ne.entry, entityAnnotations)
				matches = append(matches, TMMatch{
					Entry:             ne.entry,
					Score:             score,
					MatchType:         MatchGeneralizedFuzzy,
					EntityAdaptations: adaptations,
				})
			}
		}
	}

	// Tier 5: structural fuzzy
	if modeEnabled[MatchModeStructural] {
		for i := range tm.entries {
			ne := &tm.entries[i]
			if !matchesLocale(ne.entry, sourceLocale, targetLocale) || seen[ne.entry.ID] {
				continue
			}
			score := LevenshteinRatio(structKey, ne.structNorm)
			if score >= opts.MinScore {
				seen[ne.entry.ID] = true
				matches = append(matches, TMMatch{
					Entry:     ne.entry,
					Score:     score,
					MatchType: MatchStructuralFuzzy,
				})
			}
		}
	}

	// Tier 6: plain fuzzy
	if modeEnabled[MatchModePlain] {
		for i := range tm.entries {
			ne := &tm.entries[i]
			if !matchesLocale(ne.entry, sourceLocale, targetLocale) || seen[ne.entry.ID] {
				continue
			}
			score := LevenshteinRatio(plainKey, ne.plainNorm)
			if score >= opts.MinScore {
				seen[ne.entry.ID] = true
				matches = append(matches, TMMatch{
					Entry:     ne.entry,
					Score:     score,
					MatchType: MatchFuzzy,
				})
			}
		}
	}

	// Sort by match type priority, then by score descending.
	slices.SortFunc(matches, func(a, b TMMatch) int {
		pa := MatchTypePriority(a.MatchType)
		pb := MatchTypePriority(b.MatchType)
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return cmp.Compare(b.Score, a.Score)
	})

	return LimitResults(matches, opts.MaxResults)
}

// Delete removes an entry by ID.
func (tm *InMemoryTM) Delete(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	idx, exists := tm.byID[id]
	if !exists {
		return fmt.Errorf("entry not found: %s", id)
	}

	lastIdx := len(tm.entries) - 1
	if idx != lastIdx {
		tm.entries[idx] = tm.entries[lastIdx]
		tm.byID[tm.entries[idx].entry.ID] = idx
	}
	tm.entries = tm.entries[:lastIdx]
	delete(tm.byID, id)

	return nil
}

// Count returns the total number of entries.
func (tm *InMemoryTM) Count() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.entries)
}

// Close releases resources. For InMemoryTM, this is a no-op.
func (tm *InMemoryTM) Close() error {
	return nil
}

// Entries returns a copy of all entries. Used for export operations.
func (tm *InMemoryTM) Entries() []TMEntry {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]TMEntry, len(tm.entries))
	for i, ne := range tm.entries {
		out[i] = ne.entry
	}
	return out
}

// SearchEntries performs a case-insensitive substring search on source/target text.
func (tm *InMemoryTM) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]TMEntry, int) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	lowerQuery := strings.ToLower(query)
	var matched []TMEntry

	for _, ne := range tm.entries {
		if sourceLocale != "" && string(ne.entry.SourceLocale) != sourceLocale {
			continue
		}
		if targetLocale != "" && string(ne.entry.TargetLocale) != targetLocale {
			continue
		}
		if query != "" {
			srcText := strings.ToLower(ne.entry.SourceText())
			tgtText := strings.ToLower(ne.entry.TargetText())
			if !strings.Contains(srcText, lowerQuery) && !strings.Contains(tgtText, lowerQuery) {
				continue
			}
		}
		matched = append(matched, ne.entry)
	}

	total := len(matched)
	if offset >= total {
		return nil, total
	}
	end := min(offset+limit, total)
	return matched[offset:end], total
}

// GetEntry fetches a single entry by ID.
func (tm *InMemoryTM) GetEntry(id string) (TMEntry, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	idx, exists := tm.byID[id]
	if !exists {
		return TMEntry{}, false
	}
	return tm.entries[idx].entry, true
}

// --- helpers ---

func ApplyDefaults(opts LookupOptions) LookupOptions {
	if opts.MinScore <= 0 {
		opts.MinScore = 0.7
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}
	return opts
}

func matchesLocale(entry TMEntry, sourceLocale, targetLocale model.LocaleID) bool {
	return entry.SourceLocale == sourceLocale && entry.TargetLocale == targetLocale
}

func MatchModesEnabled(modes []MatchMode) map[MatchMode]bool {
	if len(modes) == 0 {
		return map[MatchMode]bool{
			MatchModeGeneralized: true,
			MatchModeStructural:  true,
			MatchModePlain:       true,
		}
	}
	m := make(map[MatchMode]bool)
	for _, mode := range modes {
		m[mode] = true
	}
	return m
}

func MatchTypePriority(mt MatchType) int {
	switch mt {
	case MatchGeneralizedExact:
		return 0
	case MatchStructuralExact:
		return 1
	case MatchExact:
		return 2
	case MatchGeneralizedFuzzy:
		return 3
	case MatchStructuralFuzzy:
		return 4
	case MatchFuzzy:
		return 5
	default:
		return 6
	}
}

func LimitResults(matches []TMMatch, max int) []TMMatch {
	if len(matches) > max {
		return matches[:max]
	}
	return matches
}

// ExtractEntityAnnotations pulls EntityAnnotation instances from a Block's annotations.
func ExtractEntityAnnotations(block *model.Block) []*model.EntityAnnotation {
	if block.Annotations == nil {
		return nil
	}
	var entities []*model.EntityAnnotation
	for _, ann := range block.Annotations {
		if ea, ok := ann.(*model.EntityAnnotation); ok {
			entities = append(entities, ea)
		}
	}
	return entities
}

// ComputeEntityAdaptations computes how to adapt entity values from a stored
// TM entry to match the current source content.
func ComputeEntityAdaptations(entry TMEntry, currentEntities []*model.EntityAnnotation) []EntityAdaptation {
	if len(entry.Entities) == 0 || len(currentEntities) == 0 {
		return nil
	}

	var adaptations []EntityAdaptation

	// Match stored entities to current entities by type, in order.
	// This is a simple positional matching — entities of the same type
	// are matched left-to-right.
	typeQueues := make(map[model.EntityType][]*model.EntityAnnotation)
	for _, ea := range currentEntities {
		typeQueues[ea.Type] = append(typeQueues[ea.Type], ea)
	}

	typeIdx := make(map[model.EntityType]int)
	for _, em := range entry.Entities {
		queue := typeQueues[em.Type]
		idx := typeIdx[em.Type]
		if idx < len(queue) {
			current := queue[idx]
			typeIdx[em.Type] = idx + 1

			if em.TargetValue != current.Text {
				adaptations = append(adaptations, EntityAdaptation{
					PlaceholderID: em.PlaceholderID,
					Type:          em.Type,
					StoredValue:   em.TargetValue,
					CurrentValue:  current.Text,
					TargetPos:     em.TargetPos,
				})
			}
		}
	}

	return adaptations
}

// NormalizeText normalizes text for comparison by applying Unicode NFC
// normalization, trimming whitespace, and collapsing internal whitespace
// to single spaces. NFC normalization ensures consistent representation
// of composed characters (e.g., Hangul jamo → syllables, combining
// diacritics → precomposed forms, Arabic tashkeel).
func NormalizeText(s string) string {
	s = norm.NFC.String(s)

	// Fast path: if the string contains no whitespace anomalies, return as-is.
	// This avoids allocation for the common case of already-clean text.
	if !needsWhitespaceNormalization(s) {
		return s
	}

	// Single-pass: trim leading/trailing whitespace and collapse internal
	// runs of whitespace to a single space, avoiding the two extra
	// allocations from strings.Fields + strings.Join.
	var b strings.Builder
	b.Grow(len(s))
	inSpace := true // treat leading whitespace as a run to skip
	for _, r := range s {
		if isWhitespace(r) {
			inSpace = true
		} else {
			if inSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			inSpace = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// needsWhitespaceNormalization reports whether s has leading/trailing whitespace,
// consecutive whitespace, or non-space whitespace characters.
func needsWhitespaceNormalization(s string) bool {
	if len(s) == 0 {
		return false
	}
	if isWhitespace(rune(s[0])) || isWhitespace(rune(s[len(s)-1])) {
		return true
	}
	prev := false
	for _, r := range s {
		ws := isWhitespace(r)
		if ws && prev {
			return true // consecutive whitespace
		}
		if ws && r != ' ' {
			return true // tab, newline, etc.
		}
		prev = ws
	}
	return false
}

// isWhitespace reports whether r is a Unicode whitespace character,
// matching the same set as strings.Fields / unicode.IsSpace.
func isWhitespace(r rune) bool {
	// Common ASCII cases first for speed.
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	case '\u0085', '\u00A0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A',
		'\u2028', '\u2029', '\u202F', '\u205F', '\u3000':
		return true
	}
	return false
}
