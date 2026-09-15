package sqlitestore

import (
	"context"
	"fmt"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// UpdateBlock reads one block and stores what update leaves on it, with no
// other write landing in between. See store.BlockStore.
//
// The transaction writes before it reads: the no-op update takes the database's
// write lock, so the block update judges is the block it writes.
func (s *SQLiteStore) UpdateBlock(ctx context.Context, projectID, stream, blockID string, update func(*venue.StoredBlock) error) (*venue.StoredBlock, error) {
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE blocks SET updated_at = updated_at WHERE project_id=? AND stream=? AND id=?`,
		projectID, stream, blockID)
	if err != nil {
		return nil, fmt.Errorf("hold block %s: %w", blockID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	row := tx.QueryRowContext(ctx,
		`SELECT id, project_id, item_name, source_id, name, type, mime_type, translatable, content_hash, context_hash,
			source_json, properties, overlays, stored_at, updated_at
		 FROM blocks WHERE project_id=? AND stream=? AND id=?`, projectID, stream, blockID)
	sb, err := scanStoredBlock(row)
	if err != nil {
		return nil, fmt.Errorf("block %s not found in project %s", blockID, projectID)
	}
	if err := bstore.HydrateOverlays(ctx, tx, "sqlite", projectID, stream, []*venue.StoredBlock{sb}); err != nil {
		return nil, err
	}

	if err := update(sb); err != nil {
		return sb, err
	}
	if err := s.storeBlocksTx(ctx, tx, projectID, stream, "", []*model.Block{sb.Block}, nil); err != nil {
		return sb, err
	}
	if err := tx.Commit(); err != nil {
		return sb, fmt.Errorf("commit block update: %w", err)
	}
	return sb, nil
}
