package store

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// ContentStore is the primary persistence interface for localization content.
// All content operations are stream-scoped. Omitting the stream name (empty string)
// defaults to "main". Every project implicitly has a "main" stream.
type ContentStore interface {
	// Project management
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	UpdateProject(ctx context.Context, p *Project) error
	DeleteProject(ctx context.Context, id string) error
	ArchiveProject(ctx context.Context, id string) error
	RestoreProject(ctx context.Context, id string) error
	ListArchivedProjects(ctx context.Context, workspaceID string) ([]*Project, error)

	// Stream management
	CreateStream(ctx context.Context, s *Stream) error
	GetStream(ctx context.Context, projectID, name string) (*Stream, error)
	ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*Stream, error)
	UpdateStream(ctx context.Context, s *Stream) error
	DeleteStream(ctx context.Context, projectID, name string) error

	// Stream operations
	MergeStream(ctx context.Context, projectID, streamName string, opts MergeOptions) (*MergeResult, error)
	DiffStream(ctx context.Context, projectID, streamName string) (*StreamDiff, error)

	// Stream membership (for shared visibility)
	AddStreamMember(ctx context.Context, projectID, streamName, userID string) error
	RemoveStreamMember(ctx context.Context, projectID, streamName, userID string) error
	ListStreamMembers(ctx context.Context, projectID, streamName string) ([]string, error)

	// Collection management — project-scoped (optionally stream-scoped)
	CreateCollection(ctx context.Context, c *Collection) error
	GetCollection(ctx context.Context, projectID, collectionID string) (*Collection, error)
	GetCollectionByName(ctx context.Context, projectID, name, stream string) (*Collection, error)
	GetDefaultCollection(ctx context.Context, projectID string) (*Collection, error)
	ListCollections(ctx context.Context, projectID, stream string) ([]*Collection, error)
	UpdateCollection(ctx context.Context, c *Collection) error
	DeleteCollection(ctx context.Context, projectID, collectionID string) error

	// Item management — stream-scoped
	StoreItem(ctx context.Context, projectID, stream string, item *Item) error
	GetItem(ctx context.Context, projectID, stream, itemName string) (*Item, error)
	ListItems(ctx context.Context, projectID, stream string) ([]*Item, error)
	DeleteItem(ctx context.Context, projectID, stream, itemName string) error
	GetItemByID(ctx context.Context, projectID, stream, itemID string) (*Item, error)

	// Block storage — stream-scoped
	StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	GetBlock(ctx context.Context, projectID, stream, blockID string) (*StoredBlock, error)
	GetBlocks(ctx context.Context, query BlockQuery) ([]*StoredBlock, error)
	GetBlockStats(ctx context.Context, projectID, stream string) ([]BlockStatRow, error)
	DeleteBlock(ctx context.Context, projectID, stream, blockID string) error

	// Version management — stream-scoped
	CreateVersion(ctx context.Context, projectID, stream, label, description string) (*Version, error)
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	ListVersions(ctx context.Context, projectID, stream string) ([]*Version, error)
	Diff(ctx context.Context, fromVersion, toVersion string) (*VersionDiff, error)

	// Block notes — stream-scoped
	AddBlockNote(ctx context.Context, projectID, stream, blockID string, note model.BlockNote) error
	ListBlockNotes(ctx context.Context, projectID, stream, blockID string) ([]model.BlockNote, error)
	DeleteBlockNote(ctx context.Context, projectID, stream, noteID string) error

	// Block history — stream-scoped
	GetBlockHistory(ctx context.Context, projectID, stream, blockID string, locale string, limit int) ([]BlockHistoryEntry, error)

	// Change log (incremental sync) — stream-scoped
	GetChanges(ctx context.Context, projectID, stream string, sinceCursor int64, locales []string, limit int) (*ChangeSet, error)
	LatestCursor(ctx context.Context, projectID, stream string) (int64, error)
	CompactChangeLog(ctx context.Context, projectID, stream string, retainDays int) (int64, error)

	// Asset management — stream-scoped (AD-029)
	StoreAsset(ctx context.Context, projectID, stream string, asset *Asset) error
	GetAsset(ctx context.Context, projectID, stream, assetID string) (*Asset, error)
	ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*Asset, error)
	DeleteAsset(ctx context.Context, projectID, stream, assetID string) error

	// Asset locale variants (AD-029)
	StoreAssetVariant(ctx context.Context, projectID string, variant *AssetVariant) error
	GetAssetVariant(ctx context.Context, projectID, assetID, locale string) (*AssetVariant, error)
	ListAssetVariants(ctx context.Context, projectID, assetID string) ([]*AssetVariant, error)

	// Lifecycle
	Close() error
}
