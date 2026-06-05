package termbase

import (
	"context"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
)

// TermSource indicates whether a concept comes from traditional terminology or brand vocabulary.
type TermSource string

const (
	TermSourceTerminology     TermSource = "terminology"
	TermSourceBrandVocabulary TermSource = "brand_vocabulary"
)

// Term represents a single term in a specific locale with lifecycle metadata.
type Term struct {
	Text           string           `json:"text"`                      // the term text
	Locale         model.LocaleID   `json:"locale"`                    // language/locale
	Status         model.TermStatus `json:"status"`                    // lifecycle status
	PartOfSpeech   string           `json:"part_of_speech,omitempty"`  // noun, verb, adjective, etc.
	Gender         string           `json:"gender,omitempty"`          // grammatical gender (if applicable)
	Note           string           `json:"note,omitempty"`            // usage note or context
	CompetitorTerm bool             `json:"competitor_term,omitempty"` // true if this is a competitor brand term
}

// Concept is the central unit of a termbase — a language-neutral concept
// with terms in multiple locales, organized following TBX principles.
type Concept struct {
	ID         string            `json:"id"`                   // unique concept identifier
	ProjectID  string            `json:"project_id,omitempty"` // project scope (empty = workspace-scoped)
	Domain     string            `json:"domain,omitempty"`     // subject field (software, medical, legal, etc.)
	Definition string            `json:"definition,omitempty"` // language-neutral definition
	Source     TermSource        `json:"source,omitempty"`     // "terminology" or "brand_vocabulary"
	Terms      []Term            `json:"terms,omitempty"`      // terms across locales
	Properties map[string]string `json:"properties,omitempty"` // extensible metadata
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
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

// ProjectScope controls project filtering in term lookups.
type ProjectScope int

const (
	ProjectScopeAll     ProjectScope = iota // workspace-wide, project-scoped takes precedence
	ProjectScopeOnly                        // current project only
	ProjectScopeExclude                     // other projects only
)

// LookupOptions controls term lookup behavior.
type LookupOptions struct {
	SourceLocale  model.LocaleID
	TargetLocale  model.LocaleID
	CaseSensitive bool
	MinScore      float64 // minimum match score for fuzzy (default 0.8)
	MatchModes    []model.MatchStrategy
	Domains       []string           // restrict to specific domains
	StatusFilter  []model.TermStatus // only return terms with these statuses
	SourceFilter  []TermSource       // filter by term source (empty = all)
	ProjectID     string             // project context for scope filtering
	ProjectScope  ProjectScope       // project filtering mode (default: all)
}

// ConceptRelation represents a relationship between two concepts for graph import/export.
type ConceptRelation struct {
	SourceID     string `json:"source_id"`
	TargetID     string `json:"target_id"`
	RelationType string `json:"relation_type"` // Uses graph.Label* constants
}

// TermDesignation represents a term with temporal validity for the status-on-edge model.
type TermDesignation struct {
	Term     Term            `json:"term"`
	Validity *graph.Validity `json:"validity,omitempty"`
}

// TermBase defines the interface for a terminology database.
type TermBase interface {
	// AddConcept inserts or updates a concept with all its terms.
	AddConcept(ctx context.Context, concept Concept) error

	// GetConcept retrieves a concept by ID.
	GetConcept(ctx context.Context, id string) (Concept, bool, error)

	// DeleteConcept removes a concept by ID.
	DeleteConcept(ctx context.Context, id string) error

	// Lookup finds terms matching the source text.
	Lookup(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error)

	// LookupAll finds all terms that appear in the given text.
	// Returns matches sorted by position in the text.
	LookupAll(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error)

	// Search performs a text search across terms and definitions.
	Search(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error)

	// Count returns the total number of concepts.
	Count(ctx context.Context) (int, error)

	// Concepts returns all concepts (for export).
	Concepts(ctx context.Context) ([]Concept, error)

	// Close releases resources.
	Close() error
}
