package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// MaxHistoryEntries is the default maximum number of history entries returned.
const MaxHistoryEntries = 100

// GetBlockHistory returns history entries for a block in a specific locale.
func (s *SQLiteStore) GetBlockHistory(ctx context.Context, projectID, stream, blockID string, locale string, limit int) ([]platstore.BlockHistoryEntry, error) {
	stream = defaultStream(stream)
	if limit <= 0 || limit > MaxHistoryEntries {
		limit = MaxHistoryEntries
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, change_type, text, coded_text, origin, author, created_at
		 FROM block_history
		 WHERE project_id = ? AND stream = ? AND block_id = ? AND locale = ?
		 ORDER BY id DESC
		 LIMIT ?`,
		projectID, stream, blockID, locale, limit)
	if err != nil {
		return nil, fmt.Errorf("query block history: %w", err)
	}
	defer rows.Close()

	var entries []platstore.BlockHistoryEntry
	for rows.Next() {
		var e platstore.BlockHistoryEntry
		var createdStr string
		if err := rows.Scan(&e.Seq, &e.ChangeType, &e.Text, &e.Coded, &e.Origin, &e.Author, &createdStr); err != nil {
			return nil, fmt.Errorf("scan block history entry: %w", err)
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, createdStr)
		if e.Timestamp.IsZero() {
			// SQLite CURRENT_TIMESTAMP uses "2006-01-02 15:04:05" format.
			e.Timestamp, _ = time.Parse("2006-01-02 15:04:05", createdStr)
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

// recordBlockHistory inserts a history entry within a transaction.
func recordBlockHistory(ctx context.Context, tx *sql.Tx, projectID, stream, blockID, locale, changeType, text, coded, origin, author string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO block_history (project_id, stream, block_id, locale, change_type, text, coded_text, origin, author, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, stream, blockID, locale, changeType, text, coded, origin, author, now)
	return err
}

// recordTargetHistory checks for target changes and records history entries.
func recordTargetHistory(ctx context.Context, tx *sql.Tx, projectID, stream string, blockID string, oldTargets map[model.LocaleID][]*model.Segment, newTargets map[model.LocaleID][]*model.Segment) error {
	for locale, newSegs := range newTargets {
		newText := segmentsPlainText(newSegs)
		newCoded := segmentsCoded(newSegs)

		oldSegs := oldTargets[locale]
		oldText := segmentsPlainText(oldSegs)

		if newText == oldText {
			continue
		}

		changeType := "target_modified"
		if oldText == "" {
			changeType = "target_added"
		}

		if err := recordBlockHistory(ctx, tx, projectID, stream, blockID, string(locale), changeType, newText, newCoded, "", ""); err != nil {
			return fmt.Errorf("record history for block %s locale %s: %w", blockID, locale, err)
		}
	}
	return nil
}

// segmentsPlainText concatenates the plain text of every segment in
// the slice. Mirrors the behaviour of model.Block.TargetText so that
// history-row deduplication compares the same string the rest of the
// app sees.
func segmentsPlainText(segs []*model.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, s := range segs {
		buf.WriteString(s.Text())
	}
	return buf.String()
}

// segmentsCoded returns the legacy PUA-marker coded form of the first
// segment, used for the block_history.coded_text column. Multi-segment
// blocks only persist the first segment's coded form here — the column
// is a debugging aid for editor history, not the canonical store.
func segmentsCoded(segs []*model.Segment) string {
	if len(segs) == 0 || len(segs[0].Runs) == 0 {
		return ""
	}
	coded, _ := model.MarshalRuns(segs[0].Runs)
	return coded
}

// loadExistingTargets fetches existing targets JSON for a block within a transaction.
func loadExistingTargets(ctx context.Context, tx *sql.Tx, projectID, _, blockID string) (map[model.LocaleID][]*model.Segment, error) {
	var targetsJSON string
	err := tx.QueryRowContext(ctx,
		`SELECT targets_json FROM blocks WHERE project_id = ? AND id = ?`,
		projectID, blockID).Scan(&targetsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var targets map[model.LocaleID][]*model.Segment
	if err := json.Unmarshal([]byte(targetsJSON), &targets); err != nil {
		return nil, nil
	}
	return targets, nil
}
