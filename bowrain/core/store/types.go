// Package store defines the ContentStore interface and domain types
// for versioned content persistence.
package store

import (
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
	Archived              bool              `json:"archived"`
	ArchivedAt            *time.Time        `json:"archived_at,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
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
	Limit         int             // Max results (0 = no limit)
	Offset        int             // Pagination offset
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
type Collection struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id"`
	Name            string            `json:"name"`
	Kind            CollectionKind    `json:"kind"`
	ItemLabel       string            `json:"item_label"` // e.g. "item", "page", "post", "document"
	IsDefault       bool              `json:"is_default"`
	Stream          string            `json:"stream,omitempty"`           // empty = project-wide
	ConnectorConfig map[string]string `json:"connector_config,omitempty"` // connector type + settings
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
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
}

// ---------------------------------------------------------------------------
// Translation Dashboard Statistics
// ---------------------------------------------------------------------------

// TranslationDashboardStats holds aggregated translation metrics for a project.
type TranslationDashboardStats struct {
	LocaleStats        []LocaleTranslationStats     `json:"locale_stats"`
	ItemStats          []ItemTranslationStats       `json:"item_stats"`
	CollectionStats    []CollectionTranslationStats `json:"collection_stats"`
	TotalBlocks        int                          `json:"total_blocks"`
	TranslatableBlocks int                          `json:"translatable_blocks"`
	TotalSourceWords   int                          `json:"total_source_words"`
}

// LocaleTranslationStats holds translation progress for a single target locale.
type LocaleTranslationStats struct {
	Locale           string  `json:"locale"`
	DisplayName      string  `json:"display_name,omitempty"`
	TranslatedBlocks int     `json:"translated_blocks"`
	TotalBlocks      int     `json:"total_blocks"`
	TranslatedWords  int     `json:"translated_words"`
	TotalWords       int     `json:"total_words"`
	Percentage       float64 `json:"percentage"`
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
