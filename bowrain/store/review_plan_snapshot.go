package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// The review plan's snapshots, one row per project and stream
// (review_plan_snapshots). Every time comparison reads the database's clock,
// so instances with skewed clocks agree on what is due.

// GetReviewPlanSnapshot implements platstore.ReviewPlanSnapshotStore.
func (s *PostgresStore) GetReviewPlanSnapshot(ctx context.Context, projectID, stream string) (*platstore.ReviewPlanSnapshot, error) {
	out := platstore.ReviewPlanSnapshot{ProjectID: projectID, Stream: storeutil.DefaultStream(stream)}
	var data []byte
	var at sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT data, computed_at, dirty_seq > computed_seq
		  FROM review_plan_snapshots WHERE project_id = $1 AND stream = $2`,
		out.ProjectID, out.Stream).Scan(&data, &at, &out.Changed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // no snapshot is not an error
	}
	if err != nil {
		return nil, fmt.Errorf("read the review plan snapshot of %s: %w", projectID, err)
	}
	out.Data = data
	if at.Valid {
		out.ComputedAt = at.Time
	}
	return &out, nil
}

// ClaimReviewPlanSnapshot implements platstore.ReviewPlanSnapshotStore.
func (s *PostgresStore) ClaimReviewPlanSnapshot(ctx context.Context, projectID, stream, workspaceID, workspaceSlug string, lease time.Duration) (platstore.ReviewPlanSnapshotClaim, bool, error) {
	claim := platstore.ReviewPlanSnapshotClaim{
		ProjectID: projectID, Stream: storeutil.DefaultStream(stream), WorkspaceID: workspaceID, Token: claimToken(),
	}
	// A new row starts changed, so a compute that fails leaves it due.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO review_plan_snapshots (project_id, stream, workspace_id, workspace_slug, dirty_seq, dirty_at, first_dirty_at)
		VALUES ($1, $2, $3, $4, 1, NOW(), NOW())
		ON CONFLICT (project_id, stream) DO UPDATE
		   SET workspace_id = EXCLUDED.workspace_id, workspace_slug = EXCLUDED.workspace_slug
		 WHERE review_plan_snapshots.workspace_id <> EXCLUDED.workspace_id
		    OR review_plan_snapshots.workspace_slug <> EXCLUDED.workspace_slug`,
		claim.ProjectID, claim.Stream, workspaceID, workspaceSlug); err != nil {
		return claim, false, fmt.Errorf("create the review plan snapshot of %s: %w", projectID, err)
	}
	err := s.db.QueryRowContext(ctx, `
		UPDATE review_plan_snapshots
		   SET lease_until = NOW() + make_interval(secs => $3), lease_token = $4, claimed_at = NOW()
		 WHERE project_id = $1 AND stream = $2 AND (lease_until IS NULL OR lease_until < NOW())
		RETURNING dirty_seq`,
		claim.ProjectID, claim.Stream, lease.Seconds(), claim.Token).Scan(&claim.Seq)
	if errors.Is(err, sql.ErrNoRows) {
		return claim, false, nil
	}
	if err != nil {
		return claim, false, fmt.Errorf("claim the review plan snapshot of %s: %w", projectID, err)
	}
	return claim, true, nil
}

// ClaimDueReviewPlanSnapshots implements platstore.ReviewPlanSnapshotStore.
// SKIP LOCKED keeps two instances claiming at once from taking the same row.
func (s *PostgresStore) ClaimDueReviewPlanSnapshots(ctx context.Context, quiet, maxWait, lease time.Duration, limit int) ([]platstore.ReviewPlanSnapshotClaim, error) {
	token := claimToken()
	rows, err := s.db.QueryContext(ctx, `
		UPDATE review_plan_snapshots r
		   SET lease_until = NOW() + make_interval(secs => $3), lease_token = $5, claimed_at = NOW()
		  FROM (SELECT project_id, stream FROM review_plan_snapshots
		         WHERE dirty_seq > computed_seq
		           AND (lease_until IS NULL OR lease_until < NOW())
		           AND (dirty_at <= NOW() - make_interval(secs => $1)
		                OR first_dirty_at <= NOW() - make_interval(secs => $2))
		         ORDER BY first_dirty_at
		         LIMIT $4
		         FOR UPDATE SKIP LOCKED) due
		 WHERE r.project_id = due.project_id AND r.stream = due.stream
		RETURNING r.project_id, r.stream, r.workspace_id, r.dirty_seq`,
		quiet.Seconds(), maxWait.Seconds(), lease.Seconds(), limit, token)
	if err != nil {
		return nil, fmt.Errorf("claim due review plan snapshots: %w", err)
	}
	defer rows.Close()
	var out []platstore.ReviewPlanSnapshotClaim
	for rows.Next() {
		c := platstore.ReviewPlanSnapshotClaim{Token: token}
		if err := rows.Scan(&c.ProjectID, &c.Stream, &c.WorkspaceID, &c.Seq); err != nil {
			return nil, fmt.Errorf("claim due review plan snapshots: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim due review plan snapshots: %w", err)
	}
	return out, nil
}

// CompleteReviewPlanSnapshot implements platstore.ReviewPlanSnapshotStore.
func (s *PostgresStore) CompleteReviewPlanSnapshot(ctx context.Context, claim platstore.ReviewPlanSnapshotClaim, data []byte, computedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE review_plan_snapshots
		   SET data = $4::jsonb, computed_at = $6, computed_seq = $5,
		       lease_until = NULL, lease_token = '',
		       first_dirty_at = CASE WHEN dirty_seq > $5 THEN claimed_at END
		 WHERE project_id = $1 AND stream = $2 AND lease_token = $3`,
		claim.ProjectID, claim.Stream, claim.Token, string(data), claim.Seq, computedAt.UTC()); err != nil {
		return fmt.Errorf("store the review plan snapshot of %s: %w", claim.ProjectID, err)
	}
	return nil
}

// ReleaseReviewPlanSnapshot implements platstore.ReviewPlanSnapshotStore. The
// lease stays until retry passes, which spaces out the attempts at a
// snapshot whose compute keeps failing.
func (s *PostgresStore) ReleaseReviewPlanSnapshot(ctx context.Context, claim platstore.ReviewPlanSnapshotClaim, retry time.Duration) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE review_plan_snapshots
		   SET lease_until = NOW() + make_interval(secs => $4), lease_token = ''
		 WHERE project_id = $1 AND stream = $2 AND lease_token = $3`,
		claim.ProjectID, claim.Stream, claim.Token, retry.Seconds()); err != nil {
		return fmt.Errorf("release the review plan snapshot of %s: %w", claim.ProjectID, err)
	}
	return nil
}

// MarkReviewPlanSnapshotsChanged implements platstore.ReviewPlanSnapshotStore.
func (s *PostgresStore) MarkReviewPlanSnapshotsChanged(ctx context.Context, projectIDs, workspaces []string) error {
	if len(projectIDs) == 0 && len(workspaces) == 0 {
		return nil
	}
	if projectIDs == nil {
		projectIDs = []string{}
	}
	if workspaces == nil {
		workspaces = []string{}
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE review_plan_snapshots
		   SET dirty_seq = dirty_seq + 1, dirty_at = NOW(), first_dirty_at = COALESCE(first_dirty_at, NOW())
		 WHERE project_id = ANY($1) OR workspace_id = ANY($2) OR workspace_slug = ANY($2)`,
		projectIDs, workspaces); err != nil {
		return fmt.Errorf("mark review plan snapshots changed: %w", err)
	}
	return nil
}

// claimToken is a random token naming one claim.
func claimToken() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

var _ platstore.ReviewPlanSnapshotStore = (*PostgresStore)(nil)
