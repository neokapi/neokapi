package store

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// ItemBlockReplacer is the part of a BlockStore that ReplaceItemBlocks writes
// through. ContentStore, PushApplier and the event-emitting decorator all
// satisfy it.
type ItemBlockReplacer interface {
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error)
}

// ReplaceItemBlocks makes blocks the whole block set of one item: the set a
// fresh read of the item's file produced. It reports how many stored blocks it
// removed.
//
// A block whose key the item already holds updates that row in place, so the
// row keeps its id, its translations, its overlays and the decisions recorded
// on it. A block with a new key adds a row. A stored block whose key the read
// no longer produces is removed with everything scoped to it. The keys are the
// ones a push declares for the same purpose (venue.ItemBlockKeys), so an
// upload and a push of the same read agree on which blocks are gone.
//
// An empty blocks slice removes every block of the item: the file read to
// nothing. The store happens before the prune, so a failure between the two
// leaves surplus rows for the next read to remove and never loses one that
// the read still has.
func ReplaceItemBlocks(ctx context.Context, bs ItemBlockReplacer, projectID, stream, itemName string, blocks []*model.Block) (int, error) {
	if itemName == "" {
		return 0, fmt.Errorf("replace item blocks: no item name")
	}
	// The keys are taken before the store: storing an item's blocks rewrites
	// each block's ID to the row's internal id, and a block that carries no key
	// or name is keyed by the ID the reader gave it.
	keep := venue.ItemBlockKeys(map[string][]*model.Block{itemName: blocks})[itemName]
	if len(blocks) > 0 {
		if err := bs.StoreBlocksForItem(ctx, projectID, stream, itemName, blocks); err != nil {
			return 0, fmt.Errorf("store blocks for %q: %w", itemName, err)
		}
	}
	n, err := bs.PruneItemBlocks(ctx, projectID, stream, itemName, keep)
	if err != nil {
		return 0, fmt.Errorf("remove the blocks %q no longer has: %w", itemName, err)
	}
	return n, nil
}
