package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	platstore "github.com/gokapi/gokapi/platform/store"
)

// GetChanges returns change log entries for a project since the given cursor.
func (s *PostgresStore) GetChanges(ctx context.Context, projectID string, sinceCursor int64, locales []string, limit int) (*platstore.ChangeSet, error) {
	if limit <= 0 || limit > MaxChangesPerRequest {
		limit = MaxChangesPerRequest
	}

	var query string
	var args []any
	if len(locales) > 0 {
		placeholders := make([]string, len(locales))
		paramN := 3
		for i := range locales {
			placeholders[i] = fmt.Sprintf("$%d", paramN)
			paramN++
		}
		inClause := strings.Join(placeholders, ", ")
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = $1 AND seq > $2
				   AND (locale IS NULL OR locale IN (` + inClause + `))
				 ORDER BY seq ASC
				 LIMIT $` + fmt.Sprintf("%d", paramN)
		args = []any{projectID, sinceCursor}
		for _, loc := range locales {
			args = append(args, loc)
		}
		args = append(args, limit+1)
	} else {
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = $1 AND seq > $2
				 ORDER BY seq ASC
				 LIMIT $3`
		args = []any{projectID, sinceCursor, limit + 1}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query change log: %w", err)
	}
	defer rows.Close()

	var entries []platstore.ChangeEntry
	for rows.Next() {
		var e platstore.ChangeEntry
		if err := rows.Scan(&e.Seq, &e.BlockID, &e.ChangeType, &e.Locale, &e.ContentHash, &e.LoggedAt); err != nil {
			return nil, fmt.Errorf("scan change entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate change log: %w", err)
	}

	cs := &platstore.ChangeSet{}
	if len(entries) > limit {
		cs.HasMore = true
		entries = entries[:limit]
	}
	cs.Changes = entries
	if len(entries) > 0 {
		cs.NewCursor = entries[len(entries)-1].Seq
	} else {
		cs.NewCursor = sinceCursor
	}
	return cs, nil
}

// LatestCursor returns the most recent change log sequence number for a project.
func (s *PostgresStore) LatestCursor(ctx context.Context, projectID string) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM change_log WHERE project_id = $1`,
		projectID).Scan(&cursor)
	if err != nil {
		return 0, fmt.Errorf("query latest cursor: %w", err)
	}
	return cursor, nil
}

// CompactChangeLog removes old change log entries, keeping only the latest
// entry per (project_id, block_id, locale) combination older than retainDays.
func (s *PostgresStore) CompactChangeLog(ctx context.Context, projectID string, retainDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retainDays)

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM change_log
		WHERE project_id = $1 AND logged_at <= $2
		  AND seq NOT IN (
			SELECT MAX(seq) FROM change_log
			WHERE project_id = $1
			GROUP BY block_id, COALESCE(locale, '')
		  )`, projectID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("compact change log: %w", err)
	}

	deleted, _ := result.RowsAffected()
	return deleted, nil
}
