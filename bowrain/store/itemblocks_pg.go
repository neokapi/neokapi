package store

import (
	"context"
	"errors"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
)

var _ platstore.ItemBlockReplaceStore = (*PostgresStore)(nil)

// ReplaceItemBlocks makes blocks one item's whole block set. See
// platstore.ReplaceItemBlocks for what it does to each row.
//
// The read of the item's rows, the keys resolved against them, the store and
// the prune share one transaction under the stream's shared write lock. A push
// takes that lock exclusively, so it commits before the read here or waits
// until this commits, and the prune never removes a row the push wrote.
func (s *PostgresStore) ReplaceItemBlocks(ctx context.Context, projectID, stream, itemName string, blocks []*model.Block) (int, error) {
	if itemName == "" {
		return 0, errors.New("replace item blocks: no item name")
	}
	stream = storeutil.DefaultStream(stream)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockStream(ctx, tx, projectID, stream, false); err != nil {
		return 0, err
	}
	n, err := replaceItemBlocksTx(ctx, tx, projectID, stream, itemName, blocks)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit block replacement for item %q: %w", itemName, err)
	}
	return n, nil
}

var _ platstore.ItemReplaceStore = (*PostgresStore)(nil)

// ReplaceItem stores item and makes blocks its whole block set. See
// platstore.ReplaceItem.
//
// The item row, the block replacement and the preview share one transaction
// under the stream's shared write lock, as in ReplaceItemBlocks. The preview
// is rendered after the keys are resolved and written by an update of the
// row, so no reader sees the row without its preview.
func (s *PostgresStore) ReplaceItem(ctx context.Context, projectID, stream string, item *platstore.Item, blocks []*model.Block, render func() string) (int, error) {
	if item == nil || item.Name == "" {
		return 0, errors.New("replace item: no item name")
	}
	stream = storeutil.DefaultStream(stream)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockStream(ctx, tx, projectID, stream, false); err != nil {
		return 0, err
	}
	if err := storeItemTx(ctx, tx, projectID, stream, item); err != nil {
		return 0, err
	}
	n, err := replaceItemBlocksTx(ctx, tx, projectID, stream, item.Name, blocks)
	if err != nil {
		return 0, err
	}
	if render != nil {
		item.PreviewHTML = render()
		if _, err := tx.ExecContext(ctx,
			`UPDATE items SET preview_html=$1 WHERE project_id=$2 AND stream=$3 AND name=$4`,
			item.PreviewHTML, projectID, stream, item.Name); err != nil {
			return 0, fmt.Errorf("store the preview of %q: %w", item.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit item replacement for %q: %w", item.Name, err)
	}
	return n, nil
}

// replaceItemBlocksTx resolves blocks' keys against the item's rows, stores
// them and prunes the rows none matched, on the caller's transaction.
func replaceItemBlocksTx(ctx context.Context, tx Runner, projectID, stream, itemName string, blocks []*model.Block) (int, error) {
	if len(blocks) > 0 {
		itemID, err := resolveItemIDPg(ctx, tx, projectID, stream, itemName)
		if err != nil {
			return 0, err
		}
		priors, err := itemBlockPriorsPg(ctx, tx, projectID, stream, itemName, itemID)
		if err != nil {
			return 0, err
		}
		platstore.ResolveItemBlockKeys(itemID, blocks, priors)
	}
	keep := platstore.ItemBlockKeys(itemName, blocks)
	if len(blocks) > 0 {
		if err := storeBlocksTx(ctx, tx, projectID, stream, itemName, blocks, nil); err != nil {
			return 0, fmt.Errorf("store blocks for %q: %w", itemName, err)
		}
	}
	n, err := pruneItemBlocksTx(ctx, tx, projectID, stream, itemName, keep)
	if err != nil {
		return 0, fmt.Errorf("remove the blocks %q no longer has: %w", itemName, err)
	}
	return n, nil
}

// itemBlockPriorsPg reads one item's rows as the priors a push resolves
// against, in the order the item's blocks are read.
func itemBlockPriorsPg(ctx context.Context, tx Runner, projectID, stream, itemName, itemID string) ([]reconcile.Prior, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, COALESCE(source_id, ''), COALESCE(content_hash, ''), COALESCE(context_hash, '') FROM blocks
		 WHERE project_id=$1 AND stream=$2 AND item_name=$3
		 ORDER BY position, id`,
		projectID, stream, itemName)
	if err != nil {
		return nil, fmt.Errorf("load the blocks of %q: %w", itemName, err)
	}
	defer rows.Close()
	var priors []reconcile.Prior
	for rows.Next() {
		var id, sourceID, contentHash, contextHash string
		if err := rows.Scan(&id, &sourceID, &contentHash, &contextHash); err != nil {
			return nil, fmt.Errorf("scan a block of %q: %w", itemName, err)
		}
		priors = append(priors, platstore.ItemBlockPrior(itemID, sourceID, id, contentHash, contextHash))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("the blocks of %q: %w", itemName, err)
	}
	return priors, nil
}
