package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/editor"
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

// ItemWriter is the part of a ContentStore that ReplaceItem and StoreReadItem
// write through.
type ItemWriter interface {
	ItemBlockReplacer
	StoreItem(ctx context.Context, projectID, stream string, item *Item) error
}

// ItemReplaceStore is the optional capability of storing an item row and
// replacing its block set in one transaction, under the stream's shared write
// lock, with one item write. ReplaceItem uses it when the store has it.
type ItemReplaceStore interface {
	ReplaceItem(ctx context.Context, projectID, stream string, item *Item, blocks []*model.Block, render func() string) (int, error)
}

// ReplaceItem stores item and makes blocks, a fresh read of the item's file,
// its whole block set (ReplaceItemBlocks), and reports how many stored blocks
// it removed. A non-nil render is called once the blocks' keys are resolved,
// and its result is stored as the item's preview and set on
// item.PreviewHTML. A preview rendered then marks each block by the key its
// row is stored under as source id.
func ReplaceItem(ctx context.Context, w ItemWriter, projectID, stream string, item *Item, blocks []*model.Block, render func() string) (int, error) {
	if item == nil || item.Name == "" {
		return 0, errors.New("replace item: no item name")
	}
	if rs, ok := w.(ItemReplaceStore); ok {
		return rs.ReplaceItem(ctx, projectID, stream, item, blocks, render)
	}
	return ReplaceItemInSteps(ctx, w, projectID, stream, item, blocks, render)
}

// ReplaceItemInSteps is ReplaceItem for a store without ItemReplaceStore. It
// writes the item row, replaces the blocks, and writes the row again with the
// preview when render is not nil, each in its own transaction.
func ReplaceItemInSteps(ctx context.Context, w ItemWriter, projectID, stream string, item *Item, blocks []*model.Block, render func() string) (int, error) {
	if item == nil || item.Name == "" {
		return 0, errors.New("replace item: no item name")
	}
	// The item row comes first: ReplaceItemBlocks scopes the keys it
	// resolves to the item's id.
	if err := w.StoreItem(ctx, projectID, stream, item); err != nil {
		return 0, fmt.Errorf("store item %q: %w", item.Name, err)
	}
	n, err := ReplaceItemBlocks(ctx, w, projectID, stream, item.Name, blocks)
	if err != nil {
		return n, err
	}
	if render == nil {
		return n, nil
	}
	item.PreviewHTML = render()
	if err := w.StoreItem(ctx, projectID, stream, item); err != nil {
		return n, fmt.Errorf("store the preview of %q: %w", item.Name, err)
	}
	return n, nil
}

// StoreReadItem stores item with read, a parse of the item's file, as its
// whole block set and its preview (ReplaceItem).
func StoreReadItem(ctx context.Context, w ItemWriter, projectID, stream string, item *Item, read *editor.ParseResult) error {
	_, err := ReplaceItem(ctx, w, projectID, stream, item, read.Blocks, read.PreviewHTML)
	return err
}

// BlockListPreview is the preview of an item that has no stored preview and
// no block index: a listing of its stored translatable blocks
// (editor.BuildGenericPreview). Each block is marked by its source id, the
// key a surface names it by, or by its row id when it has none.
func BlockListPreview(blocks []*venue.StoredBlock) string {
	parts := make([]*model.Part, 0, len(blocks))
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil || !sb.Block.Translatable {
			continue
		}
		b := sb.Block
		if sb.SourceID != "" {
			marked := *b
			marked.ID = sb.SourceID
			b = &marked
		}
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	return editor.BuildGenericPreview(parts)
}
