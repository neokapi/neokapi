package event

import (
	"context"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

var _ store.ItemBlockReplaceStore = (*EventEmittingStore)(nil)

// ReplaceItemBlocks forwards the optional ItemBlockReplaceStore capability, so
// a replacement through this decorator keeps its one transaction. A store
// without the capability replaces in steps through this decorator, which
// publishes each step's events itself.
//
// After a replacement commits it publishes what StoreBlocksForItem and
// PruneItemBlocks publish: block.updated for each stored block, and one
// block.updated naming the item when the replacement removed blocks.
func (s *EventEmittingStore) ReplaceItemBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) (int, error) {
	rs, ok := s.inner.(store.ItemBlockReplaceStore)
	if !ok {
		return store.ReplaceItemBlocksInSteps(ctx, s, projectID, stream, itemName, blocks)
	}
	n, err := rs.ReplaceItemBlocks(ctx, projectID, stream, itemName, blocks)
	if err != nil {
		return n, err
	}
	s.publishReplacedBlocks(ctx, projectID, itemName, blocks, n)
	return n, nil
}

var _ store.ItemReplaceStore = (*EventEmittingStore)(nil)

// ReplaceItem forwards the optional ItemReplaceStore capability, so an item
// and its block set store in one transaction with one item write. A store
// without the capability replaces in steps through this decorator, which
// publishes each step's events itself.
//
// After a replacement commits it publishes one item.created for the item and
// the block events ReplaceItemBlocks publishes.
func (s *EventEmittingStore) ReplaceItem(ctx context.Context, projectID, stream string, item *store.Item, blocks []*model.Block, render func() string) (int, error) {
	rs, ok := s.inner.(store.ItemReplaceStore)
	if !ok {
		return store.ReplaceItemInSteps(ctx, s, projectID, stream, item, blocks, render)
	}
	n, err := rs.ReplaceItem(ctx, projectID, stream, item, blocks, render)
	if err != nil {
		return n, err
	}
	s.publish(ctx, platev.Event{
		Type:      platev.EventItemCreated,
		Source:    "store",
		ProjectID: projectID,
		Data:      map[string]string{"item_name": item.Name, "stream": stream, "format": item.Format},
	})
	s.publishReplacedBlocks(ctx, projectID, item.Name, blocks, n)
	return n, nil
}

// publishReplacedBlocks publishes what StoreBlocksForItem and PruneItemBlocks
// publish: block.updated for each stored block, and one block.updated naming
// the item when the replacement removed blocks.
func (s *EventEmittingStore) publishReplacedBlocks(ctx context.Context, projectID, itemName string, blocks []*model.Block, removed int) {
	for _, b := range blocks {
		s.publish(ctx, platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"block_id": b.ID, "item_name": itemName},
		})
	}
	if removed > 0 {
		s.publish(ctx, platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"item_name": itemName},
		})
	}
}
