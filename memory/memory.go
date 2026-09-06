package memory

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// MatchType indicates how a content-memory match was found, ordered by reuse potential.
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

// Origin records where a content-memory entry came from — file, tool, import, etc.
// An entry can have multiple origins if the same source text was ingested
// from multiple locations (common content memory deduplication case).
type Origin struct {
	Source    string // "file", "tool", "import", "user"
	Key       string // e.g. "errors.notFound" for keyed formats, or file path
	Reference string // optional: git commit, job ID, URL, crawl URL
	AddedAt   time.Time
	AddedBy   string // user ID or tool name
	SessionID string // FK to ImportSession.ID (empty for non-imported origins)
	// ContextFingerprint is the governing context in force when this answer was
	// produced — the same hash model.Origin.ContextFingerprint carries on a
	// target, so an answer absorbed into the corpus keeps the statement about
	// what governed it.
	//
	// It is what makes a prior answer judgeable rather than merely retrievable:
	// reuse is only safe under the rules the answer was approved under, and
	// nothing else here records them. Empty for an import, a seed, or a
	// producer that ran ungoverned.
	ContextFingerprint string
}

// EntityValue is a per-locale entity value with its position within the
// corresponding variant's fragment.
type EntityValue struct {
	Text  string
	Start int
	End   int
}

// EntityMapping tracks a named entity across all variants of a multilingual
// content-memory entry. Each placeholder has a Type and a map of locale → value.
// This enables generalized matching: entities are replaced with typed
// placeholders in the matching key, so structurally identical segments
// match regardless of entity values, and entity values can be adapted
// across languages.
//
// ConceptID optionally links to a terms store Concept, enabling cross-reference
// between content memory entities and terminology. When set, the UI can show the
// preferred translation from the terms store and flag consistency violations.
type EntityMapping struct {
	PlaceholderID string                         // "e1", "e2"
	Type          model.EntityType               // person, product, organization, date, etc.
	Values        map[model.LocaleID]EntityValue // per-locale value + position
	ConceptID     string                         // optional: terms.Concept.ID
}

// Pair returns the source and target entity values for a given language pair.
// It reports ok=false if either locale is missing from the mapping.
func (em *EntityMapping) Pair(src, tgt model.LocaleID) (EntityValue, EntityValue, bool) {
	sv, hasSrc := em.Values[model.NormalizeLocale(src)]
	tv, hasTgt := em.Values[model.NormalizeLocale(tgt)]
	if !hasSrc || !hasTgt {
		return EntityValue{}, EntityValue{}, false
	}
	return sv, tv, true
}

// Value returns the entity value for a specific locale, or false if missing.
func (em *EntityMapping) Value(locale model.LocaleID) (EntityValue, bool) {
	v, ok := em.Values[model.NormalizeLocale(locale)]
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

// Entry is a multilingual content memory entry. Each language
// variant is stored as a peer Run sequence in Variants; there is no
// authoritative "source" at the persistence layer. HintSrcLang records
// which locale the author treated as canonical (for example the TMX
// header's srclang, or the locale chosen by a translator adding a new
// entry) and is used for display and entity-direction purposes only.
type Entry struct {
	ID          string
	ProjectID   string
	Variants    map[model.LocaleID][]model.Run
	HintSrcLang model.LocaleID
	Entities    []EntityMapping
	Properties  map[string]string
	Origins     []Origin
	Note        string
	// Point is where this answer was approved: the context point on the
	// containment ladder (see point.go), empty for an entry bound to no
	// location — a seed, an import, an ad-hoc addition. It is what lets one
	// source string carry a different approved answer per collection, and what
	// a lookup measures nearness against.
	Point string
	// Unit is the durable block identity this answer was approved for
	// (model.Block.Unit) — matched by reconciliation rather than named, so it
	// survives an edit that rewrites the source and a reorder that moves it.
	//
	// It is what turns the corpus into a version chain. Entries accumulate: a
	// block whose source changes writes a new entry beside the old one, keyed
	// by the new text, and before this there was nothing to say the two were
	// successive answers for the same block rather than two unrelated strings.
	//
	// Empty for an entry bound to no block — a seed, an import, an ad-hoc
	// addition — and for everything approved before the chain existed. Nothing
	// backfills it; see the v5 migration.
	Unit      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Variant returns the Run sequence for a given locale, or nil if not
// present. The locale is matched in its canonical form, the form the store
// keys variants by, so the spelling a caller looks up with is not a second
// identity.
func (e *Entry) Variant(locale model.LocaleID) []model.Run {
	if e == nil || e.Variants == nil {
		return nil
	}
	return e.Variants[model.NormalizeLocale(locale)]
}

// VariantText returns the plain text of the variant for a given locale,
// or the empty string if the locale has no variant.
func (e *Entry) VariantText(locale model.LocaleID) string {
	runs := e.Variant(locale)
	if runs == nil {
		return ""
	}
	return model.FlattenRuns(runs)
}

// VariantStructural returns the structural key (inline codes preserved
// as placeholders) of the variant for a given locale.
func (e *Entry) VariantStructural(locale model.LocaleID) string {
	runs := e.Variant(locale)
	if runs == nil {
		return ""
	}
	return model.RunsStructuralText(runs)
}

// VariantGeneralized returns the generalized key (entities replaced with
// typed placeholders) of the variant for a given locale.
func (e *Entry) VariantGeneralized(locale model.LocaleID) string {
	runs := e.Variant(locale)
	if runs == nil {
		return ""
	}
	return model.RunsGeneralizedText(runs)
}

// HasLocale reports whether the entry has a variant for the given locale.
func (e *Entry) HasLocale(locale model.LocaleID) bool {
	if e == nil || e.Variants == nil {
		return false
	}
	_, ok := e.Variants[model.NormalizeLocale(locale)]
	return ok
}

// Locales returns the sorted list of locales for which the entry has
// variants. The empty locale (if present for any reason) is included.
func (e *Entry) Locales() []model.LocaleID {
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
	StoredValue   string           // value in the content memory target ("Bob")
	CurrentValue  string           // value in the current source ("John")
	TargetPos     model.TextRange  // where to substitute in the target
}

// Match represents a match result from a content-memory lookup. The Entry carries
// the full multilingual entry; the caller asked for a target in a specific
// locale, which is fetched via entry.Variant(tgtLocale) at the call site.
type Match struct {
	Entry             Entry
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
func demoteAmbiguousExacts(matches []Match, targetLocale model.LocaleID) {
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

// resolveNearestApproval settles a full-score disagreement by where each answer
// was approved: the approval nearest the point the caller is asking from is the
// one that governs there, and every other full-score answer demotes out of its
// way. It reports whether it settled anything.
//
// This is what stops one collection's reviewed wording from being answered by
// another's. Both answers are real translations approved by a reader, so no
// gate can tell them apart on quality; what tells them apart is that one of them
// was approved where the string being written actually sits.
//
// A caller with no point in hand settles nothing — nearness to nowhere is not a
// measurement — and the disagreement falls to the ambiguity rule, which is the
// honest answer for a reader who cannot say where they are asking from.
func resolveNearestApproval(matches []Match, targetLocale model.LocaleID, at string) bool {
	if at == "" {
		return false
	}
	best := -1
	targets := map[string]bool{}
	for i := range matches {
		if matches[i].Score < 1.0 {
			continue
		}
		text := matches[i].Entry.VariantText(targetLocale)
		targets[text] = true
		if best < 0 || NearerAnswer(matches[i].Entry.Point, text, matches[best].Entry.Point,
			matches[best].Entry.VariantText(targetLocale), at) {
			best = i
		}
	}
	if best < 0 || len(targets) <= 1 {
		return false
	}
	winner := matches[best].Entry.VariantText(targetLocale)
	for i := range matches {
		if matches[i].Score >= 1.0 && matches[i].Entry.VariantText(targetLocale) != winner {
			matches[i].Score = ScoreNearExact
		}
	}
	return true
}

// ProjectScope controls project filtering in content-memory lookups.
type ProjectScope int

const (
	ProjectScopeAll     ProjectScope = iota // workspace-wide, boost current project (default)
	ProjectScopeOnly                        // current project only
	ProjectScopeExclude                     // other projects only
)

// LookupOptions controls the behavior of content-memory lookups.
type LookupOptions struct {
	MinScore     float64      // minimum match score (default 0.7)
	MaxResults   int          // maximum results to return (default 10)
	MatchModes   []MatchMode  // which key types to use (default: all)
	ProjectID    string       // project context for scoring boost
	ProjectScope ProjectScope // project filtering mode (default: all)
	// Point is where the caller is asking from — the context point of the unit
	// about to be filled. When a source has more than one approved answer, the
	// approval nearest this point wins (resolveNearestApproval). Empty means
	// the caller has no location in hand, and a disagreement is then reported
	// as ambiguous rather than settled by a nearness nobody can measure.
	Point string
}

// MatchMode controls which matching tiers to use.
type MatchMode string

const (
	MatchModeGeneralized MatchMode = "generalized" // entity-aware matching
	MatchModeStructural  MatchMode = "structural"  // inline-code-aware matching
	MatchModePlain       MatchMode = "plain"       // text-only matching
)

// DefaultLookupOptions returns sensible defaults for content-memory lookups.
func DefaultLookupOptions() LookupOptions {
	return LookupOptions{
		MinScore:   0.7,
		MaxResults: 10,
	}
}

// FacetData contains aggregated facet counts for the content memory sidebar.
type FacetData struct {
	Locales        []LocaleFacet
	Projects       []ProjectFacet
	EntityTypes    []EntityTypeFacet
	ImportSessions []ImportSessionFacet
	HasCodes       int
	NoCodes        int
}

// LocaleFacet is a single-locale count across all entries in the content memory.
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

// SearchFilter holds optional filter parameters for content-memory search.
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

// SearchParams groups the parameters of the Store search and facet
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

// ContentMemory defines the interface for a content-aware content memory
// store. The interface is directional at the call site — callers ask "match
// this source in locale X and return the target in locale Y" — but entries
// themselves are multilingual peers.
type ContentMemory interface {
	// Add inserts or updates a content-memory entry with full Fragment representation.
	Add(ctx context.Context, entry Entry) error

	// Lookup searches for matches using tiered matching
	// (generalized → structural → plain). The source Block's entity
	// annotations are used to compute the generalized key.
	//
	// Matches are found among entries whose Variants[sourceLocale] exists
	// and matches the source. Returned Match.Entry.Variant(targetLocale)
	// is the translation. Entries lacking the target locale are skipped.
	//
	// Lookup keys on the block's *first* segment, which is correct when
	// segmentation is off (one segment per Block — the verbatim lookup
	// case). For sentence-level leverage when segmentation is on, use
	// LookupSegment.
	Lookup(ctx context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error)

	// LookupSegment searches for matches using a specific segment of the
	// source block, for sentence-level content-memory leverage when segmentation is
	// on. Returns nil, nil if segmentIdx is out of range or the segment
	// has no content. The block's entity annotations are reused so
	// generalized (entity-aware) matching still works inside a segment.
	LookupSegment(ctx context.Context, source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error)

	// LookupText searches for matches using plain text only.
	// This is a convenience method for cases where no Block is available.
	LookupText(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error)

	// Delete removes an entry by ID.
	Delete(ctx context.Context, id string) error

	// Count returns the total number of entries.
	Count(ctx context.Context) (int, error)

	// Close releases any resources held by the content memory.
	Close() error
}
