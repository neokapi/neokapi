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
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit block replacement for item %q: %w", itemName, err)
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
