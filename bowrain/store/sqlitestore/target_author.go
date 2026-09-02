package sqlitestore

import (
	"context"
	"fmt"
	"strings"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

var _ platstore.TargetAuthorStore = (*SQLiteStore)(nil)

// LastTargetAuthors returns the last attributed author of each (block, locale)
// target: the newest content-change row per pair, found in one query rather
// than one per block.
//
// This store writes no author on a target change (recordTargetHistory passes an
// empty one), so today it reports nobody and every target reads as
// machine-authored. The query still answers what the history holds, and it
// starts reporting the moment a write carries an acting user.
func (s *SQLiteStore) LastTargetAuthors(ctx context.Context, projectID, stream string, blockIDs, locales []string) (map[platstore.TargetRef]string, error) {
	out := map[platstore.TargetRef]string{}
	if len(blockIDs) == 0 || len(locales) == 0 {
		return out, nil
	}
	args := []any{projectID, storeutil.DefaultStream(stream)}
	for _, b := range blockIDs {
		args = append(args, b)
	}
	for _, l := range locales {
		args = append(args, l)
	}

	query := `SELECT block_id, locale, author FROM block_history WHERE id IN (
			SELECT MAX(id) FROM block_history
			WHERE project_id = ? AND stream = ?
			  AND change_type IN ('target_added', 'target_modified')
			  AND author <> ''
			  AND block_id IN (` + qmarks(len(blockIDs)) + `)
			  AND locale IN (` + qmarks(len(locales)) + `)
			GROUP BY block_id, locale)`

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

// qmarks renders n comma-separated bind placeholders.
func qmarks(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}
