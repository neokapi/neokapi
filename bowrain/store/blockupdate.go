package store

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// UpdateBlock reads one block, holds its row until the transaction ends, and
// stores what update leaves on it. See store.BlockStore.
//
// The row is read FOR UPDATE, so another write to the block waits for this one,
// and the targets are read on the same transaction. What update judges is the
// block as it is written.
func (s *PostgresStore) UpdateBlock(ctx context.Context, projectID, stream, blockID string, update func(*venue.StoredBlock) error) (*venue.StoredBlock, error) {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// A block write shares the stream's lock, taken before any row (see lockStream).
	if err := lockStream(ctx, tx, projectID, stream, false); err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, overlays, stored_at, updated_at
		 FROM blocks WHERE project_id=$1 AND stream=$2 AND id=$3
		 FOR UPDATE`, projectID, stream, blockID)
	sb, err := scanStoredBlockPg(row)
	if err != nil {
		return nil, fmt.Errorf("block %s not found in project %s stream %s", blockID, projectID, stream)
	}
	if err := HydrateOverlays(ctx, tx, "pg", projectID, stream, []*venue.StoredBlock{sb}); err != nil {
		return nil, err
	}

	if err := update(sb); err != nil {
		return sb, err
	}
	if err := storeBlocksTx(ctx, tx, projectID, stream, "", []*model.Block{sb.Block}, nil); err != nil {
		return sb, err
	}
	if err := tx.Commit(); err != nil {
		return sb, fmt.Errorf("commit block update: %w", err)
	}
	return sb, nil
}
