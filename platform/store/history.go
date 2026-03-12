package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/model"
	platstore "github.com/neokapi/neokapi/platform/store"
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
		if err := rows.Scan(&e.Seq, &e.ChangeType, &e.Text, &e.CodedText, &e.Origin, &e.Author, &createdStr); err != nil {
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
func recordBlockHistory(ctx context.Context, tx *sql.Tx, projectID, stream, blockID, locale, changeType, text, codedText, origin, author string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO block_history (project_id, stream, block_id, locale, change_type, text, coded_text, origin, author, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, stream, blockID, locale, changeType, text, codedText, origin, author, now)
	return err
}

// recordTargetHistory checks for target changes and records history entries.
func recordTargetHistory(ctx context.Context, tx *sql.Tx, projectID, stream string, blockID string, oldTargets map[model.LocaleID][]*model.Segment, newTargets map[model.LocaleID][]*model.Segment) error {
	for locale, newSegs := range newTargets {
		newText := segmentsText(newSegs)
		newCoded := segmentsCodedText(newSegs)

		oldSegs := oldTargets[locale]
		oldText := segmentsText(oldSegs)

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

// segmentsText extracts plain text from segments.
func segmentsText(segs []*model.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	if segs[0].Content != nil {
		return segs[0].Content.Text()
	}
	return ""
}

// segmentsCodedText extracts coded text from segments.
func segmentsCodedText(segs []*model.Segment) string {
	if len(segs) == 0 {
		return ""
	}
	if segs[0].Content != nil {
		return segs[0].Content.CodedText
	}
	return ""
}

// loadExistingTargets fetches existing targets JSON for a block within a transaction.
func loadExistingTargets(ctx context.Context, tx *sql.Tx, projectID, _, blockID string) (map[model.LocaleID][]*model.Segment, error) {
	var targetsJSON string
	err := tx.QueryRowContext(ctx,
		`SELECT targets_json FROM blocks WHERE project_id = ? AND id = ?`,
		projectID, blockID).Scan(&targetsJSON)
	if err == sql.ErrNoRows {
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
