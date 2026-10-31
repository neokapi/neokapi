package connector

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// FetchStore is the part of a ContentStore that StoreFetched writes through.
type FetchStore interface {
	store.ItemBlockReplacer
	StoreItem(ctx context.Context, projectID, stream string, item *store.Item) error
}

// StoreFetched stores what a connector fetched into a project's stream, one
// item per ContentItem (ItemName). Each item's row is stored and its blocks
// become the item's whole block set (store.ReplaceItemBlocks), as an editor
// upload and a push store a file. A block keeps its row, its translations and
// its decisions across fetches and uploads of the same item, and a block the
// fetched content no longer has is removed. A ContentItem with no blocks adds
// no item and empties an item stored from an earlier fetch.
//
// Each item stores in its own transaction, so a partial failure heals on the
// next fetch.
func StoreFetched(ctx context.Context, s FetchStore, projectID, stream string, items []*ContentItem) error {
	for _, item := range items {
		name := ItemName(item)
		if name == "" {
			continue // nothing addressable
		}
		if len(item.Blocks) == 0 {
			if _, err := store.ReplaceItemBlocks(ctx, s, projectID, stream, name, nil); err != nil {
				return fmt.Errorf("store fetched blocks: %w", err)
			}
			continue
		}
		if err := s.StoreItem(ctx, projectID, stream, &store.Item{
			Name:       name,
			Format:     item.Format,
			ItemType:   "file",
			Properties: map[string]string{},
		}); err != nil {
			return fmt.Errorf("store fetched item %q: %w", name, err)
		}
		if _, err := store.ReplaceItemBlocks(ctx, s, projectID, stream, name, item.Blocks); err != nil {
			return fmt.Errorf("store fetched blocks: %w", err)
		}
	}
	return nil
}

// ItemName is the store item name for a fetched ContentItem: the
// source-relative Path when the connector reports one (file, git, forge),
// else the item's Name, else its ID (a CMS connector addresses content by ID).
// A Path is unique, and delivery derives target paths from it.
func ItemName(item *ContentItem) string {
	if item == nil {
		return ""
	}
	if item.Path != "" {
		return item.Path
	}
	if item.Name != "" {
		return item.Name
	}
	return item.ID
}
