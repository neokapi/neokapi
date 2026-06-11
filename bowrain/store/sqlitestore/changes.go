package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// MaxChangesPerRequest is the maximum number of change entries returned per query.
const MaxChangesPerRequest = 1000

// GetChanges returns change log entries for a project since the given cursor.
// If locales is non-empty, only source changes (locale IS NULL) and target
// changes for the specified locales are returned. The returned ChangeSet
// includes a NewCursor for pagination and HasMore to indicate additional results.
func (s *SQLiteStore) GetChanges(ctx context.Context, projectID, stream string, sinceCursor int64, locales []string, limit int) (*platstore.ChangeSet, error) {
	stream = storeutil.DefaultStream(stream)
	if limit <= 0 || limit > MaxChangesPerRequest {
		limit = MaxChangesPerRequest
	}

	var query string
	var args []any
	if len(locales) > 0 {
		// Locale-scoped: only source changes + target changes for these locales.
		placeholders := make([]string, len(locales))
		for i := range locales {
			placeholders[i] = "?"
		}
		inClause := strings.Join(placeholders, ", ")
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = ? AND stream = ? AND seq > ?
				   AND (locale IS NULL OR locale IN (` + inClause + `))
				 ORDER BY seq ASC
				 LIMIT ?`
		args = []any{projectID, stream, sinceCursor}
		for _, loc := range locales {
			args = append(args, loc)
		}
		args = append(args, limit+1)
	} else {
		// All changes.
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = ? AND stream = ? AND seq > ?
				 ORDER BY seq ASC
				 LIMIT ?`
		args = []any{projectID, stream, sinceCursor, limit + 1}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query change log: %w", err)
	}
	defer rows.Close()

	var entries []platstore.ChangeEntry
	for rows.Next() {
		var e platstore.ChangeEntry
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

// LatestCursor returns the most recent change log sequence number for a project stream.
func (s *SQLiteStore) LatestCursor(ctx context.Context, projectID, stream string) (int64, error) {
	stream = storeutil.DefaultStream(stream)
	var cursor int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM change_log WHERE project_id = ? AND stream = ?`,
		projectID, stream).Scan(&cursor)
	if err != nil {
		return 0, fmt.Errorf("query latest cursor: %w", err)
	}
	return cursor, nil
}

// CompactChangeLog removes old change log entries, keeping only the latest
// entry per (project_id, block_id, locale) combination older than retainDays.
// Returns the number of entries deleted.
func (s *SQLiteStore) CompactChangeLog(ctx context.Context, projectID, stream string, retainDays int) (int64, error) {
	stream = storeutil.DefaultStream(stream)
	cutoff := time.Now().AddDate(0, 0, -retainDays).Format(time.RFC3339)

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM change_log
		WHERE project_id = ? AND stream = ? AND logged_at <= ?
		  AND seq NOT IN (
			SELECT MAX(seq) FROM change_log
			WHERE project_id = ? AND stream = ?
			GROUP BY block_id, COALESCE(locale, '')
		  )`, projectID, stream, cutoff, projectID, stream)
	if err != nil {
		return 0, fmt.Errorf("compact change log: %w", err)
	}

	deleted, _ := result.RowsAffected()
	return deleted, nil
}

// logChange inserts a single change log entry within a transaction.
func logChange(ctx context.Context, tx *sql.Tx, projectID, stream, blockID, changeType, locale, contentHash string) error {
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
		`INSERT INTO change_log (project_id, stream, block_id, change_type, locale, content_hash, logged_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, stream, blockID, changeType, localeVal, hashVal, now)
	return err
}
