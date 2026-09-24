package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
)

// loadOldTargetText batch-loads the current target text for a set of blocks,
// keyed by block ID then variant text. Used before an upsert to detect target
// changes for block_history.
func loadOldTargetText(ctx context.Context, tx Runner, projectID, stream string, blockIDs []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if len(blockIDs) == 0 {
		return out, nil
	}
	rows, err := tx.QueryContext(ctx, sqlListTranslationTextByBlocks("pg", len(blockIDs)),
		append([]any{projectID, stream}, anyStrings(blockIDs)...)...)
	if err != nil {
		return nil, fmt.Errorf("load old target text: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var bid, variant, text string
		if err := rows.Scan(&bid, &variant, &text); err != nil {
			return nil, fmt.Errorf("scan old target text: %w", err)
		}
		if out[bid] == nil {
			out[bid] = map[string]string{}
		}
		out[bid][variant] = text
	}
	return out, rows.Err()
}

// originText renders a target Origin to the short label stored in
// block_history.origin (e.g. "human", "mt", "ai", "memory").
func originText(o model.Origin) string {
	return o.Kind
}

func sqlListTranslationTextByBlocks(dialect string, nblocks int) string {
	return `SELECT block_id, locale, text FROM translations
		WHERE project_id = ` + placeholder(dialect, 1) + ` AND stream = ` + placeholder(dialect, 2) + `
		AND block_id IN (` + placeholderList(dialect, 3, nblocks) + `)`
}

// recordTargetHistoryPg appends history for changed targets within the
// store-blocks transaction. Prior content supports per-edit rollback. author
// remains empty here; audit_log records the user through block.updated.
//
// Use the transaction's process timestamp, now, rather than database NOW().
// ComputePointInTimeReverts compares history with versions.created_at and
// change_log.logged_at, which use the process clock. Mixing clocks could exclude
// required reverts or revert content older than the selected version.
func recordTargetHistoryPg(ctx context.Context, tx Runner, projectID, stream, blockID string, oldText map[string]string, newTargets map[model.VariantKey]*model.Target, now time.Time) error {
	cc := ChangeContextFromContext(ctx)
	for key, nt := range newTargets {
		if nt == nil {
			continue
		}
		variant := VariantKeyText(key)
		newText := model.RunsText(nt.Runs)
		prev := oldText[variant]
		if newText == prev {
			continue
		}
		changeType := "target_modified"
		if prev == "" {
			changeType = "target_added"
		}
		coded := ""
		if len(nt.Runs) > 0 {
			if b, err := json.Marshal(nt.Runs); err == nil {
				coded = string(b)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, stream, block_id, locale, change_type, text, coded_text, origin, author, actor_role, edit_reason, correlation_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			projectID, stream, blockID, variant, changeType, newText, coded, originText(nt.Origin),
			cc.Actor, cc.ActorRole, cc.Reason, cc.CorrelationID, now); err != nil {
			return fmt.Errorf("record block history for %s/%s: %w", blockID, variant, err)
		}
	}
	return nil
}

// GetBlockHistory returns history entries for a block in a specific locale.
func (s *PostgresStore) GetBlockHistory(ctx context.Context, projectID, stream, blockID string, locale string, limit int) ([]platstore.BlockHistoryEntry, error) {
	stream = storeutil.DefaultStream(stream)
	if limit <= 0 || limit > MaxHistoryEntries {
		limit = MaxHistoryEntries
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, change_type, text, coded_text, origin, author, actor_role, edit_reason, correlation_id, created_at
		 FROM block_history
		 WHERE project_id = $1 AND stream = $2 AND block_id = $3 AND locale = $4
		 ORDER BY id DESC
		 LIMIT $5`,
		projectID, stream, blockID, locale, limit)
	if err != nil {
		return nil, fmt.Errorf("query block history: %w", err)
	}
	defer rows.Close()

	var entries []platstore.BlockHistoryEntry
	for rows.Next() {
		var e platstore.BlockHistoryEntry
		if err := rows.Scan(&e.Seq, &e.ChangeType, &e.Text, &e.Coded, &e.Origin, &e.Author,
			&e.ActorRole, &e.EditReason, &e.CorrelationID, &e.Timestamp); err != nil {
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
