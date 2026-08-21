package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/neokapi/neokapi/core/id"
)

// A coordinate is minted by the recipe and by nothing else. An approved axis is
// therefore not real until kapi.yaml says so and a push carries it, and this is
// the proposal in between: one row per recipe field an approval wants set,
// waiting for the next pull to write it into the working tree where git reviews
// it.
//
// It is deliberately NOT a registry of axes. A row exists to become a recipe
// line and is spent when applied; nothing reads it to decide what a project's
// context space IS. That question is answered from the collections a push
// reconciled — the recipe's own account of itself.

// PendingRecipeChangeStatus tracks whether the working tree has taken the change.
type PendingRecipeChangeStatus string

const (
	// RecipeChangePending is waiting for a pull to apply it.
	RecipeChangePending PendingRecipeChangeStatus = "pending"
	// RecipeChangeApplied has been written into a working tree.
	RecipeChangeApplied PendingRecipeChangeStatus = "applied"
)

// PendingRecipeChange is one recipe field an approval wants set, carried in the
// shape a kapi apply recipe entry already uses: a dotted path and a JSON value.
// The client hands these to the same setRecipeField the local fix loop uses, so
// there is one allowlist and one writer rather than two that can disagree.
type PendingRecipeChange struct {
	ID          string                    `json:"id"`
	WorkspaceID string                    `json:"workspace_id"`
	ProjectID   string                    `json:"project_id"`
	Path        string                    `json:"path"`
	Value       json.RawMessage           `json:"value"`
	Status      PendingRecipeChangeStatus `json:"status"`
	CreatedBy   string                    `json:"created_by,omitempty"`
	CreatedAt   time.Time                 `json:"created_at"`
	AppliedAt   *time.Time                `json:"applied_at,omitempty"`
}

// ProposeRecipeChange records one pending recipe field for a project.
//
// Idempotent by path: approving the same axis twice restates the same recipe
// line rather than queueing it again, and re-approving it at a different value
// replaces the pending one — the last decision is the one waiting to be
// applied, not a queue of them.
func (s *PostgresStore) ProposeRecipeChange(ctx context.Context, ch *PendingRecipeChange) error {
	if ch.ID == "" {
		ch.ID = id.New()
	}
	if ch.Status == "" {
		ch.Status = RecipeChangePending
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pending_recipe_changes (id, workspace_id, project_id, path, value, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (project_id, path) WHERE status = 'pending'
		DO UPDATE SET value = EXCLUDED.value, created_by = EXCLUDED.created_by, created_at = NOW()`,
		ch.ID, ch.WorkspaceID, ch.ProjectID, ch.Path, string(ch.Value), ch.Status, ch.CreatedBy)
	return err
}

// PendingRecipeChanges lists what a project's working tree has not yet taken,
// oldest first so a pull applies them in the order they were decided.
func (s *PostgresStore) PendingRecipeChanges(ctx context.Context, projectID string) ([]*PendingRecipeChange, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, project_id, path, value, status, created_by, created_at, applied_at
		FROM pending_recipe_changes
		WHERE project_id = $1 AND status = 'pending'
		ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*PendingRecipeChange
	for rows.Next() {
		ch := &PendingRecipeChange{}
		var value string
		var appliedAt sql.NullTime
		if err := rows.Scan(&ch.ID, &ch.WorkspaceID, &ch.ProjectID, &ch.Path, &value,
			&ch.Status, &ch.CreatedBy, &ch.CreatedAt, &appliedAt); err != nil {
			return nil, err
		}
		ch.Value = json.RawMessage(value)
		if appliedAt.Valid {
			ch.AppliedAt = &appliedAt.Time
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// MarkRecipeChangeApplied records that a working tree took the change, so the
// next pull does not offer it again. Applying twice is harmless — setRecipeField
// reports no change when the value already matches — but a row that never
// settles would keep proposing a decision already made.
func (s *PostgresStore) MarkRecipeChangeApplied(ctx context.Context, changeID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE pending_recipe_changes
		SET status = $1, applied_at = NOW()
		WHERE id = $2 AND status = 'pending'`, RecipeChangeApplied, changeID)
	return err
}
