package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// Block access states (ABAC attribute on content). This is ACCESS CONTROL —
// who may edit — not progress: the review ladder
// (draft → translated → reviewed → signed-off) lives on the per-locale target.
// The two used to share the word "draft", and "in_review" sat one letter from
// "reviewed" on the other side of the same block; the values now name the
// access consequence instead.
const (
	// BlockAccessOpen: normal permissions apply.
	BlockAccessOpen = "open"
	// BlockAccessRestricted: editing requires review permission for the
	// locale, unless the actor owns the block.
	BlockAccessRestricted = "restricted"
	// BlockAccessPublished: re-opening is privileged (manage-project).
	BlockAccessPublished = "published"
)

// ValidBlockAccess is the set of acceptable access values.
var ValidBlockAccess = map[string]bool{
	BlockAccessOpen:       true,
	BlockAccessRestricted: true,
	BlockAccessPublished:  true,
}

// NormalizeBlockAccess maps the retired vocabulary onto the current one, so a
// caller still speaking the old ladder is corrected rather than rejected.
// Unknown values pass through for the validity check to refuse.
func NormalizeBlockAccess(v string) string {
	switch v {
	case "draft":
		return BlockAccessOpen
	case "in_review":
		return BlockAccessRestricted
	default:
		return v
	}
}

// GetBlockAccess returns a block's access state and owner. A missing block
// reports open/empty (so callers treat unknown content as freely editable).
func (s *PostgresStore) GetBlockAccess(ctx context.Context, projectID, stream, blockID string) (access, ownerID string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT access, owner_id FROM blocks WHERE project_id = $1 AND stream = $2 AND id = $3`,
		projectID, storeutil.DefaultStream(stream), blockID).
		Scan(&access, &ownerID)
	if err == sql.ErrNoRows {
		return BlockAccessOpen, "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("get block access: %w", err)
	}
	if access == "" {
		access = BlockAccessOpen
	}
	return access, ownerID, nil
}

// SetBlockAccess updates a block's access state and (when non-empty) its
// owner. Returns an error if the block does not exist.
func (s *PostgresStore) SetBlockAccess(ctx context.Context, projectID, stream, blockID, access, ownerID string) error {
	stream = storeutil.DefaultStream(stream)
	var res sql.Result
	var err error
	if ownerID != "" {
		res, err = s.db.ExecContext(ctx,
			`UPDATE blocks SET access = $1, owner_id = $2, updated_at = NOW() WHERE project_id = $3 AND stream = $4 AND id = $5`,
			access, ownerID, projectID, stream, blockID)
	} else {
		res, err = s.db.ExecContext(ctx,
			`UPDATE blocks SET access = $1, updated_at = NOW() WHERE project_id = $2 AND stream = $3 AND id = $4`,
			access, projectID, stream, blockID)
	}
	if err != nil {
		return fmt.Errorf("set block access: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("block %s not found", blockID)
	}
	return nil
}
