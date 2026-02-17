package termbase

import (
	"time"

	"github.com/gokapi/gokapi/core/model"
)

// Term represents a single term in a specific locale with lifecycle metadata.
type Term struct {
	Text         string           // the term text
	Locale       model.LocaleID   // language/locale
	Status       model.TermStatus // lifecycle status
	PartOfSpeech string           // noun, verb, adjective, etc.
	Gender       string           // grammatical gender (if applicable)
	Note         string           // usage note or context
}

// Concept is the central unit of a termbase — a language-neutral concept
// with terms in multiple locales, organized following TBX principles.
type Concept struct {
	ID         string            // unique concept identifier
	Domain     string            // subject field (software, medical, legal, etc.)
	Definition string            // language-neutral definition
	Terms      []Term            // terms across locales
	Properties map[string]string // extensible metadata
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SourceTerm returns the first term matching the given locale.
func (c *Concept) SourceTerm(locale model.LocaleID) *Term {
	for i := range c.Terms {
		if c.Terms[i].Locale == locale {
			return &c.Terms[i]
		}
	}
	return nil
}

// TargetTerms returns all terms for the given locale.
func (c *Concept) TargetTerms(locale model.LocaleID) []Term {
	var result []Term
	for _, t := range c.Terms {
		if t.Locale == locale {
			result = append(result, t)
		}
	}
	return result
}

// PreferredTerm returns the preferred (or first approved) term for a locale.
func (c *Concept) PreferredTerm(locale model.LocaleID) *Term {
	var fallback *Term
	for i := range c.Terms {
		if c.Terms[i].Locale != locale {
			continue
		}
		if c.Terms[i].Status == model.TermPreferred {
			return &c.Terms[i]
		}
		if fallback == nil && c.Terms[i].Status != model.TermForbidden && c.Terms[i].Status != model.TermDeprecated {
			t := c.Terms[i]
			fallback = &t
		}
	}
	return fallback
}

// TermMatch represents a term found during lookup.
type TermMatch struct {
	Concept   Concept
	Term      Term    // the matched source term
	Score     float64 // match confidence (1.0 = exact)
	MatchType model.MatchStrategy
	Position  model.TextRange // where in the source text
}

// LookupOptions controls term lookup behavior.
type LookupOptions struct {
	SourceLocale  model.LocaleID
	TargetLocale  model.LocaleID
	CaseSensitive bool
	MinScore      float64 // minimum match score for fuzzy (default 0.8)
	MatchModes    []model.MatchStrategy
	Domains       []string           // restrict to specific domains
	StatusFilter  []model.TermStatus // only return terms with these statuses
}

// TermBase defines the interface for a terminology database.
type TermBase interface {
	// AddConcept inserts or updates a concept with all its terms.
	AddConcept(concept Concept) error

	// GetConcept retrieves a concept by ID.
	GetConcept(id string) (Concept, bool)

	// DeleteConcept removes a concept by ID.
	DeleteConcept(id string) error

	// Lookup finds terms matching the source text.
	Lookup(sourceText string, opts LookupOptions) []TermMatch

	// LookupAll finds all terms that appear in the given text.
	// Returns matches sorted by position in the text.
	LookupAll(sourceText string, opts LookupOptions) []TermMatch

	// Search performs a text search across terms and definitions.
	Search(query string, sourceLocale, targetLocale string, offset, limit int) ([]Concept, int)

	// Count returns the total number of concepts.
	Count() int

	// Concepts returns all concepts (for export).
	Concepts() []Concept

	// Close releases resources.
	Close() error
}
