package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// PutItemCheckTally implements platstore.ItemCheckTallyStore.
func (s *PostgresStore) PutItemCheckTally(ctx context.Context, t platstore.ItemCheckTally) error {
	if t.ProjectID == "" || t.ItemID == "" {
		return errors.New("record a check tally: no project or no item")
	}
	stream := t.Stream
	if stream == "" {
		stream = "main"
	}
	at := t.CheckedAt
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO item_check_tallies (project_id, stream, item_id, findings, fixable, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, stream, item_id) DO UPDATE
		   SET findings = EXCLUDED.findings, fixable = EXCLUDED.fixable, checked_at = EXCLUDED.checked_at`,
		t.ProjectID, stream, t.ItemID, t.Findings, t.Fixable, at.UTC()); err != nil {
		return fmt.Errorf("record the check tally of item %s: %w", t.ItemID, err)
	}
	return nil
}

// ListItemCheckTallies implements platstore.ItemCheckTallyStore.
func (s *PostgresStore) ListItemCheckTallies(ctx context.Context, projectID, stream string) ([]platstore.ItemCheckTally, error) {
	if stream == "" {
		stream = "main"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT item_id, findings, fixable, checked_at
		  FROM item_check_tallies WHERE project_id = $1 AND stream = $2 ORDER BY item_id`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list the check tallies of project %s: %w", projectID, err)
	}
	defer rows.Close()
	out := []platstore.ItemCheckTally{}
	for rows.Next() {
		t := platstore.ItemCheckTally{ProjectID: projectID, Stream: stream}
		if err := rows.Scan(&t.ItemID, &t.Findings, &t.Fixable, &t.CheckedAt); err != nil {
			return nil, fmt.Errorf("list the check tallies of project %s: %w", projectID, err)
		}
		t.CheckedAt = t.CheckedAt.UTC()
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list the check tallies of project %s: %w", projectID, err)
	}
	return out, nil
}

// DeleteItemCheckTally implements platstore.ItemCheckTallyStore.
func (s *PostgresStore) DeleteItemCheckTally(ctx context.Context, projectID, stream, itemID string) error {
	if stream == "" {
		stream = "main"
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM item_check_tallies WHERE project_id = $1 AND stream = $2 AND item_id = $3`,
		projectID, stream, itemID); err != nil {
		return fmt.Errorf("delete the check tally of item %s: %w", itemID, err)
	}
	return nil
}

var _ platstore.ItemCheckTallyStore = (*PostgresStore)(nil)
