package store

import (
	"context"
	"fmt"
)

// TargetRevert describes how to restore one (block, locale) target to the value
// it held before a batch (correlation) of changes.
type TargetRevert struct {
	BlockID string
	Locale  string
	Text    string // prior text to restore (empty when Clear)
	Coded   string // prior coded runs JSON, if any
	Clear   bool   // the target was first created in the batch → revert blanks it
}

// ComputeBatchReverts returns, for every (block, locale) target changed under a
// correlation id (a push/import/request batch), the value it had immediately
// before that batch — i.e. what reverting the batch should restore. A target
// first created by the batch has Clear=true (no prior value). This is read-only;
// callers apply the reverts through the normal StoreBlocks path so the revert is
// itself recorded in history.
func (s *PostgresStore) ComputeBatchReverts(ctx context.Context, projectID, stream, correlationID string) ([]TargetRevert, error) {
	stream = defaultStream(stream)
	if correlationID == "" {
		return nil, fmt.Errorf("correlation_id is required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.block_id, b.locale,
		   COALESCE((SELECT h.text FROM block_history h
		             WHERE h.project_id=$1 AND h.stream=$2 AND h.block_id=b.block_id AND h.locale=b.locale AND h.id < b.first_id
		             ORDER BY h.id DESC LIMIT 1), '') AS prior_text,
		   COALESCE((SELECT h.coded_text FROM block_history h
		             WHERE h.project_id=$1 AND h.stream=$2 AND h.block_id=b.block_id AND h.locale=b.locale AND h.id < b.first_id
		             ORDER BY h.id DESC LIMIT 1), '') AS prior_coded,
		   NOT EXISTS(SELECT 1 FROM block_history h
		             WHERE h.project_id=$1 AND h.stream=$2 AND h.block_id=b.block_id AND h.locale=b.locale AND h.id < b.first_id) AS clear
		 FROM (
		   SELECT block_id, locale, MIN(id) AS first_id
		   FROM block_history
		   WHERE project_id=$1 AND stream=$2 AND correlation_id=$3
		   GROUP BY block_id, locale
		 ) b`,
		projectID, stream, correlationID)
	if err != nil {
		return nil, fmt.Errorf("compute batch reverts: %w", err)
	}
	defer rows.Close()

	var out []TargetRevert
	for rows.Next() {
		var r TargetRevert
		if err := rows.Scan(&r.BlockID, &r.Locale, &r.Text, &r.Coded, &r.Clear); err != nil {
			return nil, fmt.Errorf("scan revert: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
