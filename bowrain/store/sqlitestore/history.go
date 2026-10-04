package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
)

// MaxHistoryEntries is the default maximum number of history entries returned.
const MaxHistoryEntries = 100

// GetBlockHistory returns history entries for a block in a specific locale.
func (s *SQLiteStore) GetBlockHistory(ctx context.Context, projectID, stream, blockID string, locale string, limit int) ([]platstore.BlockHistoryEntry, error) {
	stream = storeutil.DefaultStream(stream)
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

// carriesTranslation reports whether b holds an edition besides the one it was
// read in.
func carriesTranslation(b *model.Block) bool {
	src := b.EditionKeyOf(model.EditionKey{})
	for key := range b.EachEdition {
		if key != src {
			return true
		}
	}
	return false
}

// recordTargetHistory checks each translation b carries (every edition other
// than the one it was read in) against the stored editions in oldTargets and
// records history entries for the ones that changed.
func recordTargetHistory(ctx context.Context, tx *sql.Tx, projectID, stream string, blockID string, oldTargets map[model.EditionKey]model.Edition, b *model.Block) error {
	src := b.EditionKeyOf(model.EditionKey{})
	for key, newTarget := range b.EachEdition {
		if key == src {
			continue
		}
		newText := model.RunsText(newTarget.Runs)
		newCoded := targetRunsJSON(newTarget.Runs)

		oldText := ""
		if old, ok := oldTargets[key]; ok {
			oldText = model.RunsText(old.Runs)
		}

		if newText == oldText {
			continue
		}

		changeType := "target_modified"
		if oldText == "" {
			changeType = "target_added"
		}

		variant := bstore.VariantKeyText(key)
		if err := recordBlockHistory(ctx, tx, projectID, stream, blockID, variant, changeType, newText, newCoded, "", ""); err != nil {
			return fmt.Errorf("record history for block %s variant %s: %w", blockID, variant, err)
		}
	}
	return nil
}

// targetRunsJSON returns the JSON-encoded Run sequence of a target, stored in
// the block_history.coded_text column (the column name is retained for schema
// stability). The column is a debugging aid for editor history that preserves
// inline markup, not the canonical store (the translations table is).
func targetRunsJSON(runs []model.Run) string {
	if len(runs) == 0 {
		return ""
	}
	b, err := json.Marshal(runs)
	if err != nil {
		return ""
	}
	return string(b)
}

// loadExistingTargets returns the stored translations of a block, each under
// the key its row is filed under, for recordTargetHistory before a StoreBlocks
// upsert. Reads from the translations table, whose target_json column holds
// the edition (bstore.UnmarshalTargetJSON).
func loadExistingTargets(ctx context.Context, tx *sql.Tx, projectID, _, blockID string) (map[model.EditionKey]model.Edition, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT locale, target_json FROM translations
		 WHERE project_id = ? AND stream = 'main' AND block_id = ?`,
		projectID, blockID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := map[model.EditionKey]model.Edition{}
	for rows.Next() {
		var keyText, targetJSON string
		if err := rows.Scan(&keyText, &targetJSON); err != nil {
			return nil, err
		}
		var key model.EditionKey
		if err := key.UnmarshalText([]byte(keyText)); err != nil {
			continue // skip malformed keys silently
		}
		var e model.Edition
		if targetJSON != "" && targetJSON != "null" {
			decoded, err := bstore.UnmarshalTargetJSON([]byte(targetJSON))
			if err != nil {
				continue // a malformed row is skipped
			}
			e = decoded
		}
		targets[key] = e
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets, nil
}
