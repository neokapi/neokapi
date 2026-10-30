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
	for _, b := range blocks {
		s.publish(ctx, platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"block_id": b.ID, "item_name": itemName},
		})
	}
	if n > 0 {
		s.publish(ctx, platev.Event{
			Type:      platev.EventBlockUpdated,
			Source:    "store",
			ProjectID: projectID,
			Data:      map[string]string{"item_name": itemName},
		})
	}
	return n, nil
}
