// Package store defines the ContentStore interface and domain types
// for versioned content persistence.
package store

import (
	"time"

	"github.com/gokapi/gokapi/core/model"
)

// Project represents a localization project in the store.
type Project struct {
	ID            string
	Name          string
	SourceLocale  model.LocaleID
	TargetLocales []model.LocaleID
	Properties    map[string]string
	WorkspaceID   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// StoredBlock wraps a model.Block with store metadata.
type StoredBlock struct {
	*model.Block
	ProjectID   string
	ContentHash string
	ContextHash string
	StoredAt    time.Time
	UpdatedAt   time.Time
}

// BlockQuery filters blocks when listing or searching.
type BlockQuery struct {
	ProjectID   string
	IDs         []string         // Filter by block IDs
	ContentHash string           // Filter by content hash
	Translatable *bool           // Filter by translatable flag
	HasTarget   *model.LocaleID  // Filter blocks that have a target for this locale
	MissingTarget *model.LocaleID // Filter blocks missing a target for this locale
	Limit       int              // Max results (0 = no limit)
	Offset      int              // Pagination offset
}

// Version represents a named snapshot of project state.
type Version struct {
	ID          string
	ProjectID   string
	Label       string
	Description string
	BlockCount  int
	CreatedAt   time.Time
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
