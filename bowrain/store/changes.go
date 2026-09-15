package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// GetChanges returns change log entries for a project since the given cursor.
func (s *PostgresStore) GetChanges(ctx context.Context, projectID, stream string, sinceCursor int64, locales []string, limit int) (*platstore.ChangeSet, error) {
	stream = storeutil.DefaultStream(stream)
	if limit <= 0 || limit > MaxChangesPerRequest {
		limit = MaxChangesPerRequest
	}

	var query string
	var args []any
	if len(locales) > 0 {
		placeholders := make([]string, len(locales))
		paramN := 4
		for i := range locales {
			placeholders[i] = fmt.Sprintf("$%d", paramN)
			paramN++
		}
		inClause := strings.Join(placeholders, ", ")
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = $1 AND stream = $2 AND seq > $3
				   AND (locale IS NULL OR locale IN (` + inClause + `))
				 ORDER BY seq ASC
				 LIMIT $` + strconv.Itoa(paramN)
		args = []any{projectID, stream, sinceCursor}
		for _, loc := range locales {
			args = append(args, loc)
		}
		args = append(args, limit+1)
	} else {
		query = `SELECT seq, block_id, change_type, COALESCE(locale, ''), COALESCE(content_hash, ''), logged_at
				 FROM change_log
				 WHERE project_id = $1 AND stream = $2 AND seq > $3
				 ORDER BY seq ASC
				 LIMIT $4`
		args = []any{projectID, stream, sinceCursor, limit + 1}
	}

	return s.queryChangeSet(ctx, query, args, limit, sinceCursor)
}

// GetLatestChanges returns one entry per block changed since the cursor: its
// latest change within the locale scope, ordered and paged by that change's seq.
func (s *PostgresStore) GetLatestChanges(ctx context.Context, projectID, stream string, sinceCursor int64, locales []string, limit int) (*platstore.ChangeSet, error) {
	stream = storeutil.DefaultStream(stream)
	if limit <= 0 || limit > MaxChangesPerRequest {
		limit = MaxChangesPerRequest
	}

	args := []any{projectID, stream, sinceCursor}
	scope := ""
	if len(locales) > 0 {
		placeholders := make([]string, len(locales))
		for i, loc := range locales {
			args = append(args, loc)
			placeholders[i] = "$" + strconv.Itoa(len(args))
		}
		scope = ` AND (locale IS NULL OR locale IN (` + strings.Join(placeholders, ", ") + `))`
	}
	args = append(args, limit+1)
	// The page is cut inside the aggregate, over the project stream's own rows,
	// so the join looks up at most one page of rows by seq however many other
	// projects share the log.
	query := `SELECT c.seq, c.block_id, c.change_type, COALESCE(c.locale, ''), COALESCE(c.content_hash, ''), c.logged_at
			 FROM (SELECT MAX(seq) AS latest_seq FROM change_log
			       WHERE project_id = $1 AND stream = $2 AND seq > $3` + scope + `
			       GROUP BY block_id
			       ORDER BY latest_seq
			       LIMIT $` + strconv.Itoa(len(args)) + `) latest
			 JOIN change_log c ON c.seq = latest.latest_seq
			 ORDER BY c.seq ASC`
	return s.queryChangeSet(ctx, query, args, limit, sinceCursor)
}

// queryChangeSet runs a change log query that asks for one row past limit, and
// pages its result: HasMore when that row came back, and the cursor at the last
// entry returned.
func (s *PostgresStore) queryChangeSet(ctx context.Context, query string, args []any, limit int, sinceCursor int64) (*platstore.ChangeSet, error) {
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
func (s *PostgresStore) LatestCursor(ctx context.Context, projectID, stream string) (int64, error) {
	stream = storeutil.DefaultStream(stream)
	var cursor int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM change_log WHERE project_id = $1 AND stream = $2`,
		projectID, stream).Scan(&cursor)
	if err != nil {
		return 0, fmt.Errorf("query latest cursor: %w", err)
	}
	return cursor, nil
}

// CompactChangeLog trims the live change_log, keeping only the latest entry per
// (project_id, block_id, locale) older than retainDays. Trimmed rows are moved
// to change_log_archive rather than deleted, so the historical sync trail is
// preserved.
func (s *PostgresStore) CompactChangeLog(ctx context.Context, projectID, stream string, retainDays int) (int64, error) {
	stream = storeutil.DefaultStream(stream)
	cutoff := time.Now().AddDate(0, 0, -retainDays)

	const where = `project_id = $1 AND stream = $2 AND logged_at <= $3
		  AND seq NOT IN (
			SELECT MAX(seq) FROM change_log
			WHERE project_id = $1 AND stream = $2
			GROUP BY block_id, COALESCE(locale, '')
		  )`

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin compact tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO change_log_archive
			(seq, project_id, block_id, change_type, locale, content_hash, stream, correlation_id, logged_at)
		 SELECT seq, project_id, block_id, change_type, locale, content_hash, stream, correlation_id, logged_at
		 FROM change_log WHERE `+where+`
		 ON CONFLICT (seq) DO NOTHING`,
		projectID, stream, cutoff); err != nil {
		return 0, fmt.Errorf("archive change log: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM change_log WHERE `+where, projectID, stream, cutoff)
	if err != nil {
		return 0, fmt.Errorf("compact change log: %w", err)
	}
	deleted, _ := result.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit compact: %w", err)
	}
	return deleted, nil
}
