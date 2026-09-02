package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// ContextScanJobStore persists brand-scan job state (epic 016). The CRUD
// surface mirrors ExtractionJobStore; the claim/lease/retry/sweep methods
// mirror the translation JobStore because brand scans are billed — the
// claim-epoch lease is what keeps a swept-and-retried job from double-billing
// (see JobStore.ClaimJob / RenewLease for the semantics).
type ContextScanJobStore interface {
	CreateContextScanJob(ctx context.Context, job *ContextScanJob) error
	GetContextScanJob(ctx context.Context, id string) (*ContextScanJob, error)
	// ListContextScanJobs returns a workspace's scans, newest first, capped at
	// limit. A scan carries no project, collection or coordinate, so the
	// workspace slug is the whole of its scope.
	ListContextScanJobs(ctx context.Context, workspaceSlug string, limit int) ([]*ContextScanJob, error)
	// UpdateContextScanJobProgress records a progress milestone (0-100) with a
	// short machine-readable message (e.g. "drafting-voice"). Guarded by the
	// claim epoch (status 'processing' AND claim_epoch == epoch) so a stale
	// worker can neither regress the fresh owner's progress nor falsely
	// refresh its heartbeat; a lost-lease write is a silent no-op.
	UpdateContextScanJobProgress(ctx context.Context, id string, epoch int64, progress int, message string) error
	UpdateContextScanJobStatus(ctx context.Context, id string, status ContextScanJobStatus, errMsg string) error
	// FailContextScanJob marks the job failed with errMsg — but only while the
	// caller still holds the lease (status 'processing' AND claim_epoch ==
	// epoch), so a stale worker's permanent error cannot overwrite a fresh
	// owner's completed (already billed) run. Returns owner=false when the
	// lease was lost, in which case nothing was written.
	FailContextScanJob(ctx context.Context, id string, epoch int64, errMsg string) (owner bool, err error)
	// AddContextScanTokens accumulates billed token usage onto the job as each
	// phase is metered, while the caller still holds the lease. This keeps
	// tokens_used truthful for a scan that fails AFTER a billed phase — the
	// credits were deducted, so the job must report the spend.
	AddContextScanTokens(ctx context.Context, id string, epoch int64, tokens int) error
	// CompleteContextScanJob atomically persists the result and marks the job
	// completed — but only while the caller still holds the lease (status
	// 'processing' AND claim_epoch == epoch). Returns owner=false when the
	// lease was lost, in which case nothing was written. tokens_used keeps the
	// larger of the accumulated (AddContextScanTokens) and attempt-local totals,
	// so a retried run's earlier billed phases are never erased.
	CompleteContextScanJob(ctx context.Context, id string, epoch int64, result json.RawMessage, tokensUsed int) (owner bool, err error)
	DeleteContextScanJob(ctx context.Context, id string) error
	// ClaimContextScanJob atomically transitions queued → processing and bumps
	// claim_epoch (the lease generation). See JobStore.ClaimJob.
	ClaimContextScanJob(ctx context.Context, id string) (claimed bool, epoch int64, err error)
	// RenewLease refreshes the heartbeat while the caller holds the lease.
	// See JobStore.RenewLease. Shares its name with the translation store so
	// both satisfy the worker's lease-renewal interface.
	RenewLease(ctx context.Context, id string, epoch int64) (owner bool, err error)
	// RetryOrFail records a transient failure. See JobStore.RetryOrFail.
	// Epoch-guarded: a stale worker's transient error must not knock the fresh
	// owner's in-flight run back to 'queued' (spawning a duplicate) or eat its
	// retry budget.
	RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (retry bool, err error)
	// SweepStaleProcessing recovers jobs abandoned mid-processing. See
	// JobStore.SweepStaleProcessing. Shares its name with the translation
	// store so both satisfy the StaleJobSweeper's store interface.
	SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) (requeued []string, failed int, err error)
	// RevertSweepRequeue rolls back a sweep requeue whose re-enqueue failed.
	// See JobStore.RevertSweepRequeue.
	RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error
	// SweepExpiredContextScanUploads returns terminal (completed|failed) jobs
	// older than olderThan whose stored request still carries upload_keys —
	// the retention candidates for upload-envelope deletion. The grace window
	// is what keeps Regenerate working: a re-run reuses the original upload
	// keys, so the envelopes must outlive the job by the window.
	SweepExpiredContextScanUploads(ctx context.Context, olderThan time.Duration, limit int) ([]ContextScanUploadCleanup, error)
	// ClearContextScanUploadKeys removes upload_keys from the stored request
	// once the envelopes are deleted, so the next sweep does not re-select
	// the job. The rest of the request is preserved.
	ClearContextScanUploadKeys(ctx context.Context, id string) error
}

// ContextScanMigrations is the context-scan jobs schema as a single consolidated
// baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  create the scan jobs table
//	2  scan jobs baseline (folds 1)
//
// Baseline is version 3 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 4.
var ContextScanMigrations = []storage.Migration{
	{
		Version:     3,
		Description: "context scan jobs baseline (folds 1) + the rename off the former name",
		SQL: `
			-- This subsystem's ledger table was renamed with it, so the baseline
			-- replays against an empty ledger on a database that already has the
			-- jobs table under its former name. The CREATE below would then make
			-- a second, empty table and strand every existing row beside it,
			-- which is exactly how a rename that reached only the CREATE took
			-- production down (auth v10). So the rename runs first, guarded,
			-- and the CREATE is the no-op it should be.
			DO $$
			BEGIN
				IF EXISTS (SELECT 1 FROM information_schema.tables
				           WHERE table_schema = current_schema() AND table_name = 'brand_scan_jobs')
				   AND NOT EXISTS (SELECT 1 FROM information_schema.tables
				                   WHERE table_schema = current_schema() AND table_name = 'context_scan_jobs') THEN
					ALTER TABLE brand_scan_jobs RENAME TO context_scan_jobs;
				END IF;
			END $$;
			-- RENAME TO carries the rows but not the index names, and the
			-- CREATE INDEX IF NOT EXISTS below would then build a second index
			-- over the same columns beside each original.
			ALTER INDEX IF EXISTS idx_brand_scan_jobs_workspace RENAME TO idx_context_scan_jobs_workspace;
			ALTER INDEX IF EXISTS idx_brand_scan_jobs_status RENAME TO idx_context_scan_jobs_status;
			ALTER INDEX IF EXISTS idx_brand_scan_jobs_processing_updated RENAME TO idx_context_scan_jobs_processing_updated;

			CREATE TABLE IF NOT EXISTS context_scan_jobs (
				id             TEXT PRIMARY KEY,
				workspace_id   TEXT NOT NULL DEFAULT '',
				workspace_slug TEXT NOT NULL DEFAULT '',
				status         TEXT NOT NULL DEFAULT 'queued',
				progress       INTEGER NOT NULL DEFAULT 0,
				message        TEXT NOT NULL DEFAULT '',
				request        JSONB NOT NULL DEFAULT '{}'::jsonb,
				result         JSONB,
				error          TEXT NOT NULL DEFAULT '',
				attempts       INTEGER NOT NULL DEFAULT 0,
				claim_epoch    BIGINT NOT NULL DEFAULT 0,
				tokens_used    INTEGER NOT NULL DEFAULT 0,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_context_scan_jobs_workspace
				ON context_scan_jobs(workspace_slug, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_context_scan_jobs_status
				ON context_scan_jobs(status);
			-- Partial index over the columns the stale-job sweeper scans.
			CREATE INDEX IF NOT EXISTS idx_context_scan_jobs_processing_updated
				ON context_scan_jobs(updated_at) WHERE status = 'processing';
		`,
	},
}

// contextScanJobStore implements ContextScanJobStore using PostgreSQL.
type contextScanJobStore struct {
	db *storage.PgDB
}

// NewContextScanJobStore creates a PostgreSQL-backed ContextScanJobStore, running
// its (namespaced, idempotent) migrations first.
func NewContextScanJobStore(db *storage.PgDB) (ContextScanJobStore, error) {
	if err := storage.MigratePostgresNS(db, "context_scan_schema_migrations", ContextScanMigrations); err != nil {
		return nil, fmt.Errorf("migrate brand-scan schema: %w", err)
	}
	return &contextScanJobStore{db: db}, nil
}

const contextScanJobColumns = `id, workspace_id, workspace_slug, status, progress, message,
	request, result, error, attempts, claim_epoch, tokens_used, created_at, updated_at`

func (s *contextScanJobStore) CreateContextScanJob(ctx context.Context, job *ContextScanJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = ContextScanStatusQueued
	}
	request := job.Request
	if len(request) == 0 {
		request = json.RawMessage(`{}`)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO context_scan_jobs
			(id, workspace_id, workspace_slug, status, progress, message,
			 request, result, error, attempts, claim_epoch, tokens_used, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		job.ID, job.WorkspaceID, job.WorkspaceSlug, string(job.Status), job.Progress, job.Message,
		[]byte(request), nullableJSON(job.Result), job.Error, job.Attempts, job.ClaimEpoch,
		job.TokensUsed, now, now)
	if err != nil {
		return fmt.Errorf("insert brand-scan job: %w", err)
	}
	return nil
}

// nullableJSON maps an empty raw message to SQL NULL for the result column.
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

func (s *contextScanJobStore) GetContextScanJob(ctx context.Context, id string) (*ContextScanJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+contextScanJobColumns+` FROM context_scan_jobs WHERE id = $1`, id)

	var j ContextScanJob
	var status string
	var request, result []byte
	err := row.Scan(
		&j.ID, &j.WorkspaceID, &j.WorkspaceSlug, &status, &j.Progress, &j.Message,
		&request, &result, &j.Error, &j.Attempts, &j.ClaimEpoch, &j.TokensUsed,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan brand-scan job: %w", err)
	}
	j.Status = ContextScanJobStatus(status)
	j.Request = json.RawMessage(request)
	if len(result) > 0 {
		j.Result = json.RawMessage(result)
	}
	return &j, nil
}

func (s *contextScanJobStore) ListContextScanJobs(ctx context.Context, workspaceSlug string, limit int) ([]*ContextScanJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+contextScanJobColumns+` FROM context_scan_jobs
		 WHERE workspace_slug = $1 ORDER BY created_at DESC LIMIT $2`, workspaceSlug, limit)
	if err != nil {
		return nil, fmt.Errorf("list brand-scan jobs: %w", err)
	}
	defer rows.Close()

	var out []*ContextScanJob
	for rows.Next() {
		var j ContextScanJob
		var status string
		var request, result []byte
		if err := rows.Scan(
			&j.ID, &j.WorkspaceID, &j.WorkspaceSlug, &status, &j.Progress, &j.Message,
			&request, &result, &j.Error, &j.Attempts, &j.ClaimEpoch, &j.TokensUsed,
			&j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan brand-scan job: %w", err)
		}
		j.Status = ContextScanJobStatus(status)
		j.Request = json.RawMessage(request)
		if len(result) > 0 {
			j.Result = json.RawMessage(result)
		}
		out = append(out, &j)
	}
	return out, rows.Err()
}

func (s *contextScanJobStore) UpdateContextScanJobProgress(ctx context.Context, id string, epoch int64, progress int, message string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs SET progress = $1, message = $2, updated_at = NOW()
		 WHERE id = $3 AND status = 'processing' AND claim_epoch = $4`,
		progress, message, id, epoch)
	if err != nil {
		return fmt.Errorf("update brand-scan job progress: %w", err)
	}
	return nil
}

func (s *contextScanJobStore) UpdateContextScanJobStatus(ctx context.Context, id string, status ContextScanJobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs SET status = $1, error = $2, updated_at = NOW() WHERE id = $3`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update brand-scan job status: %w", err)
	}
	return nil
}

func (s *contextScanJobStore) FailContextScanJob(ctx context.Context, id string, epoch int64, errMsg string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs
		 SET status = 'failed', error = $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'processing' AND claim_epoch = $3`,
		errMsg, id, epoch)
	if err != nil {
		return false, fmt.Errorf("fail brand-scan job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *contextScanJobStore) AddContextScanTokens(ctx context.Context, id string, epoch int64, tokens int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs
		 SET tokens_used = tokens_used + $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'processing' AND claim_epoch = $3`,
		tokens, id, epoch)
	if err != nil {
		return fmt.Errorf("add brand-scan tokens: %w", err)
	}
	return nil
}

func (s *contextScanJobStore) CompleteContextScanJob(ctx context.Context, id string, epoch int64, result json.RawMessage, tokensUsed int) (bool, error) {
	// GREATEST keeps the AddContextScanTokens accumulation when it exceeds the
	// attempt-local total (earlier billed-then-retried phases), and repairs it
	// upward when an accumulation write was lost.
	res, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs
		 SET status = 'completed', progress = 100, message = 'done',
		     result = $1, tokens_used = GREATEST(tokens_used, $2), error = '', updated_at = NOW()
		 WHERE id = $3 AND status = 'processing' AND claim_epoch = $4`,
		nullableJSON(result), tokensUsed, id, epoch)
	if err != nil {
		return false, fmt.Errorf("complete brand-scan job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *contextScanJobStore) DeleteContextScanJob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM context_scan_jobs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete brand-scan job: %w", err)
	}
	return nil
}

func (s *contextScanJobStore) ClaimContextScanJob(ctx context.Context, id string) (bool, int64, error) {
	var epoch int64
	err := s.db.QueryRowContext(ctx,
		`UPDATE context_scan_jobs
		 SET status = 'processing', claim_epoch = claim_epoch + 1, updated_at = NOW()
		 WHERE id = $1 AND status = 'queued'
		 RETURNING claim_epoch`, id).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		// Not queued (already processing/terminal): another worker won.
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("claim brand-scan job: %w", err)
	}
	return true, epoch, nil
}

func (s *contextScanJobStore) RenewLease(ctx context.Context, id string, epoch int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs SET updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $2`, id, epoch)
	if err != nil {
		return false, fmt.Errorf("renew brand-scan lease: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *contextScanJobStore) RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (bool, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	// Same single-statement retry verdict as the translation store: with budget
	// left the row goes back to 'queued' for redelivery; otherwise it fails.
	// The claim_epoch guard makes a stale worker's transient error a no-op so
	// it cannot requeue (duplicate-run) the fresh owner's in-flight job or eat
	// its retry budget.
	var requeued bool
	err := s.db.QueryRowContext(ctx,
		`UPDATE context_scan_jobs
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
		return false, fmt.Errorf("retry-or-fail brand-scan job: %w", err)
	}
	return requeued, nil
}

func (s *contextScanJobStore) SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) ([]string, int, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	cutoff := time.Now().UTC().Add(-olderThan)

	// Phase 1: requeue stalled jobs that still have retry budget.
	// A stranded 'queued' row is swept alongside a stalled 'processing' one.
	// Nothing else covers it: an enqueue that failed after the row was written
	// — the create handler's rollback, or the sweep's own re-enqueue — leaves a
	// job nobody will ever deliver. Re-enqueueing one that IS merely waiting is
	// harmless, because ClaimJob admits exactly one worker.
	rows, err := s.db.QueryContext(ctx,
		`UPDATE context_scan_jobs
		 SET status = 'queued', attempts = attempts + 1, updated_at = NOW()
		 WHERE status IN ('processing', 'queued') AND updated_at < $1 AND attempts + 1 < $2
		 RETURNING id`,
		cutoff, maxAttempts)
	if err != nil {
		return nil, 0, fmt.Errorf("sweep requeue stale brand-scan jobs: %w", err)
	}
	var requeued []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan requeued brand-scan job id: %w", scanErr)
		}
		requeued = append(requeued, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate requeued brand-scan jobs: %w", rowsErr)
	}
	_ = rows.Close()

	// Phase 2: fail stalled jobs out of retry budget (disjoint from phase 1).
	res, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs
		 SET status = 'failed', attempts = attempts + 1,
		     error = 'stalled before completion; exceeded max attempts', updated_at = NOW()
		 WHERE status IN ('processing', 'queued') AND updated_at < $1 AND attempts + 1 >= $2`,
		cutoff, maxAttempts)
	if err != nil {
		return requeued, 0, fmt.Errorf("sweep fail exhausted brand-scan jobs: %w", err)
	}
	failed, _ := res.RowsAffected()
	return requeued, int(failed), nil
}

func (s *contextScanJobStore) SweepExpiredContextScanUploads(ctx context.Context, olderThan time.Duration, limit int) ([]ContextScanUploadCleanup, error) {
	if limit <= 0 {
		limit = 100
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, request->'upload_keys'
		 FROM context_scan_jobs
		 WHERE status IN ('completed', 'failed')
		   AND updated_at < $1
		   AND jsonb_typeof(request->'upload_keys') = 'array'
		   AND jsonb_array_length(request->'upload_keys') > 0
		 ORDER BY updated_at
		 LIMIT $2`,
		cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("sweep expired brand-scan uploads: %w", err)
	}
	defer rows.Close()

	var out []ContextScanUploadCleanup
	for rows.Next() {
		var c ContextScanUploadCleanup
		var keysJSON []byte
		if scanErr := rows.Scan(&c.JobID, &keysJSON); scanErr != nil {
			return nil, fmt.Errorf("scan expired brand-scan upload row: %w", scanErr)
		}
		if decErr := json.Unmarshal(keysJSON, &c.UploadKeys); decErr != nil {
			return nil, fmt.Errorf("decode brand-scan upload keys: %w", decErr)
		}
		out = append(out, c)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate expired brand-scan uploads: %w", rowsErr)
	}
	return out, nil
}

func (s *contextScanJobStore) ClearContextScanUploadKeys(ctx context.Context, id string) error {
	// updated_at is deliberately left untouched: the row is terminal, and its
	// timestamp should keep reflecting when the scan finished.
	_, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs SET request = request - 'upload_keys' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("clear brand-scan upload keys: %w", err)
	}
	return nil
}

func (s *contextScanJobStore) RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error {
	// Same rollback as the translation store: age updated_at past the sweep
	// cutoff so the next sweep re-selects the row, and undo the sweep's
	// attempts increment so a failed enqueue never eats retry budget.
	stale := time.Now().UTC().Add(-staleThreshold - time.Minute)
	_, err := s.db.ExecContext(ctx,
		`UPDATE context_scan_jobs
		 SET status = 'processing', attempts = GREATEST(attempts - 1, 0), updated_at = $2
		 WHERE id = $1 AND status = 'queued'`,
		id, stale)
	if err != nil {
		return fmt.Errorf("revert brand-scan sweep requeue: %w", err)
	}
	return nil
}
