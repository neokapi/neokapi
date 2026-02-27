package store

import (
	"context"
	"fmt"

	platstore "github.com/gokapi/gokapi/platform/store"
)

// GetBlockHistory returns history entries for a block in a specific locale.
func (s *PostgresStore) GetBlockHistory(ctx context.Context, projectID, blockID string, locale string, limit int) ([]platstore.BlockHistoryEntry, error) {
	if limit <= 0 || limit > MaxHistoryEntries {
		limit = MaxHistoryEntries
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, change_type, text, coded_text, origin, author, created_at
		 FROM block_history
		 WHERE project_id = $1 AND block_id = $2 AND locale = $3
		 ORDER BY id DESC
		 LIMIT $4`,
		projectID, blockID, locale, limit)
	if err != nil {
		return nil, fmt.Errorf("query block history: %w", err)
	}
	defer rows.Close()

	var entries []platstore.BlockHistoryEntry
	for rows.Next() {
		var e platstore.BlockHistoryEntry
		if err := rows.Scan(&e.Seq, &e.ChangeType, &e.Text, &e.CodedText, &e.Origin, &e.Author, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("scan block history entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate block history: %w", err)
	}

	if entries == nil {
		entries = []platstore.BlockHistoryEntry{}
	}
	return entries, nil
}
