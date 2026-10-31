package sqlitestore

import (
	"context"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// PutItemCheckTally implements platstore.ItemCheckTallyStore.
func (s *SQLiteStore) PutItemCheckTally(ctx context.Context, t platstore.ItemCheckTally) error {
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
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (project_id, stream, item_id) DO UPDATE
		   SET findings = excluded.findings, fixable = excluded.fixable, checked_at = excluded.checked_at`,
		t.ProjectID, stream, t.ItemID, t.Findings, t.Fixable, at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record the check tally of item %s: %w", t.ItemID, err)
	}
	return nil
}

// ListItemCheckTallies implements platstore.ItemCheckTallyStore.
func (s *SQLiteStore) ListItemCheckTallies(ctx context.Context, projectID, stream string) ([]platstore.ItemCheckTally, error) {
	if stream == "" {
		stream = "main"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT item_id, findings, fixable, checked_at
		  FROM item_check_tallies WHERE project_id = ? AND stream = ? ORDER BY item_id`, projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list the check tallies of project %s: %w", projectID, err)
	}
	defer rows.Close()
	out := []platstore.ItemCheckTally{}
	for rows.Next() {
		t := platstore.ItemCheckTally{ProjectID: projectID, Stream: stream}
		var at string
		if err := rows.Scan(&t.ItemID, &t.Findings, &t.Fixable, &at); err != nil {
			return nil, fmt.Errorf("list the check tallies of project %s: %w", projectID, err)
		}
		if t.CheckedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("list the check tallies of project %s: the time %q: %w", projectID, at, err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list the check tallies of project %s: %w", projectID, err)
	}
	return out, nil
}

// DeleteItemCheckTally implements platstore.ItemCheckTallyStore.
func (s *SQLiteStore) DeleteItemCheckTally(ctx context.Context, projectID, stream, itemID string) error {
	if stream == "" {
		stream = "main"
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM item_check_tallies WHERE project_id = ? AND stream = ? AND item_id = ?`,
		projectID, stream, itemID); err != nil {
		return fmt.Errorf("delete the check tally of item %s: %w", itemID, err)
	}
	return nil
}

var _ platstore.ItemCheckTallyStore = (*SQLiteStore)(nil)
