package store

import (
	"context"
	"database/sql"
	"fmt"
)

// Block workflow statuses (ABAC attribute on content).
const (
	BlockStatusDraft     = "draft"
	BlockStatusInReview  = "in_review"
	BlockStatusPublished = "published"
)

// ValidBlockStatuses is the set of acceptable status values.
var ValidBlockStatuses = map[string]bool{
	BlockStatusDraft:     true,
	BlockStatusInReview:  true,
	BlockStatusPublished: true,
}

// GetBlockStatus returns a block's workflow status and owner. A missing block
// reports draft/empty (so callers treat unknown content as freely editable).
func (s *PostgresStore) GetBlockStatus(ctx context.Context, projectID, blockID string) (status, ownerID string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT status, owner_id FROM blocks WHERE project_id = $1 AND id = $2`, projectID, blockID).
		Scan(&status, &ownerID)
	if err == sql.ErrNoRows {
		return BlockStatusDraft, "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("get block status: %w", err)
	}
	if status == "" {
		status = BlockStatusDraft
	}
	return status, ownerID, nil
}

// SetBlockStatus updates a block's workflow status and (when non-empty) its
// owner. Returns an error if the block does not exist.
func (s *PostgresStore) SetBlockStatus(ctx context.Context, projectID, blockID, status, ownerID string) error {
	var res sql.Result
	var err error
	if ownerID != "" {
		res, err = s.db.ExecContext(ctx,
			`UPDATE blocks SET status = $1, owner_id = $2, updated_at = NOW() WHERE project_id = $3 AND id = $4`,
			status, ownerID, projectID, blockID)
	} else {
		res, err = s.db.ExecContext(ctx,
			`UPDATE blocks SET status = $1, updated_at = NOW() WHERE project_id = $2 AND id = $3`,
			status, projectID, blockID)
	}
	if err != nil {
		return fmt.Errorf("set block status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("block %s not found", blockID)
	}
	return nil
}
