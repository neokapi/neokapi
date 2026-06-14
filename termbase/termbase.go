package termbase

import (
	"context"
	"errors"
	"fmt"
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
	Validity       *graph.Validity  `json:"validity,omitempty"`        // time/tag scoping; nil = always valid
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
	Scope         *graph.Scope       // validity scope; nil = no validity filtering
}

// ConceptRelation is a persisted, typed relationship between two concepts —
// the edge of the brand knowledge graph (AD-021). Each relation may carry a
// Validity: a half-open time interval plus free tags evaluated against a
// query-time scope, so the same edge can hold in one market and be absent in
// another.
type ConceptRelation struct {
	ID           string          `json:"id"`
	SourceID     string          `json:"source_id"`
	TargetID     string          `json:"target_id"`
	RelationType string          `json:"relation_type"` // Uses graph.Label* constants
	Note         string          `json:"note,omitempty"`
	Validity     *graph.Validity `json:"validity,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TermDesignation represents a term with temporal validity for the status-on-edge model.
type TermDesignation struct {
	Term     Term            `json:"term"`
	Validity *graph.Validity `json:"validity,omitempty"`
}

// knownRelationTypes is the concept-to-concept relation vocabulary (AD-021):
// hierarchy, succession, guidance, cross-scheme equivalence, and brand stance.
// Concept-to-term edge labels (HAS_TERM, FORBIDDEN, PREFERRED) are deliberately
// excluded — those are modeled as term statuses, not relations.
var knownRelationTypes = map[string]bool{
	graph.LabelBroader:    true,
	graph.LabelNarrower:   true,
	graph.LabelPartOf:     true,
	graph.LabelHasPart:    true,
	graph.LabelRelated:    true,
	graph.LabelReplacedBy: true,
	graph.LabelUseInstead: true,
	graph.LabelExactMatch: true,
	graph.LabelCloseMatch: true,
	graph.LabelCompetitor: true,
}

// KnownRelationType reports whether relationType is part of the
// concept-relation vocabulary accepted by AddRelation.
func KnownRelationType(relationType string) bool {
	return knownRelationTypes[relationType]
}

// ValidateRelation checks the structural invariants of a relation: a non-empty
// ID, non-empty source and target concept IDs, and a known relation type.
// Backends additionally verify that the referenced concepts exist.
func ValidateRelation(rel ConceptRelation) error {
	if rel.ID == "" {
		return errors.New("relation ID is required")
	}
	if rel.SourceID == "" || rel.TargetID == "" {
		return errors.New("relation source and target concept IDs are required")
	}
	if !KnownRelationType(rel.RelationType) {
		return fmt.Errorf("unknown relation type: %q", rel.RelationType)
	}
	return nil
}

// MatchesScope reports whether a validity is active at the given scope.
// A nil scope means no validity filtering: everything matches.
func MatchesScope(v *graph.Validity, scope *graph.Scope) bool {
	if scope == nil {
		return true
	}
	return v.Matches(*scope)
}

// validateConceptTermStatuses rejects terms whose Status is set to a value
// outside the model.Term* lifecycle vocabulary. An empty status is allowed
// (callers may leave it unset). Transitions between statuses are deliberately
// not enforced here — see ValidateTransition.
func validateConceptTermStatuses(concept Concept) error {
	for _, t := range concept.Terms {
		if t.Status != "" && !KnownTermStatus(t.Status) {
			return fmt.Errorf("term %q (%s): unknown status %q", t.Text, t.Locale, t.Status)
		}
	}
	return nil
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

	// AddRelation inserts or updates (by ID) a relation between two concepts.
	// The relation type must be a known graph label and both concepts must exist.
	AddRelation(ctx context.Context, rel ConceptRelation) error

	// DeleteRelation removes a relation by ID.
	DeleteRelation(ctx context.Context, id string) error

	// RelationsOf returns all relations touching the concept, in either
	// direction. When scope is non-nil, relations whose validity does not
	// match the scope are filtered out.
	RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]ConceptRelation, error)

	// ListRelations returns all relations (for export). When scope is non-nil,
	// relations whose validity does not match the scope are filtered out.
	ListRelations(ctx context.Context, scope *graph.Scope) ([]ConceptRelation, error)

	// Close releases resources.
	Close() error
}
