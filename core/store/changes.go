package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MaxChangesPerRequest is the maximum number of change entries returned per query.
const MaxChangesPerRequest = 1000

// GetChanges returns change log entries for a project since the given cursor.
// If locale is non-empty, only source changes and target changes for that locale
// are returned. The returned ChangeSet includes a NewCursor for pagination and
// HasMore to indicate additional results.
func (s *SQLiteStore) GetChanges(ctx context.Context, projectID string, sinceCursor int64, locale string, limit int) (*ChangeSet, error) {
	if limit <= 0 || limit > MaxChangesPerRequest {
		limit = MaxChangesPerRequest
	}

	var query string
	var args []any
	if locale != "" {
		// Locale-scoped: only source changes + target changes for this locale.
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = ? AND seq > ?
				   AND (locale IS NULL OR locale = ?)
				 ORDER BY seq ASC
				 LIMIT ?`
		args = []any{projectID, sinceCursor, locale, limit + 1}
	} else {
		// All changes.
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = ? AND seq > ?
				 ORDER BY seq ASC
				 LIMIT ?`
		args = []any{projectID, sinceCursor, limit + 1}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query change log: %w", err)
	}
	defer rows.Close()

	var entries []ChangeEntry
	for rows.Next() {
		var e ChangeEntry
		var loggedStr string
		if err := rows.Scan(&e.Seq, &e.BlockID, &e.ChangeType, &e.Locale, &e.ContentHash, &loggedStr); err != nil {
			return nil, fmt.Errorf("scan change entry: %w", err)
		}
		e.LoggedAt, _ = time.Parse(time.RFC3339, loggedStr)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate change log: %w", err)
	}

	cs := &ChangeSet{}
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
func (s *SQLiteStore) LatestCursor(ctx context.Context, projectID string) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM change_log WHERE project_id = ?`,
		projectID).Scan(&cursor)
	if err != nil {
		return 0, fmt.Errorf("query latest cursor: %w", err)
	}
	return cursor, nil
}

// CompactChangeLog removes old change log entries, keeping only the latest
// entry per (project_id, block_id, locale) combination older than retainDays.
// Returns the number of entries deleted.
func (s *SQLiteStore) CompactChangeLog(ctx context.Context, projectID string, retainDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retainDays).Format(time.RFC3339)

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM change_log
		WHERE project_id = ? AND logged_at <= ?
		  AND seq NOT IN (
			SELECT MAX(seq) FROM change_log
			WHERE project_id = ?
			GROUP BY block_id, COALESCE(locale, '')
		  )`, projectID, cutoff, projectID)
	if err != nil {
		return 0, fmt.Errorf("compact change log: %w", err)
	}

	deleted, _ := result.RowsAffected()
	return deleted, nil
}

// logChange inserts a single change log entry within a transaction.
func logChange(ctx context.Context, tx *sql.Tx, projectID, blockID, changeType, locale, contentHash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var localeVal any
	if locale == "" {
		localeVal = nil
	} else {
		localeVal = locale
	}
	var hashVal any
	if contentHash == "" {
		hashVal = nil
	} else {
		hashVal = contentHash
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO change_log (project_id, block_id, change_type, locale, content_hash, logged_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, blockID, changeType, localeVal, hashVal, now)
	return err
}
