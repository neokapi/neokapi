package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
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
		newCoded := segmentsRunsJSON(newSegs)

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

// segmentsRunsJSON returns the JSON-encoded Run sequence of the first
// segment, stored in the block_history.coded_text column (the column name is
// retained for schema stability). Multi-segment blocks only persist the first
// segment's runs here — the column is a debugging aid for editor history that
// preserves inline markup, not the canonical store (the translations table is).
func segmentsRunsJSON(segs []*model.Segment) string {
	if len(segs) == 0 || len(segs[0].Runs) == 0 {
		return ""
	}
	b, err := json.Marshal(segs[0].Runs)
	if err != nil {
		return ""
	}
	return string(b)
}

// loadExistingTargets fetches existing targets JSON for a block within a transaction.
// loadExistingTargets returns the current per-locale target segments for a
// block — used by recordTargetHistory before a StoreBlocks upsert. Reads
// from the translations table (#405) instead of the former inline
// blocks.targets_json column.
func loadExistingTargets(ctx context.Context, tx *sql.Tx, projectID, _, blockID string) (map[model.LocaleID][]*model.Segment, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT locale, segments_json FROM translations
		 WHERE project_id = ? AND stream = 'main' AND block_id = ?`,
		projectID, blockID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := map[model.LocaleID][]*model.Segment{}
	for rows.Next() {
		var locale, segJSON string
		if err := rows.Scan(&locale, &segJSON); err != nil {
			return nil, err
		}
		var segs []*model.Segment
		if segJSON != "" && segJSON != "[]" && segJSON != "null" {
			if err := json.Unmarshal([]byte(segJSON), &segs); err != nil {
				continue // skip malformed rows silently — same behaviour as the prior impl
			}
		}
		targets[model.LocaleID(locale)] = segs
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets, nil
}
