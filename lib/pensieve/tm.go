package pensieve

import (
	"time"

	"github.com/gokapi/gokapi/core/model"
)

// MatchType indicates how a TM match was found.
type MatchType int

const (
	// MatchExact indicates the source text matched exactly.
	MatchExact MatchType = iota
	// MatchFuzzy indicates the source text matched approximately.
	MatchFuzzy
)

// String returns a human-readable representation of the match type.
func (mt MatchType) String() string {
	switch mt {
	case MatchExact:
		return "exact"
	case MatchFuzzy:
		return "fuzzy"
	default:
		return "unknown"
	}
}

// TMEntry represents a single translation memory entry.
type TMEntry struct {
	ID           string
	Source       string
	Target       string
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Properties   map[string]string
}

// TMMatch represents a match result from a TM lookup.
type TMMatch struct {
	Entry     TMEntry
	Score     float64 // 0.0-1.0 (1.0 = exact match)
	MatchType MatchType
}

// LookupOptions controls the behavior of TM lookups.
type LookupOptions struct {
	MinScore   float64 // Minimum match score (default 0.7)
	MaxResults int     // Maximum results to return (default 10)
}

// DefaultLookupOptions returns sensible defaults for TM lookups.
func DefaultLookupOptions() LookupOptions {
	return LookupOptions{
		MinScore:   0.7,
		MaxResults: 10,
	}
}

// TranslationMemory defines the interface for a translation memory store.
type TranslationMemory interface {
	// Add inserts a new entry into the translation memory.
	Add(entry TMEntry) error

	// Lookup searches for matches of the given source text.
	Lookup(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

	// Delete removes an entry by ID.
	Delete(id string) error

	// Count returns the total number of entries.
	Count() int

	// Close releases any resources held by the translation memory.
	Close() error
}
