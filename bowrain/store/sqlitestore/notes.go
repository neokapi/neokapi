package sqlitestore

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// AddBlockNote inserts a new block note.
func (s *SQLiteStore) AddBlockNote(ctx context.Context, projectID, stream, blockID string, note model.BlockNote) error {
	stream = storeutil.DefaultStream(stream)
	if note.ID == "" {
		note.ID = id.New()
	}
	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO block_notes (id, project_id, stream, block_id, author, text, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		note.ID, projectID, stream, blockID, note.Author, note.Text, note.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert block note: %w", err)
	}
	return nil
}

// ListBlockNotes returns all notes for a block, ordered by creation time.
func (s *SQLiteStore) ListBlockNotes(ctx context.Context, projectID, stream, blockID string) ([]model.BlockNote, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, block_id, author, text, created_at
		 FROM block_notes
		 WHERE project_id = ? AND stream = ? AND block_id = ?
		 ORDER BY created_at ASC`,
		projectID, stream, blockID)
	if err != nil {
		return nil, fmt.Errorf("query block notes: %w", err)
	}
	defer rows.Close()

	var notes []model.BlockNote
	for rows.Next() {
		var n model.BlockNote
		var createdStr string
		if err := rows.Scan(&n.ID, &n.BlockID, &n.Author, &n.Text, &createdStr); err != nil {
			return nil, fmt.Errorf("scan block note: %w", err)
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		if n.CreatedAt.IsZero() {
			n.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdStr)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate block notes: %w", err)
	}

	if notes == nil {
		notes = []model.BlockNote{}
	}
	return notes, nil
}

// DeleteBlockNote removes a block note by ID.
func (s *SQLiteStore) DeleteBlockNote(ctx context.Context, projectID, stream, noteID string) error {
	stream = storeutil.DefaultStream(stream)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM block_notes WHERE project_id = ? AND stream = ? AND id = ?`,
		projectID, stream, noteID)
	if err != nil {
		return fmt.Errorf("delete block note: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("note %s not found in project %s", noteID, projectID)
	}
	return nil
}
