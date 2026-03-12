// Package store defines the ContentStore interface and domain types
// for versioned content persistence.
package store

import (
	"time"

	"github.com/neokapi/neokapi/core/model"
)

const (
	// MaxBlocksPerRequest limits the number of blocks in a single sync push request.
	MaxBlocksPerRequest = 1000

	// DefaultBlockLimit is the default limit for block queries.
	DefaultBlockLimit = 10000
)

// Project represents a localization project in the store.
type Project struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	SourceLocale  model.LocaleID    `json:"source_locale"`
	TargetLocales []model.LocaleID  `json:"target_locales"`
	Properties    map[string]string `json:"properties,omitempty"`
	WorkspaceID   string            `json:"workspace_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Item represents a file or data object within a project.
type Item struct {
	ProjectID   string            `json:"project_id"`
	Name        string            `json:"name"`
	Format      string            `json:"format"`
	ItemType    string            `json:"item_type"`
	SourceBytes []byte            `json:"source_bytes,omitempty"`
	BlockIndex  string            `json:"block_index"`
	Properties  map[string]string `json:"properties,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
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
	ProjectID   string           `json:"project_id"`
	Name        string           `json:"name"`        // "main", "v2.0", "feature/new-ui", "pr/142"
	Parent      string           `json:"parent"`       // parent stream name; empty for "main"
	BaseCursor  int64            `json:"base_cursor"`  // cursor in parent at branch point
	Archived    bool             `json:"archived"`
	Visibility  StreamVisibility `json:"visibility"`   // "public", "private", "shared"
	Description string           `json:"description"`  // human-readable purpose
	SharedWith  []string         `json:"shared_with,omitempty"` // user IDs (only for "shared" visibility)
	CreatedAt   time.Time        `json:"created_at"`
	CreatedBy   string           `json:"created_by"`
}

// MergeOptions controls stream merge behavior.
type MergeOptions struct {
	// DryRun when true returns the diff without applying changes.
	DryRun bool
}

// MergeResult describes the outcome of a stream merge.
type MergeResult struct {
	MergedBlocks  int           `json:"merged_blocks"`
	AddedBlocks   int           `json:"added_blocks"`
	ModifiedBlocks int          `json:"modified_blocks"`
	RemovedBlocks int           `json:"removed_blocks"`
	Changes       []BlockChange `json:"changes,omitempty"`
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
type BlockHistoryEntry struct {
	Seq        int64     `json:"seq"`
	ChangeType string    `json:"changeType"`
	Text       string    `json:"text"`
	CodedText  string    `json:"codedText"`
	Origin     string    `json:"origin"`
	Author     string    `json:"author"`
	Timestamp  time.Time `json:"timestamp"`
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
