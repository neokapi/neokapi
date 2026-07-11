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

// BrandScanJobStore persists brand-scan job state (epic 016). The CRUD
// surface mirrors ExtractionJobStore; the claim/lease/retry/sweep methods
// mirror the translation JobStore because brand scans are billed — the
// claim-epoch lease is what keeps a swept-and-retried job from double-billing
// (see JobStore.ClaimJob / RenewLease for the semantics).
type BrandScanJobStore interface {
	CreateBrandScanJob(ctx context.Context, job *BrandScanJob) error
	GetBrandScanJob(ctx context.Context, id string) (*BrandScanJob, error)
	// UpdateBrandScanJobProgress records a progress milestone (0-100) with a
	// short machine-readable message (e.g. "drafting-voice"). Guarded by the
	// claim epoch (status 'processing' AND claim_epoch == epoch) so a stale
	// worker can neither regress the fresh owner's progress nor falsely
	// refresh its heartbeat; a lost-lease write is a silent no-op.
	UpdateBrandScanJobProgress(ctx context.Context, id string, epoch int64, progress int, message string) error
	UpdateBrandScanJobStatus(ctx context.Context, id string, status BrandScanJobStatus, errMsg string) error
	// FailBrandScanJob marks the job failed with errMsg — but only while the
	// caller still holds the lease (status 'processing' AND claim_epoch ==
	// epoch), so a stale worker's permanent error cannot overwrite a fresh
	// owner's completed (already billed) run. Returns owner=false when the
	// lease was lost, in which case nothing was written.
	FailBrandScanJob(ctx context.Context, id string, epoch int64, errMsg string) (owner bool, err error)
	// AddBrandScanTokens accumulates billed token usage onto the job as each
	// phase is metered, while the caller still holds the lease. This keeps
	// tokens_used truthful for a scan that fails AFTER a billed phase — the
	// credits were deducted, so the job must report the spend.
	AddBrandScanTokens(ctx context.Context, id string, epoch int64, tokens int) error
	// CompleteBrandScanJob atomically persists the result and marks the job
	// completed — but only while the caller still holds the lease (status
	// 'processing' AND claim_epoch == epoch). Returns owner=false when the
	// lease was lost, in which case nothing was written. tokens_used keeps the
	// larger of the accumulated (AddBrandScanTokens) and attempt-local totals,
	// so a retried run's earlier billed phases are never erased.
	CompleteBrandScanJob(ctx context.Context, id string, epoch int64, result json.RawMessage, tokensUsed int) (owner bool, err error)
	DeleteBrandScanJob(ctx context.Context, id string) error
	// ClaimBrandScanJob atomically transitions queued → processing and bumps
	// claim_epoch (the lease generation). See JobStore.ClaimJob.
	ClaimBrandScanJob(ctx context.Context, id string) (claimed bool, epoch int64, err error)
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
	// SweepExpiredBrandScanUploads returns terminal (completed|failed) jobs
	// older than olderThan whose stored request still carries upload_keys —
	// the retention candidates for upload-envelope deletion. The grace window
	// is what keeps Regenerate working: a re-run reuses the original upload
	// keys, so the envelopes must outlive the job by the window.
	SweepExpiredBrandScanUploads(ctx context.Context, olderThan time.Duration, limit int) ([]BrandScanUploadCleanup, error)
	// ClearBrandScanUploadKeys removes upload_keys from the stored request
	// once the envelopes are deleted, so the next sweep does not re-select
	// the job. The rest of the request is preserved.
	ClearBrandScanUploadKeys(ctx context.Context, id string) error
}

// brandScanMigrations defines the PostgreSQL schema for brand-scan jobs.
var brandScanMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create brand_scan_jobs table",
		SQL: `
			CREATE TABLE IF NOT EXISTS brand_scan_jobs (
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
			CREATE INDEX IF NOT EXISTS idx_brand_scan_jobs_workspace
				ON brand_scan_jobs(workspace_slug, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_brand_scan_jobs_status
				ON brand_scan_jobs(status);
			-- Partial index over the columns the stale-job sweeper scans.
			CREATE INDEX IF NOT EXISTS idx_brand_scan_jobs_processing_updated
				ON brand_scan_jobs(updated_at) WHERE status = 'processing';
		`,
	},
}

// brandScanJobStore implements BrandScanJobStore using PostgreSQL.
type brandScanJobStore struct {
	db *storage.PgDB
}

// NewBrandScanJobStore creates a PostgreSQL-backed BrandScanJobStore, running
// its (namespaced, idempotent) migrations first.
func NewBrandScanJobStore(db *storage.PgDB) (BrandScanJobStore, error) {
	if err := storage.MigratePostgresNS(db, "brand_scan_schema_migrations", brandScanMigrations); err != nil {
		return nil, fmt.Errorf("migrate brand-scan schema: %w", err)
	}
	return &brandScanJobStore{db: db}, nil
}

const brandScanJobColumns = `id, workspace_id, workspace_slug, status, progress, message,
	request, result, error, attempts, claim_epoch, tokens_used, created_at, updated_at`

func (s *brandScanJobStore) CreateBrandScanJob(ctx context.Context, job *BrandScanJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = BrandScanStatusQueued
	}
	request := job.Request
	if len(request) == 0 {
		request = json.RawMessage(`{}`)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO brand_scan_jobs
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

func (s *brandScanJobStore) GetBrandScanJob(ctx context.Context, id string) (*BrandScanJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+brandScanJobColumns+` FROM brand_scan_jobs WHERE id = $1`, id)

	var j BrandScanJob
	var status string
	var request, result []byte
	err := row.Scan(
		&j.ID, &j.WorkspaceID, &j.WorkspaceSlug, &status, &j.Progress, &j.Message,
		&request, &result, &j.Error, &j.Attempts, &j.ClaimEpoch, &j.TokensUsed,
		&j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan brand-scan job: %w", err)
	}
	j.Status = BrandScanJobStatus(status)
	j.Request = json.RawMessage(request)
	if len(result) > 0 {
		j.Result = json.RawMessage(result)
	}
	return &j, nil
}

func (s *brandScanJobStore) UpdateBrandScanJobProgress(ctx context.Context, id string, epoch int64, progress int, message string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs SET progress = $1, message = $2, updated_at = NOW()
		 WHERE id = $3 AND status = 'processing' AND claim_epoch = $4`,
		progress, message, id, epoch)
	if err != nil {
		return fmt.Errorf("update brand-scan job progress: %w", err)
	}
	return nil
}

func (s *brandScanJobStore) UpdateBrandScanJobStatus(ctx context.Context, id string, status BrandScanJobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs SET status = $1, error = $2, updated_at = NOW() WHERE id = $3`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update brand-scan job status: %w", err)
	}
	return nil
}

func (s *brandScanJobStore) FailBrandScanJob(ctx context.Context, id string, epoch int64, errMsg string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs
		 SET status = 'failed', error = $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'processing' AND claim_epoch = $3`,
		errMsg, id, epoch)
	if err != nil {
		return false, fmt.Errorf("fail brand-scan job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *brandScanJobStore) AddBrandScanTokens(ctx context.Context, id string, epoch int64, tokens int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs
		 SET tokens_used = tokens_used + $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'processing' AND claim_epoch = $3`,
		tokens, id, epoch)
	if err != nil {
		return fmt.Errorf("add brand-scan tokens: %w", err)
	}
	return nil
}

func (s *brandScanJobStore) CompleteBrandScanJob(ctx context.Context, id string, epoch int64, result json.RawMessage, tokensUsed int) (bool, error) {
	// GREATEST keeps the AddBrandScanTokens accumulation when it exceeds the
	// attempt-local total (earlier billed-then-retried phases), and repairs it
	// upward when an accumulation write was lost.
	res, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs
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

func (s *brandScanJobStore) DeleteBrandScanJob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM brand_scan_jobs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete brand-scan job: %w", err)
	}
	return nil
}

func (s *brandScanJobStore) ClaimBrandScanJob(ctx context.Context, id string) (bool, int64, error) {
	var epoch int64
	err := s.db.QueryRowContext(ctx,
		`UPDATE brand_scan_jobs
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

func (s *brandScanJobStore) RenewLease(ctx context.Context, id string, epoch int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs SET updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $2`, id, epoch)
	if err != nil {
		return false, fmt.Errorf("renew brand-scan lease: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *brandScanJobStore) RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (bool, error) {
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
		`UPDATE brand_scan_jobs
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

func (s *brandScanJobStore) SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) ([]string, int, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	cutoff := time.Now().UTC().Add(-olderThan)

	// Phase 1: requeue stalled jobs that still have retry budget.
	rows, err := s.db.QueryContext(ctx,
		`UPDATE brand_scan_jobs
		 SET status = 'queued', attempts = attempts + 1, updated_at = NOW()
		 WHERE status = 'processing' AND updated_at < $1 AND attempts + 1 < $2
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
		`UPDATE brand_scan_jobs
		 SET status = 'failed', attempts = attempts + 1,
		     error = 'stalled in processing; exceeded max attempts', updated_at = NOW()
		 WHERE status = 'processing' AND updated_at < $1 AND attempts + 1 >= $2`,
		cutoff, maxAttempts)
	if err != nil {
		return requeued, 0, fmt.Errorf("sweep fail exhausted brand-scan jobs: %w", err)
	}
	failed, _ := res.RowsAffected()
	return requeued, int(failed), nil
}

func (s *brandScanJobStore) SweepExpiredBrandScanUploads(ctx context.Context, olderThan time.Duration, limit int) ([]BrandScanUploadCleanup, error) {
	if limit <= 0 {
		limit = 100
	}
	cutoff := time.Now().UTC().Add(-olderThan)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, request->'upload_keys'
		 FROM brand_scan_jobs
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

	var out []BrandScanUploadCleanup
	for rows.Next() {
		var c BrandScanUploadCleanup
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

func (s *brandScanJobStore) ClearBrandScanUploadKeys(ctx context.Context, id string) error {
	// updated_at is deliberately left untouched: the row is terminal, and its
	// timestamp should keep reflecting when the scan finished.
	_, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs SET request = request - 'upload_keys' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("clear brand-scan upload keys: %w", err)
	}
	return nil
}

func (s *brandScanJobStore) RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error {
	// Same rollback as the translation store: age updated_at past the sweep
	// cutoff so the next sweep re-selects the row, and undo the sweep's
	// attempts increment so a failed enqueue never eats retry budget.
	stale := time.Now().UTC().Add(-staleThreshold - time.Minute)
	_, err := s.db.ExecContext(ctx,
		`UPDATE brand_scan_jobs
		 SET status = 'processing', attempts = GREATEST(attempts - 1, 0), updated_at = $2
		 WHERE id = $1 AND status = 'queued'`,
		id, stale)
	if err != nil {
		return fmt.Errorf("revert brand-scan sweep requeue: %w", err)
	}
	return nil
}
