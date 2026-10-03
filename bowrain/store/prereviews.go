package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// RecordPreReview stores an agent's advice on one translation. See
// store.ContentStore.
func (s *PostgresStore) RecordPreReview(ctx context.Context, projectID, stream string, r platstore.PreReview) error {
	stream = storeutil.DefaultStream(stream)
	reasons, err := json.Marshal(nonNilReasons(r.Reasons))
	if err != nil {
		return fmt.Errorf("encode the reasons of a pre-review: %w", err)
	}
	at := r.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO pre_reviews (project_id, stream, block_id, locale, score, reviewer, reasons, revision, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE SET
			score = EXCLUDED.score, reviewer = EXCLUDED.reviewer, reasons = EXCLUDED.reasons,
			revision = EXCLUDED.revision, created_at = EXCLUDED.created_at`,
		projectID, stream, r.BlockID, r.Locale, r.Score, r.Reviewer, string(reasons), r.Revision, at); err != nil {
		return fmt.Errorf("record the pre-review of block %s in %s: %w", r.BlockID, r.Locale, err)
	}
	return nil
}

// PreReviews reads the advice recorded on blocks. See store.ContentStore.
func (s *PostgresStore) PreReviews(ctx context.Context, projectID, stream string, blockIDs []string) ([]platstore.PreReview, error) {
	if len(blockIDs) == 0 {
		return nil, nil
	}
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT block_id, locale, score, reviewer, reasons, revision, created_at
		 FROM pre_reviews WHERE project_id = $1 AND stream = $2 AND block_id = ANY($3)
		 ORDER BY block_id, locale`,
		projectID, stream, blockIDs)
	if err != nil {
		return nil, fmt.Errorf("read pre-reviews: %w", err)
	}
	defer rows.Close()
	var out []platstore.PreReview
	for rows.Next() {
		var (
			r       platstore.PreReview
			reasons string
		)
		if err := rows.Scan(&r.BlockID, &r.Locale, &r.Score, &r.Reviewer, &reasons, &r.Revision, &r.At); err != nil {
			return nil, fmt.Errorf("read a pre-review: %w", err)
		}
		if err := json.Unmarshal([]byte(reasons), &r.Reasons); err != nil {
			return nil, fmt.Errorf("decode the reasons of the pre-review of block %s: %w", r.BlockID, err)
		}
		r.At = r.At.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// nonNilReasons stores advice with no reasons as an empty list.
func nonNilReasons(reasons []string) []string {
	if reasons == nil {
		return []string{}
	}
	return reasons
}
