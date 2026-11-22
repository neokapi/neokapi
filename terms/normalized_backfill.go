package terms

import (
	"context"
	"database/sql"
	"fmt"
)

// normalizedBackfillBatch is how many term rows one backfill statement reads.
const normalizedBackfillBatch = 500

// BackfillNormalizedText fills tb_terms.text_normalized for the rows that
// hold the empty string, which is every row written before the column
// existed. It reads the rows in batches ordered by id, so memory stays bounded
// and every row is visited once. A row whose text normalizes to the empty
// string keeps it. Running it again over a filled table changes nothing.
//
// postgres selects the placeholder dialect and the workspace column, which
// only the PostgreSQL table has.
func BackfillNormalizedText(ctx context.Context, tx *sql.Tx, postgres bool) error {
	selectSQL := `SELECT id, '', text FROM tb_terms WHERE text_normalized = '' AND id > ? ORDER BY id LIMIT ?`
	updateSQL := `UPDATE tb_terms SET text_normalized = ? WHERE id = ?`
	if postgres {
		selectSQL = `SELECT id, workspace_id, text FROM tb_terms WHERE text_normalized = '' AND id > $1 ORDER BY id LIMIT $2`
		updateSQL = `UPDATE tb_terms SET text_normalized = $1 WHERE id = $2 AND workspace_id = $3`
	}

	type pending struct {
		id         int64
		workspace  string
		normalized string
	}
	var after int64
	for {
		rows, err := tx.QueryContext(ctx, selectSQL, after, normalizedBackfillBatch)
		if err != nil {
			return fmt.Errorf("read terms to normalize: %w", err)
		}
		var batch []pending
		n := 0
		for rows.Next() {
			var p pending
			var text string
			if err := rows.Scan(&p.id, &p.workspace, &text); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan term to normalize: %w", err)
			}
			n++
			after = p.id
			if p.normalized = NormalizeTerm(text); p.normalized != "" {
				batch = append(batch, p)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read terms to normalize: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("read terms to normalize: %w", err)
		}
		for _, p := range batch {
			args := []any{p.normalized, p.id}
			if postgres {
				args = append(args, p.workspace)
			}
			if _, err := tx.ExecContext(ctx, updateSQL, args...); err != nil {
				return fmt.Errorf("normalize term %d: %w", p.id, err)
			}
		}
		if n < normalizedBackfillBatch {
			return nil
		}
	}
}
