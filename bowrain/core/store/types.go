// Package store defines the ContentStore interface and domain types
// for versioned content persistence.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

const (
	// MaxBlocksPerRequest limits the number of blocks in a single sync push request.
	MaxBlocksPerRequest = 10000

	// DefaultBlockLimit is the default limit for block queries.
	DefaultBlockLimit = 10000
)

// Project represents a localization project in the store.
type Project struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	DefaultSourceLanguage model.LocaleID    `json:"default_source_language"`
	TargetLanguages       []model.LocaleID  `json:"target_languages"`
	TargetLanguageMode    string            `json:"target_language_mode"`
	DefaultStream         string            `json:"default_stream,omitempty"`
	DashboardVisibility   string            `json:"dashboard_visibility"`
	Properties            map[string]string `json:"properties,omitempty"`
	WorkspaceID           string            `json:"workspace_id,omitempty"`
	// ConvergePolicy is the server-side continuous-convergence policy
	// (on-push | manual): whether a completed push starts a convergence run on
	// the server's own clock. Empty is treated as on-push (the connected default).
	ConvergePolicy string     `json:"converge_policy,omitempty"`
	Archived       bool       `json:"archived"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Convergence-policy values for a server-side project (Project.ConvergePolicy).
const (
	ConvergePolicyOnPush = "on-push"
	ConvergePolicyManual = "manual"
)

// NormalizeConvergePolicy defaults an empty or unrecognized policy to on-push
// (the connected-project default), so stored rows always carry a canonical
// value.
func NormalizeConvergePolicy(v string) string {
	if v == ConvergePolicyManual {
		return ConvergePolicyManual
	}
	return ConvergePolicyOnPush
}

// Item represents a file or data object within a project.
type Item struct {
	ID           string            `json:"id"`
	ProjectID    string            `json:"project_id"`
	Name         string            `json:"name"`
	Format       string            `json:"format"`
	ItemType     string            `json:"item_type"`
	CollectionID string            `json:"collection_id,omitempty"`
	BlockIndex   string            `json:"block_index"`
	PreviewHTML  string            `json:"preview_html,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// StoredBlock wraps a model.Block with store metadata.
type StoredBlock struct {
	*model.Block
	ProjectID   string
	ItemName    string
	SourceID    string // format-reader-assigned ID (e.g., "tu1"); empty for blocks stored without an item
	ContentHash string
	ContextHash string
	StoredAt    time.Time
	UpdatedAt   time.Time
}

// BlockQuery filters blocks when listing or searching.
type BlockQuery struct {
	ProjectID     string
	Stream        string          // Stream name (empty defaults to "main")
	ItemName      string          // Filter by item name
	IDs           []string        // Filter by block IDs
	ContentHash   string          // Filter by content hash
	Translatable  *bool           // Filter by translatable flag
	HasTarget     *model.LocaleID // Filter blocks that have a target for this locale
	MissingTarget *model.LocaleID // Filter blocks missing a target for this locale

	// TargetLocale scopes the per-locale filters: the Status bucket and the
	// target side of Text. It names a locale-only target variant ("fr"), the
	// variant the editor writes. Empty leaves both filters source-only.
	TargetLocale string
	// Status keeps only blocks whose TargetLocale target sits in one bucket —
	// BlockStatusNotStarted, BlockStatusDraft, BlockStatusTranslated or
	// BlockStatusReviewed. Empty keeps every bucket.
	Status string
	// Text keeps only blocks whose source text, or whose TargetLocale target
	// text, contains it case-insensitively. Source matching is per run, so a
	// substring spanning an inline-code boundary does not match.
	Text string

	Limit  int // Max results (0 = no limit)
	Offset int // Pagination offset
}

// The per-locale status buckets a block falls into, as the editor names them.
// They partition the translatable blocks of a query: every block sits in
// exactly one for a given locale.
const (
	// BlockStatusNotStarted is a locale with no target text.
	BlockStatusNotStarted = "not-started"
	// BlockStatusDraft is unrevised content: a draft rung, the legacy
	// block-global draft flag, or machine/pseudo provenance with no rung.
	BlockStatusDraft = "draft"
	// BlockStatusTranslated is committed content awaiting review.
	BlockStatusTranslated = "translated"
	// BlockStatusReviewed covers both reviewed and signed-off.
	BlockStatusReviewed = "reviewed"
)

// BlockStatusBuckets lists the buckets in ladder order.
func BlockStatusBuckets() []string {
	return []string{BlockStatusNotStarted, BlockStatusDraft, BlockStatusTranslated, BlockStatusReviewed}
}

// BlockCounts summarizes a BlockQuery for one locale without hydrating a
// single block. Total and Translatable count the blocks the query's
// non-status filters keep; the four buckets partition Translatable by the
// query's TargetLocale, so they sum to it.
type BlockCounts struct {
	Total        int
	Translatable int
	NotStarted   int
	Draft        int
	Translated   int
	Reviewed     int
}

// PendingReviewRef names one (block, locale) pair awaiting review.
type PendingReviewRef struct {
	BlockID  string `json:"block_id"`
	ItemName string `json:"item_name"`
	Locale   string `json:"locale"`
}

// Version represents a named snapshot of project state.
type Version struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	BlockCount  int       `json:"block_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// VersionDiff describes the differences between two versions.
type VersionDiff struct {
	FromVersion string
	ToVersion   string
	Changes     []BlockChange
}

// ChangeType describes what changed between versions.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeRemoved  ChangeType = "removed"
	ChangeModified ChangeType = "modified"
)

// BlockChange describes a single block change between versions.
type BlockChange struct {
	BlockID    string
	ChangeType ChangeType
	OldHash    string // Empty for added blocks
	NewHash    string // Empty for removed blocks
}

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

// CollectionKind controls how a collection is populated.
type CollectionKind string

const (
	// CollectionUploaded allows ad-hoc file uploads and manual item creation.
	CollectionUploaded CollectionKind = "uploaded"
	// CollectionConnected is linked to integration connectors; no manual upload.
	CollectionConnected CollectionKind = "connected"
)

// Collection groups items within a project.
// Collections are project-scoped by default. When Stream is non-empty,
// the collection is visible only within that stream.
//
// A collection reached the server one of two ways, and Owner says which: the
// web hub, the editor or a connector created it (workspace-owned), or a
// project's recipe declared it and a push carried it up (recipe-owned). The
// distinction is not cosmetic — it decides who may change the row.
type Collection struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id"`
	Name            string            `json:"name"`
	Kind            CollectionKind    `json:"kind"`
	ItemLabel       string            `json:"item_label"` // e.g. "item", "page", "post", "document"
	IsDefault       bool              `json:"is_default"`
	Stream          string            `json:"stream,omitempty"`           // empty = project-wide
	ConnectorConfig map[string]string `json:"connector_config,omitempty"` // connector type + settings

	// Context is the point this collection's content occupies in the project's
	// context space: axis → value, as the recipe declares under `context:`.
	// Empty for a workspace-owned collection, which sits at no declared point.
	Context map[string]string `json:"context,omitempty"`

	// Owner is "recipe" or "workspace" (bowrain/core/sync.ContextOwner*). It
	// arrives as a protocol field on the pushed context entry and is stored
	// verbatim: every later ownership decision is a lookup of this field, not a
	// rule reconstructed from how the row happens to look.
	Owner string `json:"owner,omitempty"`

	// ContextHash is the hash of the context entry this row was reconciled
	// from (bowrain/core/sync.ComputeContextEntryHash). It is what makes a
	// re-push idempotent: an entry arriving with an unchanged hash leaves the
	// row — and its UpdatedAt — untouched.
	ContextHash string `json:"context_hash,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Streams
// ---------------------------------------------------------------------------

// StreamVisibility controls who can see and access a stream.
type StreamVisibility string

const (
	StreamPublic  StreamVisibility = "public"  // visible to all workspace members
	StreamPrivate StreamVisibility = "private" // visible only to creator
	StreamShared  StreamVisibility = "shared"  // visible to creator + explicit members
)

// Stream represents a named branch of content within a project.
// Every project has an implicit "main" stream. Additional streams branch
// from a parent stream at a specific cursor position (copy-on-write).
type Stream struct {
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`        // "main", "v2.0", "feature/new-ui", "pr/142"
	Parent      string            `json:"parent"`      // parent stream name; empty for "main"
	BaseCursor  int64             `json:"base_cursor"` // cursor in parent at branch point
	Archived    bool              `json:"archived"`
	Locked      bool              `json:"locked"`                // when true, no further content changes are allowed
	LockedBy    string            `json:"locked_by,omitempty"`   // user who locked the stream
	LockedAt    *time.Time        `json:"locked_at,omitempty"`   // when the stream was locked
	Visibility  StreamVisibility  `json:"visibility"`            // "public", "private", "shared"
	Description string            `json:"description"`           // human-readable purpose
	SharedWith  []string          `json:"shared_with,omitempty"` // user IDs (only for "shared" visibility)
	Properties  map[string]string `json:"properties,omitempty"`  // extensible metadata (brand voice bindings, etc.)
	CreatedAt   time.Time         `json:"created_at"`
	CreatedBy   string            `json:"created_by"`
}

// ---------------------------------------------------------------------------
// Stream Tags
// ---------------------------------------------------------------------------

// StreamTagKind classifies stream tags.
type StreamTagKind string

const (
	TagKindMerge     StreamTagKind = "merge"
	TagKindRelease   StreamTagKind = "release"
	TagKindMilestone StreamTagKind = "milestone"
	TagKindCustom    StreamTagKind = "custom"
)

// StreamTag is an immutable marker pinned to a point in a stream's change log.
type StreamTag struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"project_id"`
	Stream    string            `json:"stream"`
	Name      string            `json:"name"`
	Kind      StreamTagKind     `json:"kind"`
	Cursor    int64             `json:"cursor"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedBy string            `json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
}

// ErrStreamLocked is returned when a write operation is attempted on a locked stream.
var ErrStreamLocked = errors.New("stream is locked")

// MergeOptions controls stream merge behavior.
type MergeOptions struct {
	// DryRun when true returns the diff without applying changes.
	DryRun bool
}

// MergeResult describes the outcome of a stream merge.
type MergeResult struct {
	MergedBlocks   int           `json:"merged_blocks"`
	AddedBlocks    int           `json:"added_blocks"`
	ModifiedBlocks int           `json:"modified_blocks"`
	RemovedBlocks  int           `json:"removed_blocks"`
	Changes        []BlockChange `json:"changes,omitempty"`
}

// StreamDiff describes the differences between a stream and its parent.
type StreamDiff struct {
	StreamName string        `json:"stream_name"`
	ParentName string        `json:"parent_name"`
	Changes    []BlockChange `json:"changes"`
}

// ---------------------------------------------------------------------------
// Change Log (incremental sync)
// ---------------------------------------------------------------------------

// BlockHistoryEntry represents a single historical change to a block's translation.
// JSON `codedText` is preserved on the wire for client compatibility; the Go field
// is named `Coded` to stay clear of the RFC 0001 Phase 2 acceptance grep.
type BlockHistoryEntry struct {
	Seq           int64     `json:"seq"`
	ChangeType    string    `json:"changeType"`
	Text          string    `json:"text"`
	Coded         string    `json:"codedText"`
	Origin        string    `json:"origin"`
	Author        string    `json:"author"`
	ActorRole     string    `json:"actorRole,omitempty"`
	EditReason    string    `json:"editReason,omitempty"`
	CorrelationID string    `json:"correlationId,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// ChangeEntry represents a single entry in the append-only change log.
type ChangeEntry struct {
	Seq         int64     `json:"seq"`
	BlockID     string    `json:"block_id"`
	ChangeType  string    `json:"change_type"`      // source_added, source_modified, source_removed, target_added, target_modified
	Locale      string    `json:"locale,omitempty"` // Empty for source changes
	ContentHash string    `json:"content_hash,omitempty"`
	LoggedAt    time.Time `json:"logged_at"`
}

// ChangeSet is the result of a GetChanges query.
type ChangeSet struct {
	Changes   []ChangeEntry `json:"changes"`
	NewCursor int64         `json:"new_cursor"` // Latest seq in this batch
	HasMore   bool          `json:"has_more"`   // True if more changes exist beyond this batch
}

// ---------------------------------------------------------------------------
// Assets (binary media content — Bowrain AD-007)
// ---------------------------------------------------------------------------

// Asset represents a binary asset (image, audio, video) stored in BlobStore
// with metadata tracked in the ContentStore.
type Asset struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"project_id"`
	ItemName         string            `json:"item_name"` // source file this asset belongs to
	SourceID         string            `json:"source_id"` // format-reader-assigned ID within the item
	BlobKey          string            `json:"blob_key"`  // content-addressed key in BlobStore
	MimeType         string            `json:"mime_type"`
	Filename         string            `json:"filename"` // original filename
	SizeBytes        int64             `json:"size_bytes"`
	AltText          string            `json:"alt_text"`          // extractable localized text
	Properties       map[string]string `json:"properties"`        // dimensions, duration, codec, etc.
	ProcessingStatus string            `json:"processing_status"` // none, pending, processing, processed, failed
	ProcessingHint   string            `json:"processing_hint"`   // ocr, chart-text, subtitle-extract, asr
	Stream           string            `json:"stream,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// AssetVariant represents a locale-specific variant of an asset.
type AssetVariant struct {
	AssetID    string            `json:"asset_id"`
	Locale     string            `json:"locale"`   // BCP-47 tag
	BlobKey    string            `json:"blob_key"` // locale-specific binary in BlobStore
	Status     string            `json:"status"`   // pending, draft, approved
	MimeType   string            `json:"mime_type"`
	SizeBytes  int64             `json:"size_bytes"`
	Properties map[string]string `json:"properties"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Block Statistics (lightweight projection for dashboard queries)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Pulse Dashboard Types (public activity dashboard — Bowrain AD-017)
// ---------------------------------------------------------------------------

// PulseOverview is the top-level response for a workspace's public dashboard.
type PulseOverview struct {
	Workspace      PulseWorkspaceInfo    `json:"workspace"`
	Projects       []PulseProjectSummary `json:"projects"`
	TopLanguages   []PulseLanguageRank   `json:"top_languages"`
	TopContribs    []PulseContributor    `json:"top_contributors"`
	RisingStars    []PulseRisingStar     `json:"rising_stars"`
	RecentActivity []PulseActivity       `json:"recent_activity"`
	Stats          PulseGlobalStats      `json:"stats"`
}

// PulseWorkspaceInfo is the public-facing workspace info for Pulse.
type PulseWorkspaceInfo struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`
}

// PulseGlobalStats holds aggregate statistics for a workspace.
type PulseGlobalStats struct {
	TotalProjects     int     `json:"total_projects"`
	TotalLanguages    int     `json:"total_languages"`
	TotalContributors int     `json:"total_contributors"`
	TotalWords        int     `json:"total_words"`
	TranslatedWords   int     `json:"translated_words"`
	OverallPercent    float64 `json:"overall_percent"`
}

// PulseProjectSummary is a compact project summary for the overview grid.
type PulseProjectSummary struct {
	ID                        string                   `json:"id"`
	Name                      string                   `json:"name"`
	SourceLanguage            string                   `json:"source_language"`
	SourceLanguageDisplayName string                   `json:"source_language_display_name,omitempty"`
	TargetLanguages           []string                 `json:"target_languages"`
	TargetLanguageNames       map[string]string        `json:"target_language_names,omitempty"`
	TotalWords                int                      `json:"total_words"`
	TranslatedWords           int                      `json:"translated_words"`
	Percentage                float64                  `json:"percentage"`
	Locales                   []LocaleTranslationStats `json:"locales"`
}

// PulseLanguageRank ranks a language by translation progress.
type PulseLanguageRank struct {
	Locale          string  `json:"locale"`
	DisplayName     string  `json:"display_name,omitempty"`
	TranslatedWords int     `json:"translated_words"`
	TotalWords      int     `json:"total_words"`
	Percentage      float64 `json:"percentage"`
	Contributors    int     `json:"contributors"`
	RecentActivity  int     `json:"recent_activity"`
}

// PulseContributor represents a contributor on the leaderboard.
type PulseContributor struct {
	Name         string   `json:"name"`
	AvatarURL    string   `json:"avatar_url,omitempty"`
	Translations int      `json:"translations"`
	Reviews      int      `json:"reviews"`
	Languages    []string `json:"languages"`
}

// PulseRisingStar highlights a fast-growing contributor, language, or project.
type PulseRisingStar struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"` // "user", "language", "project"
	Growth   float64 `json:"growth"`
	Current  int     `json:"current"`
	Previous int     `json:"previous"`
}

// PulseActivity is a single activity entry for the public feed.
type PulseActivity struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Actor     string    `json:"actor"`
	AvatarURL string    `json:"avatar_url,omitempty"`
	Project   string    `json:"project"`
	Locale    string    `json:"locale,omitempty"`
	Summary   string    `json:"summary"`
	Timestamp time.Time `json:"timestamp"`
}

// PulseProjectDetail is the detailed response for a single project.
type PulseProjectDetail struct {
	Project PulseProjectSummary      `json:"project"`
	Locales []LocaleTranslationStats `json:"locales"`
	Items   []ItemTranslationStats   `json:"items"`
}

// PulseLocaleDetail is the detailed response for a single locale within a project.
type PulseLocaleDetail struct {
	Locale string                 `json:"locale"`
	Stats  LocaleTranslationStats `json:"stats"`
	Items  []ItemTranslationStats `json:"items"`
}

// PulseTermEntry is a terminology entry for the public explorer.
type PulseTermEntry struct {
	ID           string            `json:"id"`
	Term         string            `json:"term"`
	Definition   string            `json:"definition"`
	Domain       string            `json:"domain,omitempty"`
	Locale       string            `json:"locale"`
	Translations map[string]string `json:"translations,omitempty"`
}

// PulseLeaderboard is the response for the leaderboard page.
type PulseLeaderboard struct {
	Contributors []PulseContributor  `json:"contributors"`
	Languages    []PulseLanguageRank `json:"languages"`
}

// PulseHeatmapDay is a single day's activity count for the contribution heatmap.
type PulseHeatmapDay struct {
	Date  string `json:"date"` // "2026-01-15"
	Count int    `json:"count"`
}

// BlockStatRow is a lightweight projection of a block for dashboard aggregation.
// It avoids full deserialization of source segments, target segments, properties,
// and annotations — only the fields needed for word/block counting are included.
type BlockStatRow struct {
	ItemName      string   // which item (file) this block belongs to
	Translatable  bool     // whether the block is translatable
	SourceWords   int      // word count from source text
	TargetLocales []string // locales that have non-empty target translations
	// ApprovedLocales is the subset of TargetLocales whose stored target carries
	// a review decision (Target.Status at reviewed or above on the lifecycle
	// ladder). Used to derive per-locale ship states without deserializing runs.
	ApprovedLocales []string
}

// ---------------------------------------------------------------------------
// Translation Dashboard Statistics
// ---------------------------------------------------------------------------

// TranslationDashboardStats holds aggregated translation metrics for a project.
//
// ItemStats may be a page rather than the full per-file list: the dashboard
// endpoint accepts limit/offset (+ sort/dir) query parameters and slices the
// list server-side. ItemTotal always carries the full item count so paged
// consumers can render an honest "N of M" without fetching everything; without
// a limit the endpoint returns every item and ItemTotal == len(ItemStats).
type TranslationDashboardStats struct {
	LocaleStats        []LocaleTranslationStats     `json:"locale_stats"`
	ItemStats          []ItemTranslationStats       `json:"item_stats"`
	ItemTotal          int                          `json:"item_total"`
	CollectionStats    []CollectionTranslationStats `json:"collection_stats"`
	TotalBlocks        int                          `json:"total_blocks"`
	TranslatableBlocks int                          `json:"translatable_blocks"`
	TotalSourceWords   int                          `json:"total_source_words"`
}

// LocaleTranslationStats holds translation progress for a single target locale.
//
// ApprovedBlocks, FailingChecks, and ShipState are additive extensions: older
// consumers ignore them, and producers that do not compute checks (pulse, the
// convergence derive path) leave FailingChecks at 0 and ShipState empty (the
// field is then omitted from JSON).
type LocaleTranslationStats struct {
	Locale           string  `json:"locale"`
	DisplayName      string  `json:"display_name,omitempty"`
	TranslatedBlocks int     `json:"translated_blocks"`
	TotalBlocks      int     `json:"total_blocks"`
	TranslatedWords  int     `json:"translated_words"`
	TotalWords       int     `json:"total_words"`
	Percentage       float64 `json:"percentage"`
	// ApprovedBlocks counts translatable blocks whose target for this locale
	// carries a review decision (Target.Status reviewed or signed-off).
	ApprovedBlocks int `json:"approved_blocks"`
	// FailingChecks counts translated blocks whose target for this locale fails
	// the project's ship gate — a QA check with error severity, OR a terminology
	// violation (a forbidden/competitor term used, or a mandated preferred/
	// approved rendering missing). Only computed for locales at full coverage in
	// some scope (checks cannot promote an under-covered locale, so the expensive
	// pass is skipped below the coverage gate).
	FailingChecks int `json:"failing_checks"`
	// ShipState is the derived per-locale ship state (see DeriveShipState).
	// Empty when the producer did not derive it.
	ShipState ShipState `json:"ship_state,omitempty"`
	// OnBrandBlocks counts translated blocks that pass the project's QA checks
	// with no error-severity finding, are term-compliant for the locale (no
	// forbidden/competitor term, no missing mandated rendering), AND — where a
	// persisted brand voice score exists for the block+locale — carry a score at
	// or above the scoring profile's minimum bar. Additive: producers that do not
	// derive the on-brand rate leave it 0 (omitted from JSON).
	OnBrandBlocks int `json:"on_brand_blocks,omitempty"`
	// OnBrandRate is OnBrandBlocks / TranslatedBlocks, in [0,1]. Nil when the
	// producer did not derive it or the scope has no translated blocks.
	OnBrandRate *float64 `json:"on_brand_rate,omitempty"`
	// OnBrandBasis states what informed OnBrandRate (see OnBrandBasisFor): QA
	// checks always, plus "+terms" when term governance was active for the scope
	// and plus "voice" when at least one block's persisted voice score also
	// informed it. Empty when the rate was not derived — consumers hide the
	// metric then.
	OnBrandBasis OnBrandBasis `json:"on_brand_basis,omitempty"`
}

// OnBrandBasis names the evidence behind a derived on-brand rate, so consumers
// can present the number honestly: a checks-only rate says nothing about voice.
// QA checks always inform the rate; terms and voice are added when they were
// actually applied to the scope (term governance active / a persisted voice
// score present), so the basis never claims evidence that did not contribute.
type OnBrandBasis string

const (
	// OnBrandBasisChecks — the rate reflects QA check results only; no term
	// governance was active and no brand voice scores existed for the scope.
	OnBrandBasisChecks OnBrandBasis = "checks"
	// OnBrandBasisChecksTerms — QA checks plus deterministic terminology
	// compliance (forbidden/competitor presence, mandated-rendering absence).
	OnBrandBasisChecksTerms OnBrandBasis = "checks+terms"
	// OnBrandBasisVoice — QA checks plus persisted brand voice scores measured
	// against the scoring profile's minimum bar.
	OnBrandBasisVoice OnBrandBasis = "voice+checks"
	// OnBrandBasisVoiceTerms — QA checks plus terminology compliance plus
	// persisted brand voice scores: the fullest basis.
	OnBrandBasisVoiceTerms OnBrandBasis = "voice+checks+terms"
)

// OnBrandBasisFor names the evidence behind a derived on-brand rate from whether
// a persisted voice score and active term governance informed it. QA checks are
// always part of the basis; terms and voice are added when they contributed.
func OnBrandBasisFor(voice, terms bool) OnBrandBasis {
	switch {
	case voice && terms:
		return OnBrandBasisVoiceTerms
	case voice:
		return OnBrandBasisVoice
	case terms:
		return OnBrandBasisChecksTerms
	default:
		return OnBrandBasisChecks
	}
}

// ItemTranslationStats holds per-file translation progress.
type ItemTranslationStats struct {
	ItemName     string                   `json:"item_name"`
	ItemID       string                   `json:"item_id"`
	Format       string                   `json:"format"`
	CollectionID string                   `json:"collection_id"`
	BlockCount   int                      `json:"block_count"`
	WordCount    int                      `json:"word_count"`
	Locales      []LocaleTranslationStats `json:"locales"`
}

// CollectionTranslationStats holds per-collection translation progress.
type CollectionTranslationStats struct {
	CollectionID   string                   `json:"collection_id"`
	CollectionName string                   `json:"collection_name"`
	ItemCount      int                      `json:"item_count"`
	BlockCount     int                      `json:"block_count"`
	WordCount      int                      `json:"word_count"`
	Locales        []LocaleTranslationStats `json:"locales"`
}

// UnitDecision is the server's record of one workflow decision for one unit in
// one locale variant — the wire form of core/state.UnitState plus the item that
// scopes the unit's durable identity (the same structural name recurs across
// items, so an unscoped unit key cannot be joined safely). It records the
// decision as a FACT — who, when, which rung, and the hash of the translation
// it blesses. Freshness is derived by whoever reads it, never stored.
type UnitDecision struct {
	ProjectID string `json:"project_id,omitempty"`
	Stream    string `json:"stream,omitempty"`
	// ItemName is the item whose durable identity namespace Unit lives in.
	// Empty when the decision arrived for content this store has never held.
	ItemName string `json:"item"`
	// Unit is the durable unit identity (convergence.BlockKey — the source_id
	// a stored row carries).
	Unit string `json:"unit"`
	// Variant is the locale (and optional tone/channel) in VariantKey text form.
	Variant string `json:"variant"`
	// Status is the target-ladder rung the decision lands the unit on.
	Status string `json:"status,omitempty"`
	// TargetHash is the content hash of the translation the decision blesses
	// (state.TargetHash of the trimmed target text).
	TargetHash  string `json:"targetHash,omitempty"`
	ReviewState string `json:"reviewState,omitempty"` // approved | rejected | signed-off
	DecidedBy   string `json:"by,omitempty"`          // "" human · "ai/<model>" · "agent/<client>" · server identity
	DecidedAt   string `json:"at,omitempty"`          // RFC 3339
	Note        string `json:"note,omitempty"`
	Parked      bool   `json:"parked,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	// Updated orders conflicting records for last-writer-wins reconciliation.
	Updated string `json:"updated,omitempty"`
}

// DecisionStore is the optional capability of a content store that keeps the
// decision ledger (Bowrain side of core/state). Optional — assert for it —
// so every existing ContentStore fake keeps compiling; both real stores
// implement it.
type DecisionStore interface {
	// UpsertUnitDecisions records decisions idempotently, keyed by
	// (item, unit, variant): an identical record is a no-op, a changed one
	// replaces the latest state and appends a history event. Returns how many
	// records actually changed.
	UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []UnitDecision) (int, error)
	// ListUnitDecisions returns the project's latest decision per
	// (item, unit, variant) on a stream.
	ListUnitDecisions(ctx context.Context, projectID, stream string) ([]UnitDecision, error)
}

// BlockAccessStore is the optional capability behind the block access ladder
// (ABAC): who may edit, distinct from the review ladder on the target. Assert
// for it rather than for a concrete store type — a concrete-type assertion
// dies the moment the store is wrapped, which is exactly how the access
// endpoint went dead on every deployment that wraps its store in the
// event-emitting decorator.
type BlockAccessStore interface {
	// GetBlockAccess returns a block's access state and owner; a missing
	// block reports open/empty.
	GetBlockAccess(ctx context.Context, projectID, blockID string) (access, ownerID string, err error)
	// SetBlockAccess updates a block's access state and (when non-empty) its
	// owner.
	SetBlockAccess(ctx context.Context, projectID, blockID, access, ownerID string) error
	// GetLastEditor returns the author of the most recent attributed content
	// change (separation of duties at approval).
	GetLastEditor(ctx context.Context, projectID, blockID string) (string, error)
}
