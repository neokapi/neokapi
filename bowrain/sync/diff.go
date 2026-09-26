// Package sync hosts the server-side sync engine: the Merkle-tree diff engine
// (diff.go) and the Redis-backed hash cache (cache_redis.go), which depend on
// bowrain/core/store and redis and therefore belong in the platform module.
//
// The pure model<->protobuf converters and the content-hash helpers
// (ComputeItemHash, ComputeRootHash) live in the framework-only package
// github.com/neokapi/neokapi/core/venue so the bowrain/core push client
// can use them without importing the platform module.
package sync

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// DiffEngine computes the difference between client and server state
// using the Merkle tree hash model (Bowrain AD-009).
type DiffEngine struct {
	contentStore platstore.ContentStore
	cache        HashCache // optional; nil = always query DB
}

// HashCache provides cached access to a project's transfer hashes.
// Implementations: RedisHashCache (production), nil (fallback to DB).
//
// A reader takes a View before it reads the store and files what it computed
// through that same view. InvalidateProject moves the project to a new
// generation, and a view taken before it files into the old one, which no
// later view reads. Without that, a reader that read the store just before a
// push applied and wrote its hashes just after the push invalidated would
// leave the pre-apply hashes answering for the project for the whole TTL.
type HashCache interface {
	// View pins the project's current generation for one stream.
	View(ctx context.Context, projectID, stream string) HashView

	// InvalidateProject retires every cached hash for a project, on every
	// stream. The push worker calls it after its apply commits.
	InvalidateProject(ctx context.Context, projectID string)
}

// HashView reads and writes one stream's cached hashes at one generation.
type HashView interface {
	// GetItemHashes returns all item_name → item_hash. Returns nil, false on
	// a miss.
	GetItemHashes(ctx context.Context) (map[string]string, bool)

	// GetBlockHashes returns all block_id → record_hash for an item. Returns
	// nil, false on a miss.
	GetBlockHashes(ctx context.Context, itemName string) (map[string]string, bool)

	// SetItemHashes caches item hashes.
	SetItemHashes(ctx context.Context, hashes map[string]string)

	// SetBlockHashes caches block hashes for an item.
	SetBlockHashes(ctx context.Context, itemName string, hashes map[string]string)
}

// view pins the cache for one read of the store; a nil cache caches nothing.
func (d *DiffEngine) view(ctx context.Context, projectID, stream string) HashView {
	if d.cache == nil {
		return nopView{}
	}
	return d.cache.View(ctx, projectID, stream)
}

// NewDiffEngine creates a diff engine.
func NewDiffEngine(cs platstore.ContentStore, cache HashCache) *DiffEngine {
	return &DiffEngine{contentStore: cs, cache: cache}
}

// ItemDiffResult describes which items need attention.
type ItemDiffResult struct {
	// Items where the client hash differs from server (need block-level diff).
	ChangedItems []string

	// Items the server has that the client didn't include (deletions on client side).
	DeletedItems []string

	// Items the client has that the server doesn't (new items).
	NewItems []string

	// Count of items with matching hashes (no changes).
	UnchangedCount int

	// Whether terms/content memory collections changed.
	TermsChanged  bool
	MemoryChanged bool
}

// CompareItems performs the first level of Merkle comparison: item-level hashes.
// Returns which items need block-level diff.
func (d *DiffEngine) CompareItems(ctx context.Context, projectID, stream string, clientItemHashes map[string]string) (*ItemDiffResult, error) {
	serverHashes, err := d.loadItemHashes(ctx, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("load server item hashes: %w", err)
	}

	result := &ItemDiffResult{}

	// Check client items against server.
	for itemName, clientHash := range clientItemHashes {
		serverHash, exists := serverHashes[itemName]
		if !exists {
			result.NewItems = append(result.NewItems, itemName)
		} else if clientHash != serverHash {
			result.ChangedItems = append(result.ChangedItems, itemName)
		} else {
			result.UnchangedCount++
		}
	}

	// Check for server items the client doesn't have (upstream deletions or items
	// the client never synced).
	for itemName := range serverHashes {
		if _, inClient := clientItemHashes[itemName]; !inClient {
			result.DeletedItems = append(result.DeletedItems, itemName)
		}
	}

	return result, nil
}

// BlockDiffResult describes which blocks need uploading for a single item.
type BlockDiffResult struct {
	// Blocks to upload (new or changed on client side).
	Needed []string

	// Blocks the server has that client doesn't (upstream deletions within item).
	Deleted []string

	// Blocks changed by another client (conflict: server hash differs from
	// client's expected_hash).
	Conflicts []string
}

// CompareBlocks performs the second level: block-level comparison for one item.
func (d *DiffEngine) CompareBlocks(ctx context.Context, projectID, stream, itemName string, clientBlockHashes map[string]string) (*BlockDiffResult, error) {
	serverHashes, err := d.loadBlockHashes(ctx, d.view(ctx, projectID, stream), projectID, stream, itemName)
	if err != nil {
		return nil, fmt.Errorf("load server block hashes for %s: %w", itemName, err)
	}

	result := &BlockDiffResult{}

	for blockID, clientHash := range clientBlockHashes {
		serverHash, exists := serverHashes[blockID]
		if !exists {
			// New block — client has it, server doesn't.
			result.Needed = append(result.Needed, blockID)
		} else if clientHash != serverHash {
			// Changed — content differs.
			result.Needed = append(result.Needed, blockID)
		}
		// else: matching hash, skip.
	}

	// Server blocks the client doesn't have.
	for blockID := range serverHashes {
		if _, inClient := clientBlockHashes[blockID]; !inClient {
			result.Deleted = append(result.Deleted, blockID)
		}
	}

	return result, nil
}

// ExportItemHashes returns computed item-level hashes for a project.
// Exported for CLI and tests.
func (d *DiffEngine) ExportItemHashes(ctx context.Context, projectID, stream string) (map[string]string, error) {
	return d.loadItemHashes(ctx, projectID, stream)
}

// CheckRootHash performs the fast path: if the client root hash matches the
// server's, nothing changed and we can skip all diff computation.
func (d *DiffEngine) CheckRootHash(ctx context.Context, projectID, stream, clientRootHash string) (bool, error) {
	serverHashes, err := d.loadItemHashes(ctx, projectID, stream)
	if err != nil {
		return false, err
	}
	serverRoot := venue.ComputeRootHash(serverHashes)
	return clientRootHash == serverRoot, nil
}

// loadItemHashes loads item-level hashes (item_name → hash of block hashes).
// Uses cache if available, falls back to computing from DB.
func (d *DiffEngine) loadItemHashes(ctx context.Context, projectID, stream string) (map[string]string, error) {
	view := d.view(ctx, projectID, stream)
	if cached, ok := view.GetItemHashes(ctx); ok {
		return cached, nil
	}

	items, err := d.contentStore.ListItems(ctx, projectID, stream)
	if err != nil {
		return nil, err
	}

	itemHashes := make(map[string]string, len(items))
	for _, item := range items {
		blockHashes, err := d.loadBlockHashes(ctx, view, projectID, stream, item.Name)
		if err != nil {
			return nil, err
		}
		// An item holding no blocks is not content, as in LoadTree: the row
		// can stand to anchor decisions after its file was removed. Hashing it
		// would give the venue a file no producer declares, and every push of
		// the project would negotiate a diff.
		if len(blockHashes) == 0 {
			continue
		}
		itemHashes[item.Name] = venue.ComputeItemHash(blockHashes)
	}

	view.SetItemHashes(ctx, itemHashes)
	return itemHashes, nil
}

// loadBlockHashes loads block-level hashes for a single item, through the
// view the caller pinned before it started reading the store.
func (d *DiffEngine) loadBlockHashes(ctx context.Context, view HashView, projectID, stream, itemName string) (map[string]string, error) {
	if cached, ok := view.GetBlockHashes(ctx, itemName); ok {
		return cached, nil
	}

	blocks, err := d.contentStore.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	hashes := make(map[string]string, len(blocks))
	for _, sb := range blocks {
		// Use source_id as key when available — this is the stable client-facing
		// block ID. The internal ID may differ due to source_id remapping in
		// StoreBlocksForItem. Clients compute hashes keyed by their original IDs.
		key := sb.SourceID
		if key == "" {
			key = sb.Block.ID
		}
		// The transfer hash, not the content hash: a block the client stores
		// with a property this row was written before does not match, so the
		// push that carries it is asked for it. Both halves are columns here,
		// so the fold costs nothing — see model.BlockIdentity.RecordHash.
		hashes[key] = model.ComputeRecordHash(sb.ContentHash, sb.ContextHash)
	}

	view.SetBlockHashes(ctx, itemName, hashes)
	return hashes, nil
}
