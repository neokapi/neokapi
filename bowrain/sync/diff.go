package sync

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// DiffEngine computes the difference between client and server state
// using the Merkle tree hash model (AD-038).
type DiffEngine struct {
	contentStore platstore.ContentStore
	cache        HashCache // optional; nil = always query DB
}

// HashCache provides cached access to project content hashes.
// Implementations: RedisHashCache (production), nil (fallback to DB).
type HashCache interface {
	// GetItemHashes returns all item_name → item_hash for a project.
	// Returns nil, false on cache miss.
	GetItemHashes(ctx context.Context, projectID string) (map[string]string, bool)

	// GetBlockHashes returns all block_id → content_hash for an item.
	// Returns nil, false on cache miss.
	GetBlockHashes(ctx context.Context, projectID, itemName string) (map[string]string, bool)

	// SetItemHashes caches item hashes for a project.
	SetItemHashes(ctx context.Context, projectID string, hashes map[string]string)

	// SetBlockHashes caches block hashes for an item.
	SetBlockHashes(ctx context.Context, projectID, itemName string, hashes map[string]string)

	// InvalidateProject removes all cached hashes for a project.
	InvalidateProject(ctx context.Context, projectID string)
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

	// Whether terms/TM collections changed.
	TermsChanged bool
	TMChanged    bool
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
	serverHashes, err := d.loadBlockHashes(ctx, projectID, stream, itemName)
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
	serverRoot := ComputeRootHash(serverHashes)
	return clientRootHash == serverRoot, nil
}

// loadItemHashes loads item-level hashes (item_name → hash of block hashes).
// Uses cache if available, falls back to computing from DB.
func (d *DiffEngine) loadItemHashes(ctx context.Context, projectID, stream string) (map[string]string, error) {
	// Try cache first.
	if d.cache != nil {
		if cached, ok := d.cache.GetItemHashes(ctx, projectID); ok {
			return cached, nil
		}
	}

	// Load items from DB.
	items, err := d.contentStore.ListItems(ctx, projectID, stream)
	if err != nil {
		return nil, err
	}

	itemHashes := make(map[string]string, len(items))
	for _, item := range items {
		blockHashes, err := d.loadBlockHashes(ctx, projectID, stream, item.Name)
		if err != nil {
			return nil, err
		}
		itemHashes[item.Name] = ComputeItemHash(blockHashes)
	}

	// Cache the result.
	if d.cache != nil {
		d.cache.SetItemHashes(ctx, projectID, itemHashes)
	}

	return itemHashes, nil
}

// loadBlockHashes loads block-level hashes for a single item.
func (d *DiffEngine) loadBlockHashes(ctx context.Context, projectID, stream, itemName string) (map[string]string, error) {
	// Try cache first.
	if d.cache != nil {
		if cached, ok := d.cache.GetBlockHashes(ctx, projectID, itemName); ok {
			return cached, nil
		}
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
		hashes[key] = sb.ContentHash
	}

	// Cache.
	if d.cache != nil {
		d.cache.SetBlockHashes(ctx, projectID, itemName, hashes)
	}

	return hashes, nil
}
