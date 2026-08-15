package store

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The persistence surface is decomposed into role interfaces by concern so
// handlers and test doubles can depend on the narrow slice they actually use
// (e.g. the sync worker needs only BlockStore + ChangeFeed). ContentStore is
// the umbrella the concrete server store satisfies; it is exactly the union of
// the roles below, so existing implementations and callers are unchanged.

// ProjectStore persists projects and their archive lifecycle.
type ProjectStore interface {
	CreateProject(ctx context.Context, p *Project) error
	GetProject(ctx context.Context, id string) (*Project, error)
	ListProjects(ctx context.Context) ([]*Project, error)
	UpdateProject(ctx context.Context, p *Project) error
	DeleteProject(ctx context.Context, id string) error
	ArchiveProject(ctx context.Context, id string) error
	RestoreProject(ctx context.Context, id string) error
	ListArchivedProjects(ctx context.Context, workspaceID string) ([]*Project, error)
}

// StreamStore manages streams and their operations, locks, tags and membership.
type StreamStore interface {
	CreateStream(ctx context.Context, s *Stream) error
	GetStream(ctx context.Context, projectID, name string) (*Stream, error)
	ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*Stream, error)
	UpdateStream(ctx context.Context, s *Stream) error
	DeleteStream(ctx context.Context, projectID, name string) error

	MergeStream(ctx context.Context, projectID, streamName string, opts MergeOptions) (*MergeResult, error)
	DiffStream(ctx context.Context, projectID, streamName string) (*StreamDiff, error)

	LockStream(ctx context.Context, projectID, streamName, userID string) error
	UnlockStream(ctx context.Context, projectID, streamName string) error

	CreateStreamTag(ctx context.Context, tag *StreamTag) error
	ListStreamTags(ctx context.Context, projectID, stream string) ([]*StreamTag, error)
	GetStreamTag(ctx context.Context, projectID, stream, tagName string) (*StreamTag, error)
	DeleteStreamTag(ctx context.Context, projectID, stream, tagName string) error
	ListProjectTags(ctx context.Context, projectID string, kind StreamTagKind) ([]*StreamTag, error)

	AddStreamMember(ctx context.Context, projectID, streamName, userID string) error
	RemoveStreamMember(ctx context.Context, projectID, streamName, userID string) error
	ListStreamMembers(ctx context.Context, projectID, streamName string) ([]string, error)
}

// CollectionStore manages collections (project-scoped, optionally stream-scoped).
type CollectionStore interface {
	CreateCollection(ctx context.Context, c *Collection) error
	GetCollection(ctx context.Context, projectID, collectionID string) (*Collection, error)
	GetCollectionByName(ctx context.Context, projectID, name, stream string) (*Collection, error)
	GetDefaultCollection(ctx context.Context, projectID string) (*Collection, error)
	ListCollections(ctx context.Context, projectID, stream string) ([]*Collection, error)
	UpdateCollection(ctx context.Context, c *Collection) error
	DeleteCollection(ctx context.Context, projectID, collectionID string) error
}

// ItemStore manages items (stream-scoped).
type ItemStore interface {
	StoreItem(ctx context.Context, projectID, stream string, item *Item) error
	GetItem(ctx context.Context, projectID, stream, itemName string) (*Item, error)
	ListItems(ctx context.Context, projectID, stream string) ([]*Item, error)
	DeleteItem(ctx context.Context, projectID, stream, itemName string) error
	GetItemByID(ctx context.Context, projectID, stream, itemID string) (*Item, error)
}

// BlockStore manages blocks and their notes/history (stream-scoped).
type BlockStore interface {
	StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	GetBlock(ctx context.Context, projectID, stream, blockID string) (*venue.StoredBlock, error)
	GetBlocks(ctx context.Context, query BlockQuery) ([]*venue.StoredBlock, error)
	// CountBlocks answers a BlockQuery's totals and its per-locale status
	// histogram in SQL, hydrating nothing. Limit and Offset are ignored — the
	// counts describe the whole matching set — and so is Status, which is the
	// histogram the call reports.
	CountBlocks(ctx context.Context, query BlockQuery) (BlockCounts, error)
	GetBlockStats(ctx context.Context, projectID, stream string) ([]BlockStatRow, error)
	// ListPendingReview pages the (block, locale) pairs awaiting a review
	// decision: translatable blocks whose stored target has text and a status
	// still below reviewed. Ordered by item then block so a session walks the
	// project in a stable, file-grouped order. Total counts the whole queue the
	// query scopes — with a collection filter, that collection's queue, so a
	// caller paging a collection is never told the project's total.
	ListPendingReview(ctx context.Context, query PendingReviewQuery) ([]PendingReviewRef, int, error)
	DeleteBlock(ctx context.Context, projectID, stream, blockID string) error

	// ClearBlockTargets removes ALL committed target translations for a block,
	// leaving the block (and its source + source-side overlays) in place. It is the
	// "invalidate every locale" primitive behind a back-to-source change: once a
	// block's source is rewritten, its translations are stale, so clearing them
	// drops the block back to untranslated and the next convergence pass re-drafts
	// every locale (the worker skips a block that still carries a target).
	ClearBlockTargets(ctx context.Context, projectID, stream, blockID string) error

	AddBlockNote(ctx context.Context, projectID, stream, blockID string, note model.BlockNote) error
	ListBlockNotes(ctx context.Context, projectID, stream, blockID string) ([]model.BlockNote, error)
	DeleteBlockNote(ctx context.Context, projectID, stream, noteID string) error

	GetBlockHistory(ctx context.Context, projectID, stream, blockID string, locale string, limit int) ([]BlockHistoryEntry, error)
}

// VersionStore manages named versions (stream-scoped).
type VersionStore interface {
	CreateVersion(ctx context.Context, projectID, stream, label, description string) (*Version, error)
	GetVersion(ctx context.Context, versionID string) (*Version, error)
	ListVersions(ctx context.Context, projectID, stream string) ([]*Version, error)
	Diff(ctx context.Context, fromVersion, toVersion string) (*VersionDiff, error)
}

// ChangeFeed exposes the incremental sync change log (stream-scoped).
type ChangeFeed interface {
	GetChanges(ctx context.Context, projectID, stream string, sinceCursor int64, locales []string, limit int) (*ChangeSet, error)
	LatestCursor(ctx context.Context, projectID, stream string) (int64, error)
	CompactChangeLog(ctx context.Context, projectID, stream string, retainDays int) (int64, error)
}

// AssetStore manages assets and their locale variants (Bowrain AD-007).
type AssetStore interface {
	StoreAsset(ctx context.Context, projectID, stream string, asset *venue.Asset) error
	GetAsset(ctx context.Context, projectID, stream, assetID string) (*venue.Asset, error)
	ListAssets(ctx context.Context, projectID, stream, itemName string) ([]*venue.Asset, error)
	DeleteAsset(ctx context.Context, projectID, stream, assetID string) error

	StoreAssetVariant(ctx context.Context, projectID string, variant *venue.AssetVariant) error
	GetAssetVariant(ctx context.Context, projectID, assetID, locale string) (*venue.AssetVariant, error)
	ListAssetVariants(ctx context.Context, projectID, assetID string) ([]*venue.AssetVariant, error)
}

// ContentStore is the primary persistence interface for localization content,
// the union of the role interfaces above. All content operations are
// stream-scoped; omitting the stream name (empty string) defaults to "main".
// Every project implicitly has a "main" stream.
type ContentStore interface {
	ProjectStore
	StreamStore
	CollectionStore
	ItemStore
	BlockStore
	VersionStore
	ChangeFeed
	AssetStore

	// Close releases the store's resources.
	Close() error
}
