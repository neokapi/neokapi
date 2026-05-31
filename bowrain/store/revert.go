package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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

// ComputePointInTimeReverts returns, for every (block, locale) that changed
// AFTER the cutoff, the value it held as of the cutoff — i.e. what restoring the
// stream to that point should set. Targets created after the cutoff get
// Clear=true. Targets unchanged since the cutoff are not returned (already
// correct). Read-only; callers apply via StoreBlocks so the restore is recorded.
func (s *PostgresStore) ComputePointInTimeReverts(ctx context.Context, projectID, stream string, cutoff time.Time) ([]TargetRevert, error) {
	stream = defaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT b.block_id, b.locale,
		   COALESCE((SELECT h.text FROM block_history h
		             WHERE h.project_id=$1 AND h.stream=$2 AND h.block_id=b.block_id AND h.locale=b.locale AND h.created_at <= $3
		             ORDER BY h.id DESC LIMIT 1), '') AS asof_text,
		   COALESCE((SELECT h.coded_text FROM block_history h
		             WHERE h.project_id=$1 AND h.stream=$2 AND h.block_id=b.block_id AND h.locale=b.locale AND h.created_at <= $3
		             ORDER BY h.id DESC LIMIT 1), '') AS asof_coded,
		   NOT EXISTS(SELECT 1 FROM block_history h
		             WHERE h.project_id=$1 AND h.stream=$2 AND h.block_id=b.block_id AND h.locale=b.locale AND h.created_at <= $3) AS clear
		 FROM (
		   SELECT DISTINCT block_id, locale FROM block_history
		   WHERE project_id=$1 AND stream=$2 AND created_at > $3
		 ) b`,
		projectID, stream, cutoff)
	if err != nil {
		return nil, fmt.Errorf("compute point-in-time reverts: %w", err)
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

// CursorTime returns the timestamp of the latest change at or before a change_log
// cursor — the moment the stream was in the state identified by that cursor.
func (s *PostgresStore) CursorTime(ctx context.Context, projectID, stream string, cursor int64) (time.Time, error) {
	stream = defaultStream(stream)
	var t time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT logged_at FROM change_log WHERE project_id=$1 AND stream=$2 AND seq <= $3 ORDER BY seq DESC LIMIT 1`,
		projectID, stream, cursor).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("cursor %d has no corresponding change", cursor)
	}
	return t, err
}

// VersionTime returns the creation time of a named version.
func (s *PostgresStore) VersionTime(ctx context.Context, projectID, versionID string) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT created_at FROM versions WHERE id=$1 AND project_id=$2`, versionID, projectID).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, fmt.Errorf("version %s not found", versionID)
	}
	return t, err
}
