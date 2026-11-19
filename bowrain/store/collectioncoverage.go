package store

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// CountCollectionCoverage implements platstore.CollectionCoverageStore with
// two grouped queries: the translatable blocks per collection, and the
// established ones per collection and locale. The items join is LEFT, so a
// block whose item has no row counts in the ungrouped bucket.
func (s *PostgresStore) CountCollectionCoverage(ctx context.Context, projectID, stream string, locales []string) ([]platstore.CollectionCoverage, error) {
	defer observe.StartSpan(ctx, "db.query", "store.CountCollectionCoverage")()
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.DB.QueryContext(ctx, `SELECT COALESCE(i.collection_id, ''), count(*)
		FROM blocks b LEFT JOIN items i
		ON i.project_id = b.project_id AND i.stream = $2 AND i.name = b.item_name
		WHERE b.project_id = $1 AND b.stream = $2 AND b.translatable
		GROUP BY 1 ORDER BY 1`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("count collection blocks: %w", err)
	}
	defer rows.Close()
	var out []platstore.CollectionCoverage
	index := map[string]int{}
	for rows.Next() {
		var c platstore.CollectionCoverage
		if err := rows.Scan(&c.CollectionID, &c.Blocks); err != nil {
			return nil, fmt.Errorf("scan collection blocks: %w", err)
		}
		c.Established = map[string]int{}
		index[c.CollectionID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 || len(locales) == 0 {
		return out, nil
	}

	q := fmt.Sprintf(`SELECT COALESCE(i.collection_id, ''), t.locale, count(*)
		FROM blocks b JOIN translations t
		ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = $2
		LEFT JOIN items i
		ON i.project_id = b.project_id AND i.stream = $2 AND i.name = b.item_name
		WHERE b.project_id = $1 AND b.stream = $2 AND b.translatable
		AND t.locale = ANY($3) AND (%s) = 'established'
		GROUP BY 1, 2`, pgStatusBucket)
	est, err := s.db.DB.QueryContext(ctx, q, projectID, stream, locales)
	if err != nil {
		return nil, fmt.Errorf("count established blocks: %w", err)
	}
	defer est.Close()
	for est.Next() {
		var coll, locale string
		var n int
		if err := est.Scan(&coll, &locale, &n); err != nil {
			return nil, fmt.Errorf("scan established blocks: %w", err)
		}
		if i, ok := index[coll]; ok {
			out[i].Established[locale] = n
		}
	}
	return out, est.Err()
}
