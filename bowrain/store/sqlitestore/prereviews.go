package sqlitestore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// RecordPreReview stores an agent's advice on one translation. See
// store.ContentStore.
func (s *SQLiteStore) RecordPreReview(ctx context.Context, projectID, stream string, r platstore.PreReview) error {
	stream = storeutil.DefaultStream(stream)
	reasons := r.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	raw, err := json.Marshal(reasons)
	if err != nil {
		return fmt.Errorf("encode the reasons of a pre-review: %w", err)
	}
	at := r.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO pre_reviews (project_id, stream, block_id, locale, score, reviewer, reasons, revision, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, stream, block_id, locale) DO UPDATE SET
			score = excluded.score, reviewer = excluded.reviewer, reasons = excluded.reasons,
			revision = excluded.revision, created_at = excluded.created_at`,
		projectID, stream, r.BlockID, r.Locale, r.Score, r.Reviewer, string(raw), r.Revision,
		at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record the pre-review of block %s in %s: %w", r.BlockID, r.Locale, err)
	}
	return nil
}

// PreReviews reads the advice recorded on blocks. See store.ContentStore.
func (s *SQLiteStore) PreReviews(ctx context.Context, projectID, stream string, blockIDs []string) ([]platstore.PreReview, error) {
	if len(blockIDs) == 0 {
		return nil, nil
	}
	stream = storeutil.DefaultStream(stream)
	args := make([]any, 0, len(blockIDs)+2)
	args = append(args, projectID, stream)
	for _, id := range blockIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT block_id, locale, score, reviewer, reasons, revision, created_at
		 FROM pre_reviews WHERE project_id = ? AND stream = ? AND block_id IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(blockIDs)), ",")+`)
		 ORDER BY block_id, locale`, args...)
	if err != nil {
		return nil, fmt.Errorf("read pre-reviews: %w", err)
	}
	defer rows.Close()
	var out []platstore.PreReview
	for rows.Next() {
		var (
			r            platstore.PreReview
			reasons, raw string
		)
		if err := rows.Scan(&r.BlockID, &r.Locale, &r.Score, &r.Reviewer, &reasons, &r.Revision, &raw); err != nil {
			return nil, fmt.Errorf("read a pre-review: %w", err)
		}
		if err := json.Unmarshal([]byte(reasons), &r.Reasons); err != nil {
			return nil, fmt.Errorf("decode the reasons of the pre-review of block %s: %w", r.BlockID, err)
		}
		if r.At, err = storeutil.ParseStoredTime("created_at", raw); err != nil {
			return nil, fmt.Errorf("pre-review of block %s: %w", r.BlockID, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
