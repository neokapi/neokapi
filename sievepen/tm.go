package sievepen

import (
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// MatchType indicates how a TM match was found, ordered by reuse potential.
type MatchType string

const (
	MatchGeneralizedExact MatchType = "generalized-exact"
	MatchStructuralExact  MatchType = "structural-exact"
	MatchExact            MatchType = "exact"
	MatchGeneralizedFuzzy MatchType = "generalized-fuzzy"
	MatchStructuralFuzzy  MatchType = "structural-fuzzy"
	MatchFuzzy            MatchType = "fuzzy"
)

// String returns the string representation of the match type.
func (mt MatchType) String() string {
	return string(mt)
}

// IsExact returns true if this is an exact match (any tier).
func (mt MatchType) IsExact() bool {
	return mt == MatchGeneralizedExact || mt == MatchStructuralExact || mt == MatchExact
}

// EntityMapping tracks a named entity and its values in source and target.
// This enables generalized matching: entities are replaced with typed
// placeholders in the matching key, so structurally identical segments
// match regardless of entity values.
type EntityMapping struct {
	PlaceholderID string           // "e1", "e2" — links source and target positions
	Type          model.EntityType // person, product, organization, date, etc.
	SourceValue   string           // original value in source ("John")
	SourcePos     model.TextRange  // position in source fragment
	TargetValue   string           // original value in target ("John" or adapted form)
	TargetPos     model.TextRange  // position in target fragment
}

// TMEntry represents a single translation memory entry with full
// content model representation. Stores Fragments (not plain strings)
// to preserve inline markup and entity metadata.
type TMEntry struct {
	ID           string
	ProjectID    string          // project scope (empty = workspace-scoped)
	Source       *model.Fragment // coded text + inline spans
	Target       *model.Fragment // coded text + inline spans
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Entities     []EntityMapping // entity placeholders in this entry
	Properties   map[string]string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SourceText returns the plain text of the source fragment.
func (e *TMEntry) SourceText() string {
	if e.Source == nil {
		return ""
	}
	return e.Source.Text()
}

// TargetText returns the plain text of the target fragment.
func (e *TMEntry) TargetText() string {
	if e.Target == nil {
		return ""
	}
	return e.Target.Text()
}

// SourceStructural returns the structural key of the source fragment.
func (e *TMEntry) SourceStructural() string {
	if e.Source == nil {
		return ""
	}
	return e.Source.StructuralText()
}

// SourceGeneralized returns the generalized key of the source fragment.
func (e *TMEntry) SourceGeneralized() string {
	if e.Source == nil {
		return ""
	}
	return e.Source.GeneralizedText()
}

// EntityAdaptation describes how to substitute an entity value
// in the matched target to produce a translation for the current source.
type EntityAdaptation struct {
	PlaceholderID string           // which entity ("e1")
	Type          model.EntityType // person, product, etc.
	StoredValue   string           // value in the TM target ("Bob")
	CurrentValue  string           // value in the current source ("John")
	TargetPos     model.TextRange  // where to substitute in the target
}

// TMMatch represents a match result from a TM lookup.
type TMMatch struct {
	Entry             TMEntry
	Score             float64 // 0.0-1.0 (1.0 = exact match)
	MatchType         MatchType
	ProjectID         string // provenance: project ID of the matched entry
	EntityAdaptations []EntityAdaptation
}

// ProjectScope controls project filtering in TM lookups.
type ProjectScope int

const (
	ProjectScopeAll     ProjectScope = iota // workspace-wide, boost current project (default)
	ProjectScopeOnly                        // current project only
	ProjectScopeExclude                     // other projects only
)

// LookupOptions controls the behavior of TM lookups.
type LookupOptions struct {
	MinScore     float64      // minimum match score (default 0.7)
	MaxResults   int          // maximum results to return (default 10)
	MatchModes   []MatchMode  // which key types to use (default: all)
	ProjectID    string       // project context for scoring boost
	ProjectScope ProjectScope // project filtering mode (default: all)
}

// MatchMode controls which matching tiers to use.
type MatchMode string

const (
	MatchModeGeneralized MatchMode = "generalized" // entity-aware matching
	MatchModeStructural  MatchMode = "structural"  // inline-code-aware matching
	MatchModePlain       MatchMode = "plain"       // text-only matching
)

// DefaultLookupOptions returns sensible defaults for TM lookups.
func DefaultLookupOptions() LookupOptions {
	return LookupOptions{
		MinScore:   0.7,
		MaxResults: 10,
	}
}

// TMEntryGroup represents a source text with all its target translations.
type TMEntryGroup struct {
	SourceText   string
	SourceCoded  string // JSON-marshaled Fragment
	Source       *model.Fragment
	SourceLocale model.LocaleID
	Targets      []TMEntry
}

// FacetData contains aggregated facet counts for the TM sidebar.
type FacetData struct {
	LocalePairs []LocalePairStat
	Projects    []ProjectFacet
	EntityTypes []EntityTypeFacet
	HasCodes    int
	NoCodes     int
}

// ProjectFacet is a project ID with its entry count.
type ProjectFacet struct {
	ProjectID string `json:"project_id"`
	Count     int    `json:"count"`
}

// EntityTypeFacet is an entity type with its entry count.
type EntityTypeFacet struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// SearchFilter holds optional filter parameters for TM search.
type SearchFilter struct {
	ProjectID    string              // filter by project (empty = all)
	EntityTypes  []string            // filter by entity types (empty = all)
	EntityValues []EntityValueFilter // filter by specific entity value+type pairs (any match)
	HasCodes     *bool               // nil = all, true = only with codes, false = only without
}

// EntityValueFilter matches entries that have an entity mapping with the given
// value and type. Multiple filters are OR-ed (any match).
type EntityValueFilter struct {
	Value string // the entity's source_value, e.g. "Acme Corp"
	Type  string // entity:person, entity:organization, etc.
}

// TranslationMemory defines the interface for a content-aware translation
// memory store. Unlike traditional TMs that store plain strings, this
// interface works with the full content model (Fragments, Blocks, entities).
type TranslationMemory interface {
	// Add inserts or updates a TM entry with full Fragment representation.
	Add(entry TMEntry) error

	// Lookup searches for matches using tiered matching
	// (generalized → structural → plain). The source Block's entity
	// annotations are used to compute the generalized key.
	Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

	// LookupText searches for matches using plain text only.
	// This is a convenience method for cases where no Block is available.
	LookupText(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

	// Delete removes an entry by ID.
	Delete(id string) error

	// Count returns the total number of entries.
	Count() int

	// Close releases any resources held by the translation memory.
	Close() error
}
