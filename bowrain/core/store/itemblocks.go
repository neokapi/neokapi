package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/venue"
)

// ItemBlockReplacer is the part of a ContentStore that ReplaceItemBlocks reads
// and writes through. ContentStore and the event-emitting decorator satisfy
// it.
type ItemBlockReplacer interface {
	GetItem(ctx context.Context, projectID, stream, itemName string) (*Item, error)
	GetBlocks(ctx context.Context, query BlockQuery) ([]*venue.StoredBlock, error)
	StoreBlocksForItem(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) error
	PruneItemBlocks(ctx context.Context, projectID, stream, itemName string, keep []string) (int, error)
}

// ItemBlockReplaceStore is the optional capability of replacing an item's
// block set in one transaction, under the stream's shared write lock, so a
// push to the stream lands entirely before the replacement reads the item or
// entirely after it commits. ReplaceItemBlocks uses it when the store has it.
type ItemBlockReplaceStore interface {
	ReplaceItemBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) (int, error)
}

// ReplaceItemBlocks makes blocks, a fresh read of one item's file, the item's
// whole block set, and reports how many stored blocks it removed. The item
// row must exist before the call when blocks is not empty.
//
// Each block is first given the durable key a push would give it
// (ResolveItemBlockKeys), resolved against the item's stored rows. A block
// that matches a row updates that row in place: the row keeps its id, its
// translations, its annotations and the decisions recorded on it. A block that
// matches nothing adds a row. A row no block matched is removed with
// everything scoped to it. An empty blocks slice removes every block of the
// item.
func ReplaceItemBlocks(ctx context.Context, bs ItemBlockReplacer, projectID, stream, itemName string, blocks []*model.Block) (int, error) {
	if itemName == "" {
		return 0, errors.New("replace item blocks: no item name")
	}
	if rs, ok := bs.(ItemBlockReplaceStore); ok {
		return rs.ReplaceItemBlocks(ctx, projectID, stream, itemName, blocks)
	}
	return ReplaceItemBlocksInSteps(ctx, bs, projectID, stream, itemName, blocks)
}

// ReplaceItemBlocksInSteps is ReplaceItemBlocks for a store without
// ItemBlockReplaceStore: a read, a store and a prune, each in its own
// transaction. The store runs before the prune, so a failure between the two
// leaves surplus rows for the next read to remove. A write to the item between
// the read and the prune can be removed by the prune.
func ReplaceItemBlocksInSteps(ctx context.Context, bs ItemBlockReplacer, projectID, stream, itemName string, blocks []*model.Block) (int, error) {
	if itemName == "" {
		return 0, errors.New("replace item blocks: no item name")
	}
	if len(blocks) > 0 {
		item, err := bs.GetItem(ctx, projectID, stream, itemName)
		if err != nil {
			return 0, fmt.Errorf("read item %q: %w", itemName, err)
		}
		rows, err := bs.GetBlocks(ctx, BlockQuery{ProjectID: projectID, Stream: stream, ItemName: itemName})
		if err != nil {
			return 0, fmt.Errorf("read the blocks of %q: %w", itemName, err)
		}
		priors := make([]reconcile.Prior, 0, len(rows))
		for _, sb := range rows {
			if sb == nil || sb.Block == nil {
				continue
			}
			priors = append(priors, ItemBlockPrior(item.ID, sb.SourceID, sb.Block.ID, sb.ContentHash, sb.ContextHash))
		}
		ResolveItemBlockKeys(item.ID, blocks, priors)
	}
	keep := ItemBlockKeys(itemName, blocks)
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

// ItemBlockPrior is one stored row of an item as a reconcile prior, in the
// shape the venue's tree serves it to a pushing producer
// (venue.Tree.Priors): keyed by the row's source id, or by its row id for a
// row stored without one, and scoped to the item's id.
func ItemBlockPrior(itemID, sourceID, rowID, contentHash, contextHash string) reconcile.Prior {
	key := sourceID
	if key == "" {
		key = rowID
	}
	return reconcile.Prior{Key: key, Scope: itemID, ContentHash: contentHash, ContextHash: contextHash}
}

// ResolveItemBlockKeys sets each block's Key to the durable key a push
// resolves for it (host.ResolveIdentity): the key of the prior it matches, or
// a key minted under the item's scope when it matches none. A read that sets
// no key is otherwise keyed by its reader names, which never match the rows a
// push stored.
func ResolveItemBlockKeys(itemID string, blocks []*model.Block, priors []reconcile.Prior) {
	for _, r := range reconcile.Blocks(itemID, blocks, priors) {
		if r.Block != nil {
			r.Block.Key = r.Key
		}
	}
}

// ItemBlockKeys is the key set a push declares for an item that reads to
// blocks (venue.ItemBlockKeys). Take it before the blocks are stored: storing
// rewrites each block's ID to its row id.
func ItemBlockKeys(itemName string, blocks []*model.Block) []string {
	return venue.ItemBlockKeys(map[string][]*model.Block{itemName: blocks})[itemName]
}
