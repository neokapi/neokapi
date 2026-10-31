package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// AssignContextSuggestion implements platstore.ContextAssignmentStore.
func (s *PostgresStore) AssignContextSuggestion(ctx context.Context, a platstore.ContextAssignment) error {
	if a.ProjectID == "" || a.SuggestionID == "" {
		return errors.New("assign a suggestion: no project or no suggestion")
	}
	if a.Assignee == "" {
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM context_assignments WHERE project_id = $1 AND suggestion_id = $2`,
			a.ProjectID, a.SuggestionID); err != nil {
			return fmt.Errorf("unassign suggestion %s: %w", a.SuggestionID, err)
		}
		return nil
	}
	at := a.AssignedAt
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO context_assignments (project_id, suggestion_id, assignee, assigned_by, assigned_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, suggestion_id) DO UPDATE
		   SET assignee = EXCLUDED.assignee, assigned_by = EXCLUDED.assigned_by, assigned_at = EXCLUDED.assigned_at`,
		a.ProjectID, a.SuggestionID, a.Assignee, a.AssignedBy, at.UTC()); err != nil {
		return fmt.Errorf("assign suggestion %s: %w", a.SuggestionID, err)
	}
	return nil
}

// ListContextAssignments implements platstore.ContextAssignmentStore.
func (s *PostgresStore) ListContextAssignments(ctx context.Context, projectID string) ([]platstore.ContextAssignment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT suggestion_id, assignee, assigned_by, assigned_at
		  FROM context_assignments WHERE project_id = $1 ORDER BY suggestion_id`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list the assignments of project %s: %w", projectID, err)
	}
	defer rows.Close()
	out := []platstore.ContextAssignment{}
	for rows.Next() {
		a := platstore.ContextAssignment{ProjectID: projectID}
		if err := rows.Scan(&a.SuggestionID, &a.Assignee, &a.AssignedBy, &a.AssignedAt); err != nil {
			return nil, fmt.Errorf("list the assignments of project %s: %w", projectID, err)
		}
		a.AssignedAt = a.AssignedAt.UTC()
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list the assignments of project %s: %w", projectID, err)
	}
	return out, nil
}

var _ platstore.ContextAssignmentStore = (*PostgresStore)(nil)
