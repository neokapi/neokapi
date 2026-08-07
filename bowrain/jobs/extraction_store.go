package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// ExtractionJobStore persists extraction job state.
//
// The retry and lease surface mirrors the translation JobStore, method for
// method, so one sweeper implementation recovers every leased job type and an
// extraction that hits a provider 503 is retried rather than stranded in
// 'processing' until the push-completion tracker gives up on it.
type ExtractionJobStore interface {
	CreateExtractionJob(ctx context.Context, job *ExtractionJob) error
	GetExtractionJob(ctx context.Context, id string) (*ExtractionJob, error)
	UpdateExtractionJobStatus(ctx context.Context, id string, status ExtractionJobStatus, errMsg string) error
	UpdateExtractionJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks, itemsCreated int) error
	ListByPushID(ctx context.Context, pushID string) ([]*ExtractionJob, error)
	// ClaimExtractionJob atomically transitions queued → processing and bumps
	// the claim epoch. See JobStore.ClaimJob.
	ClaimExtractionJob(ctx context.Context, id string) (claimed bool, epoch int64, err error)
	// RenewLease refreshes the heartbeat while the caller holds the lease.
	// See JobStore.RenewLease.
	RenewLease(ctx context.Context, id string, epoch int64) (owner bool, err error)
	// RetryOrFail records a transient failure. See JobStore.RetryOrFail.
	RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (retry bool, err error)
	// FailExtractionJob marks the job failed while the caller still holds the
	// lease. See JobStore.FailJob.
	FailExtractionJob(ctx context.Context, id string, epoch int64, errMsg string) (owner bool, err error)
	// SweepStaleProcessing recovers jobs abandoned mid-processing. See
	// JobStore.SweepStaleProcessing.
	SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) (requeued []string, failed int, err error)
	// RevertSweepRequeue rolls back a sweep requeue whose re-enqueue failed.
	// See JobStore.RevertSweepRequeue.
	RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error
}

// ExtractionMigrations is the extraction-jobs schema as a single consolidated
// baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  create extraction_jobs table
//	2  extraction jobs baseline (folded 1)
//
// The subsystem carries exactly one baseline (migrations/schema_test.go
// enforces it), so a schema change is made by editing the baseline in place and
// bumping its version. Version 3 adds the attempt and lease bookkeeping the
// translation jobs have always had.
//
// Baseline is version 3 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 4.
var ExtractionMigrations = []storage.Migration{
	{
		Version:     3,
		Description: "extraction jobs baseline (folds 1-2) + attempts and lease",
		SQL: `
			CREATE TABLE IF NOT EXISTS extraction_jobs (
				id             TEXT PRIMARY KEY,
				workspace_slug TEXT NOT NULL,
				project_id     TEXT NOT NULL,
				item_name      TEXT NOT NULL,
				locale         TEXT NOT NULL DEFAULT '',
				push_id        TEXT NOT NULL DEFAULT '',
				step_id        TEXT NOT NULL DEFAULT '',
				model          TEXT NOT NULL DEFAULT '',
				status         TEXT NOT NULL DEFAULT 'queued',
				total_blocks   INTEGER NOT NULL DEFAULT 0,
				done_blocks    INTEGER NOT NULL DEFAULT 0,
				items_created  INTEGER NOT NULL DEFAULT 0,

				-- Retry budget, spent on transient upstream failures.
				attempts       INTEGER NOT NULL DEFAULT 0,

				-- The lease generation, bumped on every claim, so a worker that
				-- is swept and re-claimed can tell it no longer owns the job.
				claim_epoch    BIGINT NOT NULL DEFAULT 0,

				error          TEXT NOT NULL DEFAULT '',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			-- Serves a database that already has extraction_jobs, where the
			-- CREATE above is a no-op.
			ALTER TABLE extraction_jobs ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE extraction_jobs ADD COLUMN IF NOT EXISTS claim_epoch BIGINT NOT NULL DEFAULT 0;
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_project ON extraction_jobs(project_id);
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_status ON extraction_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_push_id ON extraction_jobs(push_id);
			-- Partial index over the columns the stale-job sweeper scans.
			CREATE INDEX IF NOT EXISTS idx_extraction_jobs_processing_updated
				ON extraction_jobs(updated_at) WHERE status = 'processing';
		`,
	},
}

// extractionJobStore implements ExtractionJobStore using PostgreSQL.
type extractionJobStore struct {
	db *storage.PgDB
}

// NewExtractionJobStore creates a PostgreSQL-backed ExtractionJobStore.
func NewExtractionJobStore(db *storage.PgDB) (ExtractionJobStore, error) {
	if err := storage.MigratePostgresNS(db, "extraction_schema_migrations", ExtractionMigrations); err != nil {
		return nil, fmt.Errorf("migrate extraction schema: %w", err)
	}
	return &extractionJobStore{db: db}, nil
}

func (s *extractionJobStore) CreateExtractionJob(ctx context.Context, job *ExtractionJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = ExtractionStatusQueued
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO extraction_jobs
			(id, workspace_slug, project_id, item_name, locale, push_id, step_id, model,
			 status, total_blocks, done_blocks, items_created, error, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		job.ID, job.WorkspaceSlug, job.ProjectID, job.ItemName, job.Locale,
		job.PushID, job.StepID, job.Model, string(job.Status), job.TotalBlocks,
		job.DoneBlocks, job.ItemsCreated, job.Error, now, now)
	if err != nil {
		return fmt.Errorf("insert extraction job: %w", err)
	}
	return nil
}

func (s *extractionJobStore) GetExtractionJob(ctx context.Context, id string) (*ExtractionJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, locale, push_id, step_id, model,
				status, total_blocks, done_blocks, items_created, error, created_at, updated_at
		 FROM extraction_jobs WHERE id = $1`, id)

	var j ExtractionJob
	var status string
	err := row.Scan(
		&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.Locale,
		&j.PushID, &j.StepID, &j.Model, &status, &j.TotalBlocks, &j.DoneBlocks,
		&j.ItemsCreated, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan extraction job: %w", err)
	}
	j.Status = ExtractionJobStatus(status)
	return &j, nil
}

func (s *extractionJobStore) UpdateExtractionJobStatus(ctx context.Context, id string, status ExtractionJobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET status = $1, error = $2, updated_at = NOW() WHERE id = $3`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update extraction job status: %w", err)
	}
	return nil
}

func (s *extractionJobStore) UpdateExtractionJobProgress(ctx context.Context, id string, doneBlocks, totalBlocks, itemsCreated int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET done_blocks = $1, total_blocks = $2, items_created = $3, updated_at = NOW() WHERE id = $4`,
		doneBlocks, totalBlocks, itemsCreated, id)
	if err != nil {
		return fmt.Errorf("update extraction job progress: %w", err)
	}
	return nil
}

func (s *extractionJobStore) ClaimExtractionJob(ctx context.Context, id string) (bool, int64, error) {
	var epoch int64
	err := s.db.QueryRowContext(ctx,
		`UPDATE extraction_jobs
		 SET status = 'processing', claim_epoch = claim_epoch + 1, updated_at = NOW()
		 WHERE id = $1 AND status = 'queued'
		 RETURNING claim_epoch`, id).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("claim extraction job: %w", err)
	}
	return true, epoch, nil
}

func (s *extractionJobStore) RenewLease(ctx context.Context, id string, epoch int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs SET updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $2`, id, epoch)
	if err != nil {
		return false, fmt.Errorf("renew extraction lease: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *extractionJobStore) RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (bool, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var requeued bool
	err := s.db.QueryRowContext(ctx,
		`UPDATE extraction_jobs
		 SET attempts = attempts + 1,
		     status = CASE WHEN attempts + 1 < $2 THEN 'queued' ELSE 'failed' END,
		     error  = CASE WHEN attempts + 1 < $2 THEN error ELSE $3 END,
		     updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $4
		 RETURNING status = 'queued'`,
		id, maxAttempts, errMsg, epoch).Scan(&requeued)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("retry-or-fail extraction job: %w", err)
	}
	return requeued, nil
}

func (s *extractionJobStore) FailExtractionJob(ctx context.Context, id string, epoch int64, errMsg string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs
		 SET status = 'failed', error = $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'processing' AND claim_epoch = $3`,
		errMsg, id, epoch)
	if err != nil {
		return false, fmt.Errorf("fail extraction job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *extractionJobStore) SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) ([]string, int, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	cutoff := time.Now().UTC().Add(-olderThan)

	// A stranded 'queued' row is swept alongside a stalled 'processing' one.
	// Nothing else covers it: an enqueue that failed after the row was written
	// — the create handler's rollback, or the sweep's own re-enqueue — leaves a
	// job nobody will ever deliver. Re-enqueueing one that IS merely waiting is
	// harmless, because ClaimJob admits exactly one worker.
	rows, err := s.db.QueryContext(ctx,
		`UPDATE extraction_jobs
		 SET status = 'queued', attempts = attempts + 1, updated_at = NOW()
		 WHERE status IN ('processing', 'queued') AND updated_at < $1 AND attempts + 1 < $2
		 RETURNING id`,
		cutoff, maxAttempts)
	if err != nil {
		return nil, 0, fmt.Errorf("sweep requeue stale extraction jobs: %w", err)
	}
	var requeued []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan requeued extraction job id: %w", scanErr)
		}
		requeued = append(requeued, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate requeued extraction jobs: %w", rowsErr)
	}
	_ = rows.Close()

	res, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs
		 SET status = 'failed', attempts = attempts + 1,
		     error = 'stalled before completion; exceeded max attempts', updated_at = NOW()
		 WHERE status IN ('processing', 'queued') AND updated_at < $1 AND attempts + 1 >= $2`,
		cutoff, maxAttempts)
	if err != nil {
		return requeued, 0, fmt.Errorf("sweep fail exhausted extraction jobs: %w", err)
	}
	failed, _ := res.RowsAffected()
	return requeued, int(failed), nil
}

func (s *extractionJobStore) RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error {
	stale := time.Now().UTC().Add(-staleThreshold - time.Minute)
	_, err := s.db.ExecContext(ctx,
		`UPDATE extraction_jobs
		 SET status = 'processing', attempts = GREATEST(attempts - 1, 0), updated_at = $2
		 WHERE id = $1 AND status = 'queued'`,
		id, stale)
	if err != nil {
		return fmt.Errorf("revert extraction sweep requeue: %w", err)
	}
	return nil
}

func (s *extractionJobStore) ListByPushID(ctx context.Context, pushID string) ([]*ExtractionJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, locale, push_id, step_id, model,
				status, total_blocks, done_blocks, items_created, error, created_at, updated_at
		 FROM extraction_jobs WHERE push_id = $1 ORDER BY created_at`, pushID)
	if err != nil {
		return nil, fmt.Errorf("list extraction jobs by push: %w", err)
	}
	defer rows.Close()

	var result []*ExtractionJob
	for rows.Next() {
		var j ExtractionJob
		var status string
		if err := rows.Scan(
			&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.Locale,
			&j.PushID, &j.StepID, &j.Model, &status, &j.TotalBlocks, &j.DoneBlocks,
			&j.ItemsCreated, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan extraction job: %w", err)
		}
		j.Status = ExtractionJobStatus(status)
		result = append(result, &j)
	}
	return result, rows.Err()
}
