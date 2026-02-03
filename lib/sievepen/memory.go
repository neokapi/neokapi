package sievepen

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gokapi/gokapi/core/model"
)

// InMemoryTM is a thread-safe, in-memory implementation of TranslationMemory.
type InMemoryTM struct {
	mu      sync.RWMutex
	entries []TMEntry
	byID    map[string]int // maps entry ID to index in entries slice
}

// NewInMemoryTM creates a new empty in-memory translation memory.
func NewInMemoryTM() *InMemoryTM {
	return &InMemoryTM{
		entries: make([]TMEntry, 0),
		byID:    make(map[string]int),
	}
}

// Add inserts a new entry into the translation memory.
func (tm *InMemoryTM) Add(entry TMEntry) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry.ID == "" {
		return fmt.Errorf("entry ID is required")
	}

	if _, exists := tm.byID[entry.ID]; exists {
		// Update existing entry.
		idx := tm.byID[entry.ID]
		tm.entries[idx] = entry
		return nil
	}

	tm.byID[entry.ID] = len(tm.entries)
	tm.entries = append(tm.entries, entry)
	return nil
}

// Lookup searches for matches of the given source text in the translation memory.
func (tm *InMemoryTM) Lookup(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if opts.MinScore <= 0 {
		opts.MinScore = 0.7
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}

	normalizedSource := normalizeText(source)
	var matches []TMMatch

	for _, entry := range tm.entries {
		if entry.SourceLocale != sourceLocale || entry.TargetLocale != targetLocale {
			continue
		}

		normalizedEntry := normalizeText(entry.Source)

		var score float64
		var matchType MatchType

		if normalizedEntry == normalizedSource {
			score = 1.0
			matchType = MatchExact
		} else {
			score = LevenshteinRatio(normalizedSource, normalizedEntry)
			matchType = MatchFuzzy
		}

		if score >= opts.MinScore {
			matches = append(matches, TMMatch{
				Entry:     entry,
				Score:     score,
				MatchType: matchType,
			})
		}
	}

	// Sort by score descending.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > opts.MaxResults {
		matches = matches[:opts.MaxResults]
	}

	return matches, nil
}

// Delete removes an entry by ID.
func (tm *InMemoryTM) Delete(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	idx, exists := tm.byID[id]
	if !exists {
		return fmt.Errorf("entry not found: %s", id)
	}

	// Remove from slice by swapping with last element.
	lastIdx := len(tm.entries) - 1
	if idx != lastIdx {
		tm.entries[idx] = tm.entries[lastIdx]
		tm.byID[tm.entries[idx].ID] = idx
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
	copy(out, tm.entries)
	return out
}

// SearchEntries performs a case-insensitive substring search on source/target text
// with optional locale filtering and pagination. Empty strings mean "no filter".
// Returns matched entries and total count.
func (tm *InMemoryTM) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]TMEntry, int) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	lowerQuery := strings.ToLower(query)
	var matched []TMEntry

	for _, entry := range tm.entries {
		if sourceLocale != "" && string(entry.SourceLocale) != sourceLocale {
			continue
		}
		if targetLocale != "" && string(entry.TargetLocale) != targetLocale {
			continue
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(entry.Source), lowerQuery) &&
			!strings.Contains(strings.ToLower(entry.Target), lowerQuery) {
			continue
		}
		matched = append(matched, entry)
	}

	total := len(matched)
	if offset >= total {
		return nil, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
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
	return tm.entries[idx], true
}

// normalizeText normalizes text for comparison by trimming whitespace
// and collapsing internal whitespace to single spaces.
func normalizeText(s string) string {
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
