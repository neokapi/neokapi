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

// StreamStore manages streams and their locks, tags and membership.
//
// Branching is NOT here — see StreamBranchStore. What remains is the surface a
// store keeps whether or not it can branch: naming streams, listing them,
// locking one, tagging it.
type StreamStore interface {
	GetStream(ctx context.Context, projectID, name string) (*Stream, error)
	ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*Stream, error)
	UpdateStream(ctx context.Context, s *Stream) error
	DeleteStream(ctx context.Context, projectID, name string) error

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

// StreamBranchStore is the optional capability behind branching: taking a
// branch, comparing it with its parent, and merging it back.
//
// Optional because branching is a SERVER concern. The desktop app keeps a local
// store for offline and cached editing and works on one stream — it hardcodes
// "main" at every call site and calls no stream verb at all — so carrying a
// branch implementation there would be code no caller can reach, kept in step
// with a model it never exercises.
//
// Assert for it rather than for a concrete store type: a concrete-type
// assertion dies the moment the store is wrapped, which is how the access
// endpoint went dead on every deployment using the event-emitting decorator.
type StreamBranchStore interface {
	// CreateStream takes a branch: the new stream starts as its parent, with
	// the parent's content, translations and approvals under the same ids.
	CreateStream(ctx context.Context, s *Stream) error
	// MergeStream fast-forwards a stream's parent onto it, or refuses because
	// the parent has moved since the branch was taken.
	MergeStream(ctx context.Context, projectID, streamName string, opts MergeOptions) (*MergeResult, error)
	// DiffStream compares a stream against its parent by content.
	DiffStream(ctx context.Context, projectID, streamName string) (*StreamDiff, error)
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
	// PruneItemBlocks removes the blocks of one item whose keys `keep` does not
	// name, and reports how many went.
	//
	// It is what makes a push able to say a string is GONE. A push carries only
	// what changed, so a deleted paragraph or a removed `t()` call sends
	// nothing at all: storing what arrives and pruning nothing left the block
	// in place for good — counted in the item's totals, listed in its content,
	// queued for review, dragging the coverage a ship gate reads. The producer
	// declares what each item it read now holds (core/venue.ItemBlockKeys) and
	// this removes the rest.
	//
	// `keep` holds producer keys — the ids a caller sends, which the store maps
	// to its own — and an empty slice is a real answer: an item whose last
	// translatable string was deleted keeps none. Callers that mean "say
	// nothing" must not call at all.
	//
	// Scoped to one stream. A branch holding the same item at the same ids is
	// untouched, which is what makes a prune safe to run on a branch at all.
	PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error)
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

// PushApplier is the write surface one push performs, every call landing in
// the same transaction. It is deliberately small — the verbs a push actually
// uses, and no others — so that what a push can do inside a transition is a
// readable list rather than the whole store.
//
// The reads on it are here for one reason: a push must see what the push has
// already written. An item's collection binding is decided against items this
// same transition may have just stored, and the decisions ledger is asserted
// against the rows this same transition is about to replace. A lookup made on
// the pool would answer from the state the push started in, and the answer
// would be wrong in exactly the cases that matter.
type PushApplier interface {
	StoreItem(ctx context.Context, projectID, stream string, item *Item) error
	StoreBlocks(ctx context.Context, projectID, stream string, blocks []*model.Block) error
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error)
	UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []venue.UnitDecision) (int, error)
	ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error)
	GetItem(ctx context.Context, projectID, stream, itemName string) (*Item, error)
	GetCollectionByName(ctx context.Context, projectID, name, stream string) (*Collection, error)
	GetDefaultCollection(ctx context.Context, projectID string) (*Collection, error)

	// The collection verbs. A push declares the context its content sits in,
	// and reconciling that is part of the same transition rather than a
	// preamble to it: an updated collection moves existing governance, so a
	// reconcile that survived a failed push would leave the project governed
	// by a declaration that never landed.
	//
	// The list read is on the transaction for the same reason the item reads
	// are: reconciliation answers "does this collection exist yet" against
	// collections this same transition may have just created.
	ListCollections(ctx context.Context, projectID, stream string) ([]*Collection, error)
	CreateCollection(ctx context.Context, c *Collection) error
	UpdateCollection(ctx context.Context, c *Collection) error

	// The identity verbs. A push declares a tree; which document each entry IS
	// is resolved against what the venue holds, and acted on before any content
	// lands — so a renamed file's blocks are written to the item that already
	// carries its approvals rather than to a freshly minted one.
	//
	// RenameItem moves an item to a new path, keeping its id and everything
	// hanging from it. DeleteItem removes an item the declared scope covers and
	// the declared tree does not mention.
	RenameItem(ctx context.Context, projectID, stream, itemID, newName string) error
	DeleteItem(ctx context.Context, projectID, stream, itemName string) error
}

// PushApplyStore is the optional capability of applying a whole push as one
// transition. A store that has it gets atomicity; one that does not is asked
// for its verbs one at a time, as before, and a push that fails partway leaves
// what it managed to write.
//
// It is asserted for by interface rather than required of ContentStore because
// the offline SQLite store on the desktop has no push to apply — it is the
// thing being pushed FROM.
type PushApplyStore interface {
	// ApplyPush runs fn against one transaction and commits it. An error from
	// fn rolls the whole transition back: nothing fn wrote survives.
	ApplyPush(ctx context.Context, fn func(PushApplier) error) error
}
