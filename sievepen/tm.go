package sievepen

import (
	"context"
	"maps"
	"slices"
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

// Origin records where a TM entry came from — file, tool, import, etc.
// An entry can have multiple origins if the same source text was ingested
// from multiple locations (common TM deduplication case).
type Origin struct {
	Source    string // "file", "tool", "import", "user"
	Key       string // e.g. "errors.notFound" for keyed formats, or file path
	Reference string // optional: git commit, job ID, URL, crawl URL
	AddedAt   time.Time
	AddedBy   string // user ID or tool name
	SessionID string // FK to ImportSession.ID (empty for non-imported origins)
}

// EntityValue is a per-locale entity value with its position within the
// corresponding variant's fragment.
type EntityValue struct {
	Text  string
	Start int
	End   int
}

// EntityMapping tracks a named entity across all variants of a multilingual
// TM entry. Each placeholder has a Type and a map of locale → value.
// This enables generalized matching: entities are replaced with typed
// placeholders in the matching key, so structurally identical segments
// match regardless of entity values, and entity values can be adapted
// across languages.
//
// ConceptID optionally links to a termbase Concept, enabling cross-reference
// between TM entities and terminology. When set, the UI can show the
// preferred translation from the termbase and flag consistency violations.
type EntityMapping struct {
	PlaceholderID string                         // "e1", "e2"
	Type          model.EntityType               // person, product, organization, date, etc.
	Values        map[model.LocaleID]EntityValue // per-locale value + position
	ConceptID     string                         // optional: termbase.Concept.ID
}

// Pair returns the source and target entity values for a given language pair.
// It reports ok=false if either locale is missing from the mapping.
func (em *EntityMapping) Pair(src, tgt model.LocaleID) (EntityValue, EntityValue, bool) {
	sv, hasSrc := em.Values[src]
	tv, hasTgt := em.Values[tgt]
	if !hasSrc || !hasTgt {
		return EntityValue{}, EntityValue{}, false
	}
	return sv, tv, true
}

// Value returns the entity value for a specific locale, or false if missing.
func (em *EntityMapping) Value(locale model.LocaleID) (EntityValue, bool) {
	v, ok := em.Values[locale]
	return v, ok
}

// ImportSession records per-file metadata captured once at import time.
// Entries imported from the same file share a single ImportSession row;
// each Origin row carries the SessionID instead of duplicating the
// header metadata.
type ImportSession struct {
	ID               string
	FileKey          string // filename or user-friendly label
	FileHash         string // sha256 hex of the raw file bytes
	FileSizeBytes    int64
	ImportedAt       time.Time
	ImportedBy       string
	ToolName         string // TMX <header creationtool>
	ToolVersion      string // TMX <header creationtoolversion>
	SegType          string // sentence, paragraph, phrase, block
	AdminLang        string
	SrcLang          string // TMX header's default source language
	DataType         string // PlainText, html, xml, etc.
	OriginalFormat   string // TMX <header o-tmf>
	OriginalEncoding string // TMX <header o-encoding>
	EntryCount       int    // number of TUs imported in this session
	Properties       map[string]string
}

// TMEntry is a multilingual translation memory entry. Each language
// variant is stored as a peer Run sequence in Variants; there is no
// authoritative "source" at the persistence layer. HintSrcLang records
// which locale the author treated as canonical (for example the TMX
// header's srclang, or the locale chosen by a translator adding a new
// entry) and is used for display and entity-direction purposes only.
type TMEntry struct {
	ID          string
	ProjectID   string
	Variants    map[model.LocaleID][]model.Run
	HintSrcLang model.LocaleID
	Entities    []EntityMapping
	Properties  map[string]string
	Origins     []Origin
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Variant returns the Run sequence for a given locale, or nil if not
// present.
func (e *TMEntry) Variant(locale model.LocaleID) []model.Run {
	if e == nil || e.Variants == nil {
		return nil
	}
	return e.Variants[locale]
}

// VariantText returns the plain text of the variant for a given locale,
// or the empty string if the locale has no variant.
func (e *TMEntry) VariantText(locale model.LocaleID) string {
	runs := e.Variant(locale)
	if runs == nil {
		return ""
	}
	return model.FlattenRuns(runs)
}

// VariantStructural returns the structural key (inline codes preserved
// as placeholders) of the variant for a given locale.
func (e *TMEntry) VariantStructural(locale model.LocaleID) string {
	runs := e.Variant(locale)
	if runs == nil {
		return ""
	}
	return model.RunsStructuralText(runs)
}

// VariantGeneralized returns the generalized key (entities replaced with
// typed placeholders) of the variant for a given locale.
func (e *TMEntry) VariantGeneralized(locale model.LocaleID) string {
	runs := e.Variant(locale)
	if runs == nil {
		return ""
	}
	return model.RunsGeneralizedText(runs)
}

// HasLocale reports whether the entry has a variant for the given locale.
func (e *TMEntry) HasLocale(locale model.LocaleID) bool {
	if e == nil || e.Variants == nil {
		return false
	}
	_, ok := e.Variants[locale]
	return ok
}

// Locales returns the sorted list of locales for which the entry has
// variants. The empty locale (if present for any reason) is included.
func (e *TMEntry) Locales() []model.LocaleID {
	if e == nil || len(e.Variants) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(e.Variants))
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

// TMMatch represents a match result from a TM lookup. The Entry carries
// the full multilingual entry; the caller asked for a target in a specific
// locale, which is fetched via entry.Variant(tgtLocale) at the call site.
type TMMatch struct {
	Entry             TMEntry
	Score             float64 // 0.0-1.0 (1.0 = exact match)
	MatchType         MatchType
	ProjectID         string // provenance: project ID of the matched entry
	EntityAdaptations []EntityAdaptation

	// Ambiguous marks an exact text match that full-score policies must
	// not trust blindly: several entries matched the query exactly but
	// disagree on the target text. Such matches are demoted to
	// ScoreNearExact and flagged so unattended fills (extract pre-fill,
	// fillTargetThreshold=100 leverage) can leave them for review instead
	// of silently picking one by storage order.
	Ambiguous bool
}

// ScoreNearExact is the score for exact-text matches that fall short of a
// fully trustworthy 100%: a plain-text match whose inline-code structure
// differs from the query's (industry "tag mismatch" penalty), or an exact
// match that is ambiguous (multiple distinct targets at full score).
const ScoreNearExact = 0.99

// demoteAmbiguousExacts applies the ambiguity rule over the exact-tier
// matches: when more than one match sits at score 1.0 with DIFFERING
// target texts, none of them can claim to be THE exact translation, so
// all are demoted to ScoreNearExact and flagged. Identical targets at
// 1.0 are fine (the pick doesn't matter) and keep their score.
func demoteAmbiguousExacts(matches []TMMatch, targetLocale model.LocaleID) {
	targets := map[string]bool{}
	for i := range matches {
		if matches[i].Score >= 1.0 {
			targets[matches[i].Entry.VariantText(targetLocale)] = true
		}
	}
	if len(targets) <= 1 {
		return
	}
	for i := range matches {
		if matches[i].Score >= 1.0 {
			matches[i].Score = ScoreNearExact
			matches[i].Ambiguous = true
		}
	}
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

// FacetData contains aggregated facet counts for the TM sidebar.
type FacetData struct {
	Locales        []LocaleFacet
	Projects       []ProjectFacet
	EntityTypes    []EntityTypeFacet
	ImportSessions []ImportSessionFacet
	HasCodes       int
	NoCodes        int
}

// LocaleFacet is a single-locale count across all entries in the TM.
// An entry with N variants contributes to N LocaleFacet counts.
type LocaleFacet struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
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

// ImportSessionFacet exposes an import session as a facet option so the
// UI can scope the browse view to "entries imported from this file".
type ImportSessionFacet struct {
	SessionID  string    `json:"session_id"`
	FileKey    string    `json:"file_key"`
	ToolName   string    `json:"tool_name,omitempty"`
	ImportedAt time.Time `json:"imported_at"`
	Count      int       `json:"count"`
}

// ActivityStat is a daily entry-creation count for the activity sparkline.
type ActivityStat struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"count"`
}

// SearchFilter holds optional filter parameters for TM search.
type SearchFilter struct {
	ProjectID    string              // filter by project (empty = all)
	SessionIDs   []string            // filter to entries imported in these sessions (empty = all)
	EntityTypes  []string            // filter by entity types (empty = all)
	EntityValues []EntityValueFilter // filter by specific entity value+type pairs (OR-matched)
	HasCodes     *bool               // nil = all, true = only with codes, false = only without
}

// EntityValueFilter matches entries that have an entity mapping with the
// given source value and type. Multiple filters are OR-ed (any match).
// Matching is performed across all locale variants of the entity.
type EntityValueFilter struct {
	Value string // the entity's value, e.g. "Acme Corp"
	Type  string // entity:person, entity:organization, etc.
}

// SearchParams groups the parameters of the TMStore search and facet
// methods into a single struct. This replaces the previous four-adjacent
// string signature (query, anyLocale, requireLocale, stream) so callers
// can no longer transpose query and locale arguments at the call site.
//
// Field semantics:
//   - Query:        text search query (empty = no text filter)
//   - AnyLocale:    require matches in this locale's variant (empty = any locale)
//   - RequireLocale: additionally require the entry to have this locale variant
//     (empty = no additional requirement)
//   - Stream:       the originating stream (e.g. a git branch name), for the
//     stream-inheritance variant
//   - StreamChain:  ordered ancestor streams to search; earlier streams win
//   - Filter:       optional facet filter (used by the *Filtered variants)
//   - Offset/Limit: pagination
//
// Callers doing bilingual browsing typically set (AnyLocale: src, RequireLocale: tgt);
// monolingual browsing sets (AnyLocale: locale); fully open search leaves both empty.
type SearchParams struct {
	Query         string
	AnyLocale     string
	RequireLocale string
	Stream        string
	StreamChain   []string
	Filter        SearchFilter
	Offset        int
	Limit         int
}

// TranslationMemory defines the interface for a content-aware translation
// memory store. The interface is directional at the call site — callers
// ask "match this source in locale X and return the target in locale Y"
// — but entries themselves are multilingual peers.
type TranslationMemory interface {
	// Add inserts or updates a TM entry with full Fragment representation.
	Add(ctx context.Context, entry TMEntry) error

	// Lookup searches for matches using tiered matching
	// (generalized → structural → plain). The source Block's entity
	// annotations are used to compute the generalized key.
	//
	// Matches are found among entries whose Variants[sourceLocale] exists
	// and matches the source. Returned TMMatch.Entry.Variant(targetLocale)
	// is the translation. Entries lacking the target locale are skipped.
	//
	// Lookup keys on the block's *first* segment, which is correct when
	// segmentation is off (one segment per Block — the verbatim lookup
	// case). For sentence-level leverage when segmentation is on, use
	// LookupSegment.
	Lookup(ctx context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

	// LookupSegment searches for matches using a specific segment of the
	// source block, for sentence-level TM leverage when segmentation is
	// on. Returns nil, nil if segmentIdx is out of range or the segment
	// has no content. The block's entity annotations are reused so
	// generalized (entity-aware) matching still works inside a segment.
	LookupSegment(ctx context.Context, source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

	// LookupText searches for matches using plain text only.
	// This is a convenience method for cases where no Block is available.
	LookupText(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

	// Delete removes an entry by ID.
	Delete(ctx context.Context, id string) error

	// Count returns the total number of entries.
	Count(ctx context.Context) (int, error)

	// Close releases any resources held by the translation memory.
	Close() error
}
