// Package store defines the ContentStore interface and domain types
// for versioned content persistence.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
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

// Item.Properties keys this server assigns meaning to. Everything else in the
// map is a producer's own and is stored untouched.
const (
	// ItemPropSourcePath is the file the item's content was lifted out of,
	// when that is not the item itself. Set for a generated catalog — a KBF
	// bundle extracted from `App.tsx` is named `…/App.kbf.json`, because that
	// path is its identity — so a surface can show the item as its source
	// without the item ever being renamed. Absent for every item that IS its
	// own source, which is most of them.
	ItemPropSourcePath = "source_path"
)

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

	// AfterID resumes a scan after the block id it names, which is how a
	// caller walks a project's blocks a batch at a time. Blocks are returned in
	// id order, so this is a keyset cursor: it costs the same on the thousandth
	// batch as on the first, and — unlike Offset — a row updated mid-scan
	// cannot shift the window and make the walk skip its neighbour.
	//
	// A whole-project read is what memory is spent on, so reach for this rather
	// than for a limitless query. See EachBlockBatch, which is that walk.
	AfterID string

	// BeforeID is the same keyset cursor pointing the other way: the blocks
	// whose id sorts before the one it names, nearest first. A surface asking
	// "what precedes this block" pairs `BeforeID` with `Limit: 1` and pays for
	// one row, where a scan from the start of the item pays for the item.
	//
	// Results still arrive in ascending id order, so a caller can hand them to
	// the same projection an AfterID page goes through.
	BeforeID string
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

// PendingReviewRef names one (block, locale) pair awaiting review, with the
// collection its item belongs to so a queue row can be grouped and filtered
// without a second source for the pairing.
type PendingReviewRef struct {
	BlockID  string `json:"block_id"`
	ItemName string `json:"item_name"`
	Locale   string `json:"locale"`
	// CollectionID is the collection of the item this block belongs to, "" for
	// an item in no collection. A block whose item has no row for the stream
	// reads as "" too — the item row is the only thing that knows, and a queue
	// that dropped such a block would be worse than one that files it as
	// ungrouped.
	CollectionID string `json:"collection_id"`
}

// PendingReviewQuery scopes one page of the review queue.
type PendingReviewQuery struct {
	ProjectID string
	Stream    string
	// Locales narrows to these target locales; empty imposes no constraint.
	Locales []string
	// CollectionID narrows the queue to the items of one collection. Nil
	// imposes no constraint; a non-nil empty string selects the items belonging
	// to no collection — the ungrouped bucket the dashboard rollups also name.
	// A pointer rather than a string because the empty collection is a scope,
	// not the absence of one.
	CollectionID *string
	Limit        int
	Offset       int
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

	// Owner is "recipe" or "workspace" (core/venue.ContextOwner*). It
	// arrives as a protocol field on the pushed context entry and is stored
	// verbatim: every later ownership decision is a lookup of this field, not a
	// rule reconstructed from how the row happens to look.
	Owner string `json:"owner,omitempty"`

	// ContextHash is the hash of the context entry this row was reconciled
	// from (core/venue.ComputeContextEntryHash). It is what makes a
	// re-push idempotent: an entry arriving with an unchanged hash leaves the
	// row — and its UpdatedAt — untouched.
	ContextHash string `json:"context_hash,omitempty"`

	// Preview is where this collection's strings can be read in place: the
	// component explorer or running site that shows them as a reader sees
	// them. Declared per collection because a repository publishes one per
	// surface it ships, and carried on the context entry with the coordinates
	// and the voice. Empty when the recipe declares none, which is how a
	// reviewer decides to offer document reading only.
	//
	// PreviewKind says how a view is found within that host. A kind this
	// server does not recognise is stored and served unchanged: what can be
	// read is the client's judgement, not the store's.
	PreviewKind string `json:"preview_kind,omitempty"`
	PreviewURL  string `json:"preview_url,omitempty"`

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
	Properties  map[string]string `json:"properties,omitempty"`  // extensible metadata (voice bindings, etc.)
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
	LocaleStats []LocaleTranslationStats `json:"locale_stats"`
	ItemStats   []ItemTranslationStats   `json:"item_stats"`
	ItemTotal   int                      `json:"item_total"`

	// ItemBase is the directory prefix every item in scope shares, with a
	// trailing slash, or empty when they share none. A recipe collection is
	// declared with a `base:` and its items are named relative to the project
	// root, so every row of a collection repeats that base — "bowrain/packages/
	// app/src/…" on each of a thousand files, which is the part carrying no
	// information at all. Consumers display names relative to it.
	//
	// It is computed over the whole scope rather than the returned page, so it
	// does not move as the reader pages or re-sorts. That is why it is a field
	// here and not something a client derives from ItemStats.
	ItemBase string `json:"item_base,omitempty"`

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
	// the project's ship gate — a rule-based check with error severity, OR a
	// terminology violation (a forbidden/competitor term used, or a mandated preferred/
	// approved rendering missing). Only computed for locales at full coverage in
	// some scope (checks cannot promote an under-covered locale, so the expensive
	// pass is skipped below the coverage gate).
	FailingChecks int `json:"failing_checks"`
	// StaleBlocks counts this scope's (block, locale) pairs whose recorded
	// decision blessed source wording the block no longer carries — the basis
	// the decision names against the block's current content hash. A stale pair
	// withholds the scope from shipping whether or not any coverage bar was
	// declared (see DeriveShipState). Additive: producers that do not grade the
	// basis leave it 0 (omitted from JSON).
	StaleBlocks int `json:"stale_blocks,omitempty"`
	// BasisUnknownBlocks counts pairs whose decision carries no basis at all.
	// Such a record says nothing about the source it blessed, so it keeps its
	// rung and ships as it did before — but the assumption behind that rung is
	// reported rather than left silent, and it clears itself the next time the
	// unit is decided.
	BasisUnknownBlocks int `json:"basis_unknown_blocks,omitempty"`
	// ShipState is the derived per-locale ship state (see DeriveShipState).
	// Empty when the producer did not derive it.
	ShipState ShipState `json:"ship_state,omitempty"`
	// CompliantBlocks counts translated blocks that pass the project's rule-based
	// checks with no error-severity finding, are term-compliant for the locale (no
	// forbidden/competitor term, no missing mandated rendering), AND — where a
	// persisted voice score exists for the block+locale — carry a score at
	// or above the scoring profile's minimum bar. Additive: producers that do not
	// derive the compliance rate leave it 0 (omitted from JSON).
	CompliantBlocks int `json:"compliant_blocks,omitempty"`
	// ComplianceRate is CompliantBlocks / TranslatedBlocks, in [0,1]. Nil when the
	// producer did not derive it or the scope has no translated blocks.
	ComplianceRate *float64 `json:"compliance_rate,omitempty"`
	// ComplianceBasis states what informed ComplianceRate (see ComplianceBasisFor):
	// rule-based checks always, plus "+terms" when term governance was active for
	// the scope and plus "voice" when at least one block's persisted voice score
	// also informed it. Empty when the rate was not derived — consumers hide the
	// metric then.
	ComplianceBasis ComplianceBasis `json:"compliance_basis,omitempty"`
}

// ComplianceBasis names the evidence behind a derived compliance rate, so consumers
// can present the number honestly: a checks-only rate says nothing about voice.
// Rule-based checks always inform the rate; terms and voice are added when they
// were actually applied to the scope (term governance active / a persisted voice
// score present), so the basis never claims evidence that did not contribute.
type ComplianceBasis string

const (
	// ComplianceBasisChecks — the rate reflects rule-based check findings only; no
	// term governance was active and no voice scores existed for the scope.
	ComplianceBasisChecks ComplianceBasis = "checks"
	// ComplianceBasisChecksTerms — rule-based checks plus deterministic terminology
	// compliance (forbidden/competitor presence, mandated-rendering absence).
	ComplianceBasisChecksTerms ComplianceBasis = "checks+terms"
	// ComplianceBasisVoice — rule-based checks plus persisted voice scores measured
	// against the scoring profile's minimum bar.
	ComplianceBasisVoice ComplianceBasis = "voice+checks"
	// ComplianceBasisVoiceTerms — rule-based checks plus terminology compliance plus
	// persisted voice scores: the fullest basis.
	ComplianceBasisVoiceTerms ComplianceBasis = "voice+checks+terms"
)

// ComplianceBasisFor names the evidence behind a derived compliance rate from whether
// a persisted voice score and active term governance informed it. Rule-based
// checks are always part of the basis; terms and voice are added when they
// contributed.
func ComplianceBasisFor(voice, terms bool) ComplianceBasis {
	switch {
	case voice && terms:
		return ComplianceBasisVoiceTerms
	case voice:
		return ComplianceBasisVoice
	case terms:
		return ComplianceBasisChecksTerms
	default:
		return ComplianceBasisChecks
	}
}

// TermCompliance is one target's terminology verdict, as a review surface
// receives it per (block, locale). It is the per-block half of what
// ComplianceBasis names in aggregate: the deterministic, offline check that the
// target uses no forbidden or competitor term and omits no mandated rendering
// its source concept requires.
//
// Three rungs, not two, because "not checked" is not "compliant": a project
// with no terms store and no brand vocabulary has nothing to be compliant with,
// and a queue that painted those targets green would be claiming evidence it
// never had.
type TermCompliance string

const (
	// TermComplianceUnchecked — no terminology governance was active for the
	// locale, so nothing was checked and nothing is claimed.
	TermComplianceUnchecked TermCompliance = ""
	// TermComplianceCompliant — the target was checked and respects the
	// project's terminology.
	TermComplianceCompliant TermCompliance = "compliant"
	// TermComplianceViolation — the target uses a forbidden or competitor term,
	// or omits a rendering its source concept mandates for the locale. The
	// server will not auto-approve it.
	TermComplianceViolation TermCompliance = "violation"
)

// ItemTranslationStats holds per-file translation progress.
type ItemTranslationStats struct {
	ItemName     string `json:"item_name"`
	ItemID       string `json:"item_id"`
	Format       string `json:"format"`
	CollectionID string `json:"collection_id"`
	// SourcePath is the file this item's content was lifted out of, when the
	// item is a generated catalog rather than the source itself
	// (Item.Properties[ItemPropSourcePath]). It is what a list SHOWS; ItemName
	// stays what everything ADDRESSES. Empty for an item that is its own
	// source, which is most of them.
	SourcePath string `json:"source_path,omitempty"`
	// CollectionName is the name of the collection CollectionID refers to.
	// Empty when the item is ungrouped, or when its collection id names a row
	// this project no longer holds.
	CollectionName string                   `json:"collection_name,omitempty"`
	BlockCount     int                      `json:"block_count"`
	WordCount      int                      `json:"word_count"`
	Locales        []LocaleTranslationStats `json:"locales"`
}

// CollectionTranslationStats holds per-collection translation progress.
//
// Channel and Coordinates project what the collection row already persists, so
// a consumer can group the rollup by the point in context space the content
// occupies without a second round trip. Both are omitted for a collection that
// declares neither — including the ungrouped bucket.
type CollectionTranslationStats struct {
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name"`
	// Channel is the delivery channel the collection is bound to, as the
	// context push stored it on the collection's connector config.
	Channel string `json:"channel,omitempty"`
	// Coordinates is the collection's point in the project's context space:
	// axis → value, as Collection.Context holds it.
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// PreviewKind and PreviewURL are where this collection's strings can be
	// read in place, as the context push stored them on the collection row.
	// Projected here because the items view opens a preview from this rollup
	// and has no other route to the collection: without them the in-context
	// reading is offered on the project page and missing on the page where a
	// reviewer actually opens a file.
	PreviewKind string `json:"preview_kind,omitempty"`
	PreviewURL  string `json:"preview_url,omitempty"`
	// Ungrouped marks the synthetic bucket that holds items belonging to no
	// collection (CollectionID is empty and no collection row exists). It is
	// the flag rather than an invented id, so a consumer can label and place
	// the bucket without an id that resolves to nothing.
	Ungrouped  bool                     `json:"ungrouped,omitempty"`
	ItemCount  int                      `json:"item_count"`
	BlockCount int                      `json:"block_count"`
	WordCount  int                      `json:"word_count"`
	Locales    []LocaleTranslationStats `json:"locales"`
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
	UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []venue.UnitDecision) (int, error)
	// ListUnitDecisions returns the project's latest decision per
	// (item, unit, variant) on a stream.
	ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error)
	// TallyDecisionBasis grades the stream's recorded decisions against the
	// source the project holds NOW, grouped by (item, variant). A decision
	// records the basis it blessed (UnitDecision.ContentHash) and the block
	// records its current source hash; the two are the same value
	// (model.ComputeContentHash of the source text), so the grading is an
	// equality join and never a re-derivation. Decisions naming a unit this
	// store holds no block for are omitted — there is nothing to grade them
	// against.
	TallyDecisionBasis(ctx context.Context, projectID, stream string) ([]DecisionBasisTally, error)
}

// UnitDecisionReader reads ONE unit's latest decision. It sits beside
// DecisionStore rather than inside it because the two answer different
// questions: a reconciliation pass wants the project's ledger, and a surface
// showing who decided the unit in front of the reviewer wants one row. Asking
// the first question to answer the second reads the whole project's decisions
// on every block a reviewer opens.
//
// Optional — assert for it — so a store that has only the ledger keeps
// compiling; both real stores implement it.
type UnitDecisionReader interface {
	// GetUnitDecision returns the latest decision for (item, unit, variant), or
	// nil with no error when the ledger holds none: a unit awaiting its first
	// decision is the ordinary case, not a failure.
	GetUnitDecision(ctx context.Context, projectID, stream, itemName, unit, variant string) (*venue.UnitDecision, error)
}

// DecisionBasisTally is one (item, variant) scope's standing of recorded
// decisions against the source the project holds now.
type DecisionBasisTally struct {
	// ItemName scopes the units, so a consumer can attribute the counts to the
	// collection the item belongs to.
	ItemName string
	// Variant is the decision's locale (and optional tone/channel) in
	// VariantKey text form, as the ledger stores it.
	Variant string
	// Stale counts decisions whose basis names source wording the block no
	// longer carries: the translation renders a sentence the project has since
	// rewritten.
	Stale int
	// BasisUnknown counts decisions recorded before a basis was tracked. Empty
	// is unknown, never stale — reading that silence as drift would withhold
	// every locale of every project holding decisions from before the field
	// existed.
	BasisUnknown int
}

// ChannelAliasProposal is the workspace's observation that two channel slugs
// look like one channel.
//
// A workspace owns EQUIVALENCE, never RESOLUTION. A project resolves its own
// coordinates from its own recipe, offline, and a server that rewrote a pushed
// slug would make the same recipe mean different things depending on whether it
// had been connected. So this is a proposal, with its evidence attached, and
// nothing reads it to resolve anything.
type ChannelAliasProposal struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
	// Profile is the product-axis value both slugs sit under. Equivalence is
	// only ever proposed within one profile: a channel is a surface OF a
	// product, so two products' identically named channels are two channels.
	Profile string `json:"profile,omitempty"`
	// ProposedChannel is the slug that arrived; ExistingChannel is the one the
	// workspace already held.
	ProposedChannel string `json:"proposed_channel"`
	ExistingChannel string `json:"existing_channel"`
	// Evidence names why the two look alike, so a reviewer judges the
	// observation rather than trusting it.
	Evidence string `json:"evidence,omitempty"`
	// ProjectID and Collection locate the push that raised it.
	ProjectID  string `json:"project_id,omitempty"`
	Collection string `json:"collection,omitempty"`
	// Status is proposed | accepted | dismissed. Accepting records agreement;
	// it still rewrites nothing.
	Status string `json:"status,omitempty"`
	// JudgedBy and JudgedAt record who settled the proposal and when. They are
	// distinct from UpdatedAt, which moves every time the same fragmentation is
	// observed again: the sighting is fresh, the judgement is not.
	JudgedBy  string `json:"judged_by,omitempty"`
	JudgedAt  string `json:"judged_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ChannelAliasJudgement settles one proposal: the key that identifies it, the
// status a reviewer chose, and the reviewer.
type ChannelAliasJudgement struct {
	WorkspaceID     string
	Profile         string
	ProposedChannel string
	ExistingChannel string
	// Status is ChannelAliasAccepted or ChannelAliasDismissed.
	Status string
	// JudgedBy is the user id of the reviewer.
	JudgedBy string
}

// Channel-alias proposal statuses.
const (
	ChannelAliasProposed  = "proposed"
	ChannelAliasAccepted  = "accepted"
	ChannelAliasDismissed = "dismissed"
)

// ChannelAliasStore is the optional capability of a content store that keeps
// channel-slug equivalence proposals. Optional — assert for it — so every
// existing ContentStore fake keeps compiling.
type ChannelAliasStore interface {
	// UpsertChannelAliasProposals records proposals idempotently, keyed by
	// (workspace, profile, proposed, existing). A proposal a reviewer already
	// judged keeps its status: re-observing the same fragmentation must not
	// reopen a dismissal.
	UpsertChannelAliasProposals(ctx context.Context, proposals []ChannelAliasProposal) (int, error)
	// ListChannelAliasProposals returns a workspace's proposals, newest first.
	// An empty status returns every one.
	ListChannelAliasProposals(ctx context.Context, workspaceID, status string) ([]ChannelAliasProposal, error)
	// JudgeChannelAliasProposal settles one proposal. It reports false when the
	// workspace holds no such proposal, so a stale page cannot invent one.
	// Accepting records equivalence between two spellings; it rewrites no
	// project's slug, because resolution is the recipe's and stays offline.
	JudgeChannelAliasProposal(ctx context.Context, j ChannelAliasJudgement) (bool, error)
}

// BlockAccessStore is the optional capability behind the block access ladder
// (ABAC): who may edit, distinct from the review ladder on the target. Assert
// for it rather than for a concrete store type — a concrete-type assertion
// dies the moment the store is wrapped, which is exactly how the access
// endpoint went dead on every deployment that wraps its store in the
// event-emitting decorator.
// Every verb here names a stream: a block belongs to one, and the same id sits
// on every branch that holds it, so who may edit it is a question about a
// branch and not about a project.
type BlockAccessStore interface {
	// GetBlockAccess returns a block's access state and owner; a missing
	// block reports open/empty.
	GetBlockAccess(ctx context.Context, projectID, stream, blockID string) (access, ownerID string, err error)
	// SetBlockAccess updates a block's access state and (when non-empty) its
	// owner.
	SetBlockAccess(ctx context.Context, projectID, stream, blockID, access, ownerID string) error
	// GetLastEditor returns the author of the most recent attributed content
	// change (separation of duties at approval).
	GetLastEditor(ctx context.Context, projectID, stream, blockID string) (string, error)
}

// TargetRef names one block's target in one locale.
type TargetRef struct {
	BlockID string
	Locale  string
}

// TargetAuthorStore is the optional capability that names who last wrote each
// target by hand. Separation of duties at approval asks the question per
// language: approving the French wording you typed is a conflict of interest,
// approving the German wording somebody else typed is not.
//
// Two filters make the answer mean "who wrote this translation". Only content
// changes count, so the decision rows the ledger files against the same block
// do not make the previous reviewer look like the author. Only attributed
// changes count. A target a run produced was written outside any request, with
// no acting user, so it reports nothing and one person can still approve what
// the machine wrote.
//
// Assert for it rather than for a concrete store type: a concrete-type
// assertion dies the moment the store is wrapped in the event-emitting
// decorator.
type TargetAuthorStore interface {
	// LastTargetAuthors returns the last human author of each (block, locale)
	// target in the cross product of blockIDs and locales. A pair with no
	// attributed content change is absent from the map. One query answers a
	// whole batch, so a bulk approval does not become a round trip per block.
	LastTargetAuthors(ctx context.Context, projectID, stream string, blockIDs, locales []string) (map[TargetRef]string, error)
}
