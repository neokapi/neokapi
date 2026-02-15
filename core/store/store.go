package store

import (
	"context"
	"io"

	"github.com/gokapi/gokapi/core/model"
)

// ContentStore is the primary persistence interface for localization content.
// It provides project management, block storage with content-addressable
// deduplication, version tracking, and KAZ export/import.
type ContentStore interface {
	// Project management
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	UpdateProject(ctx context.Context, p *Project) error
	DeleteProject(ctx context.Context, id string) error

	// Block storage
	StoreBlocks(ctx context.Context, projectID string, blocks []*model.Block) error
	GetBlock(ctx context.Context, projectID, blockID string) (*StoredBlock, error)
	GetBlocks(ctx context.Context, query BlockQuery) ([]*StoredBlock, error)
	DeleteBlock(ctx context.Context, projectID, blockID string) error

	// Version management
	CreateVersion(ctx context.Context, projectID, label, description string) (*Version, error)
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	ListVersions(ctx context.Context, projectID string) ([]*Version, error)
	Diff(ctx context.Context, fromVersion, toVersion string) (*VersionDiff, error)

	// KAZ export/import
	ExportKAZ(ctx context.Context, projectID string, w io.Writer) error
	ImportKAZ(ctx context.Context, r io.Reader) (string, error) // Returns project ID

	// Change log (incremental sync)
	GetChanges(ctx context.Context, projectID string, sinceCursor int64, locale string, limit int) (*ChangeSet, error)
	LatestCursor(ctx context.Context, projectID string) (int64, error)
	CompactChangeLog(ctx context.Context, projectID string, retainDays int) (int64, error)

	// Lifecycle
	Close() error
}
