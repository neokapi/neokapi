package store

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

var _ platstore.TargetAuthorStore = (*PostgresStore)(nil)

// targetContentChangeTypes are the block_history change types that record a
// translation being written. The same table holds the decision rows the ledger
// files (change_type 'decision', whose author is the decider) and the rows a
// settled projection writes (author 'system'), and neither of those names who
// wrote the wording now under review.
const targetContentChangeTypes = `('target_added', 'target_modified')`

// LastTargetAuthors returns the last attributed author of each (block, locale)
// target. DISTINCT ON keeps one row per pair, the newest by id, and the whole
// batch travels in one query so a bulk approval costs one round trip rather
// than one per block.
func (s *PostgresStore) LastTargetAuthors(ctx context.Context, projectID, stream string, blockIDs, locales []string) (map[platstore.TargetRef]string, error) {
	out := map[platstore.TargetRef]string{}
	if len(blockIDs) == 0 || len(locales) == 0 {
		return out, nil
	}
	args := []any{projectID, storeutil.DefaultStream(stream)}
	args = append(args, anyStrings(blockIDs)...)
	args = append(args, anyStrings(locales)...)

	query := `SELECT DISTINCT ON (block_id, locale) block_id, locale, author
		FROM block_history
		WHERE project_id = $1 AND stream = $2
		  AND change_type IN ` + targetContentChangeTypes + `
		  AND author <> ''
		  AND block_id IN (` + placeholderList("pg", 3, len(blockIDs)) + `)
		  AND locale IN (` + placeholderList("pg", 3+len(blockIDs), len(locales)) + `)
		ORDER BY block_id, locale, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query last target authors: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref platstore.TargetRef
		var author string
		if err := rows.Scan(&ref.BlockID, &ref.Locale, &author); err != nil {
			return nil, fmt.Errorf("scan last target author: %w", err)
		}
		out[ref] = author
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate last target authors: %w", err)
	}
	return out, nil
}
