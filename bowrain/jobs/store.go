package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// JobStore persists translation jobs.
type JobStore interface {
	CreateJob(ctx context.Context, job *TranslationJob) error
	GetJob(ctx context.Context, id string) (*TranslationJob, error)
	ListJobs(ctx context.Context, workspaceSlug string, limit int) ([]*TranslationJob, error)
	// UpdateJobProgress records translation progress. Guarded by the claim
	// epoch (status 'processing' AND claim_epoch == epoch) so a stale worker
	// can neither regress the fresh owner's progress nor falsely refresh its
	// heartbeat; a lost-lease write is a silent no-op.
	UpdateJobProgress(ctx context.Context, id string, epoch int64, doneBlocks, totalBlocks int) error
	// UpdateJobMemorySplit records the content memory-first split for the job: viaMemory is the
	// block count filled from the project content memory (recycled), viaAI the count sent to
	// the AI translator. The convergence produce emitter aggregates these across
	// a locale's jobs to report a truthful "content memory N · AI M" (theme A2). Epoch-
	// guarded like UpdateJobProgress so a stale worker cannot overwrite the
	// fresh owner's counts.
	UpdateJobMemorySplit(ctx context.Context, id string, epoch int64, viaMemory, viaAI int) error
	UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error
	// CancelJob stops a job a person asked to stop. It applies only to a job
	// still queued or processing — a completed job cannot be cancelled after
	// the fact, and reporting it failed would be a lie the push aggregation
	// then repeats — and it bumps claim_epoch, which is what makes the
	// cancellation reach a worker: the running worker's next RenewLease
	// reports !owner and it abandons at the chunk boundary without writing
	// 'completed' back over the cancellation. Returns false when the job was
	// already terminal.
	CancelJob(ctx context.Context, id, reason string) (cancelled bool, err error)
	// FailJob marks the job failed with errMsg — but only while the caller
	// still holds the lease (status 'processing' AND claim_epoch == epoch), so
	// a stale worker's permanent error cannot overwrite a fresh owner's
	// completed run. Returns owner=false when the lease was lost, in which
	// case nothing was written.
	FailJob(ctx context.Context, id string, epoch int64, errMsg string) (owner bool, err error)
	// CompleteJob marks the job completed under the same lease guard as
	// FailJob. A worker whose lease was taken — by the sweeper's re-claim, or
	// by a cancellation — must not report success for a run somebody else now
	// owns or a person asked to stop.
	CompleteJob(ctx context.Context, id string, epoch int64) (owner bool, err error)
	DeleteJob(ctx context.Context, id string) error
	ListJobsByPushID(ctx context.Context, pushID string) ([]*TranslationJob, error)
	// SetPushGovernance records what a push's review gate did not accept, on
	// the job row the producer polls. A push is answered with 202 and applied
	// by this worker afterwards, so the refusal is discovered long after the
	// request that carried it has been answered, and the row is the only place
	// the producer can still be told.
	SetPushGovernance(ctx context.Context, jobID, report string) error
	// PushGovernance reads that record back for a push. Empty when the push
	// carried no verdict, when every verdict was accepted, or when the push is
	// not yet applied.
	PushGovernance(ctx context.Context, pushID string) (string, error)
	// ClaimJob atomically transitions a job from queued to processing and bumps
	// its claim_epoch (the lease generation). Returns claimed=true with the new
	// epoch if this caller won the claim, or (false, 0, nil) if another worker
	// already claimed it. The caller holds the returned epoch as its lease token
	// and passes it to RenewLease so a job resurrected by the sweeper (which
	// bumps the epoch on re-claim) invalidates the abandoned worker's writes.
	ClaimJob(ctx context.Context, id string) (claimed bool, epoch int64, err error)
	// RenewLease refreshes the job's updated_at heartbeat while the caller still
	// holds the lease (status still 'processing' AND claim_epoch == epoch). It
	// returns owner=false once the job has been swept back to 'queued' or
	// re-claimed by another worker (claim_epoch advanced), signalling the caller
	// to abandon its in-flight work so a fresh owner is not double-billed. It
	// never resurrects a terminal (completed/failed) job.
	RenewLease(ctx context.Context, id string, epoch int64) (owner bool, err error)
	// RetryOrFail records a transient failure for a job currently in
	// 'processing'. If it still has retry budget (attempts < maxAttempts) the
	// job is reset to 'queued' (attempts incremented) and (true, nil) is
	// returned so the caller re-delivers it; once the budget is exhausted the
	// job is marked 'failed' with errMsg and (false, nil) is returned. A job
	// that is no longer 'processing' (raced to a terminal state) yields
	// (false, nil). Epoch-guarded: a stale worker's transient error must not
	// knock the fresh owner's in-flight run back to 'queued' (spawning a
	// duplicate) or eat its retry budget.
	RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (retry bool, err error)
	// DeferJob parks a job whose dependency is known to be down — the circuit
	// breaker rejected the call, so nothing was attempted upstream. The job goes
	// back to 'queued' WITHOUT consuming a retry attempt, because the attempt
	// budget exists to stop a job that keeps failing, not to punish a job that
	// was never tried. An outage therefore costs waiting, not dead jobs.
	//
	// Deferrals are counted separately and capped by maxDeferrals: an upstream
	// that never comes back must still terminate the job rather than cycle it
	// through the queue forever. Exceeding the cap marks the job 'failed' with
	// errMsg and returns (false, nil).
	//
	// Epoch-guarded exactly like RetryOrFail: a stale worker must not requeue
	// the fresh owner's in-flight job. A job no longer in 'processing' yields
	// (false, nil).
	DeferJob(ctx context.Context, id string, epoch int64, maxDeferrals int, errMsg string) (deferred bool, err error)
	// SweepStaleProcessing recovers jobs that have gone quiet for longer than
	// olderThan: stuck in 'processing' because a worker crashed after ClaimJob,
	// or stranded in 'queued' because the enqueue that should have followed the
	// row failed. Jobs with retry budget left are reset to 'queued' — their IDs
	// are returned so the caller can re-enqueue them — and jobs out of budget
	// are marked 'failed'. Returns the requeued IDs and the count of failed jobs.
	SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) (requeued []string, failed int, err error)
	// RevertSweepRequeue rolls a job that SweepStaleProcessing flipped to
	// 'queued' back to 'processing' when the caller's follow-up Enqueue failed,
	// leaving the row with no live broker message. Reverting it to 'processing'
	// with a STALE updated_at (older than staleThreshold) — and undoing the sweep's
	// attempts increment — lets the NEXT sweep re-select and re-enqueue it without
	// having spent budget on a delivery that never happened. Guarded by
	// status='queued' so it never disturbs a job a worker has since claimed.
	RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error
}

// JobMigrations is the translation-jobs schema as a single consolidated
// baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  translation jobs schema
//	2  add workspace_id (billing workspace) to translation_jobs
//	3  translation job retry attempt tracking
//	4  translation job claim lease (epoch) for double-run protection
//	5  content memory-first convergence: record the content memory-vs-AI block split per job
//	6  deferral count for jobs parked on an unavailable dependency
//	7  the first consolidated baseline (folded 1-6)
//	8  stream scope for translation jobs (targets are per-stream overlays)
//	9  the second consolidated baseline (folded 1-8), then created_by beside it
//
// The subsystem carries exactly one baseline (migrations/schema_test.go
// enforces it), so a schema change is made by editing the baseline in place and
// bumping its version. Version 10 adds created_by — who asked for the job.
//
// Every column the baseline gained after a database could already hold
// translation_jobs is declared twice: once in the CREATE, which serves an empty
// database, and once as an ALTER ... ADD COLUMN IF NOT EXISTS, which serves a
// database where CREATE ... IF NOT EXISTS is a no-op and would otherwise leave
// the column missing. Both are idempotent, both land the same column, and
// neither is a second baseline.
//
// Baseline is version 10 — above every number issued, so an existing database
// applies it once and any drift between its schema and its bookkeeping is
// repaired. Retired numbers are never reused; the next migration is version 11.
var JobMigrations = []storage.Migration{
	{
		Version:     10,
		Description: "translation jobs baseline (folds 1-9)",
		SQL: `
			CREATE TABLE IF NOT EXISTS translation_jobs (
				id                 TEXT PRIMARY KEY,
				workspace_slug     TEXT NOT NULL,

				-- The billing workspace id drives per-chunk credit deduction and
				-- Stripe metering in the worker (Epic 004). It is set from the auth
				-- context at enqueue, but the worker reconstitutes the job via
				-- GetJob before translating, so without persisting it,
				-- job.WorkspaceID is always "" at the worker and platform runs are
				-- never metered.
				workspace_id       TEXT NOT NULL DEFAULT '',

				project_id         TEXT NOT NULL,

				-- The stream the job's targets belong to; targets are per-stream
				-- overlays, so a job that does not name one writes where the
				-- pusher did not look. '' means "main", so pre-stream rows and
				-- stream-naive enqueues keep their behavior.
				stream             TEXT NOT NULL DEFAULT '',

				item_name          TEXT NOT NULL,
				target_locale      TEXT NOT NULL,
				provider_config_id TEXT NOT NULL DEFAULT '',
				model              TEXT NOT NULL DEFAULT '',
				push_id            TEXT NOT NULL DEFAULT '',
				step_id            TEXT NOT NULL DEFAULT '',
				status             TEXT NOT NULL DEFAULT 'queued',
				progress           INTEGER NOT NULL DEFAULT 0,
				total_blocks       INTEGER NOT NULL DEFAULT 0,
				done_blocks        INTEGER NOT NULL DEFAULT 0,
				tokens_used        INTEGER NOT NULL DEFAULT 0,
				attempts           INTEGER NOT NULL DEFAULT 0,

				-- A deferral is not a retry: the job was never attempted because
				-- the dependency's circuit was open, so it must not spend attempts.
				-- Counting deferrals separately keeps the two budgets independent:
				-- a job can wait out a long outage and still arrive at the upstream
				-- with its full retry budget intact, while the cap stops an
				-- upstream that never returns from cycling jobs indefinitely.
				deferrals          INTEGER NOT NULL DEFAULT 0,

				-- claim_epoch is the lease generation: ClaimJob bumps it on every
				-- claim so a worker that gets swept-and-reclaimed can detect (via
				-- RenewLease) that it no longer owns the job and must stop before
				-- double-billing the fresh owner's work.
				claim_epoch        BIGINT NOT NULL DEFAULT 0,

				-- A content memory-first convergence run recycles exact/near-exact
				-- matches before paying for AI. via_tm/via_ai carry that split so
				-- the convergence produce emitter can report a truthful
				-- "content memory N · AI M".
				via_tm             INTEGER NOT NULL DEFAULT 0,
				via_ai             INTEGER NOT NULL DEFAULT 0,

				-- The authenticated caller at enqueue, empty for platform-initiated
				-- jobs; failure summons routes on it.
				created_by         TEXT NOT NULL DEFAULT '',

				error              TEXT NOT NULL DEFAULT '',
				created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);
			ALTER TABLE translation_jobs ADD COLUMN IF NOT EXISTS stream TEXT NOT NULL DEFAULT '';
			ALTER TABLE translation_jobs ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
			CREATE INDEX IF NOT EXISTS idx_jobs_workspace ON translation_jobs(workspace_slug, created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_jobs_status ON translation_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_jobs_push_id ON translation_jobs(push_id) WHERE push_id != '';
			CREATE INDEX IF NOT EXISTS idx_jobs_workspace_id ON translation_jobs(workspace_id) WHERE workspace_id != '';
			-- Partial index over the (status, updated_at) columns the stale-job
			-- sweeper scans so recovery stays cheap as the jobs table grows.
			CREATE INDEX IF NOT EXISTS idx_jobs_processing_updated
				ON translation_jobs(updated_at) WHERE status = 'processing';
		`,
	},
	{
		Version:     11,
		Description: "record what a push's review governance refused",
		SQL: `
			-- What a push carried and the platform did not accept: the
			-- approvals and sign-offs the pusher was not entitled to make. The
			-- commit is answered with 202 and applied by a worker, so the
			-- refusal happens after the producer's request is over, and the job
			-- row is where the producer reads it back from.
			ALTER TABLE translation_jobs
				ADD COLUMN IF NOT EXISTS governance_report TEXT NOT NULL DEFAULT '';
		`,
	},
}

// jobStore implements JobStore using PostgreSQL.
type jobStore struct {
	db *storage.PgDB
}

// NewJobStore creates a PostgreSQL-backed JobStore.
// It runs migrations to ensure the translation_jobs table exists.
func NewJobStore(db *storage.PgDB) (JobStore, error) {
	if err := storage.MigratePostgresNS(db, "jobs_schema_migrations", JobMigrations); err != nil {
		return nil, fmt.Errorf("migrate job schema: %w", err)
	}
	return &jobStore{db: db}, nil
}

func (s *jobStore) CreateJob(ctx context.Context, job *TranslationJob) error {
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = StatusQueued
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO translation_jobs
			(id, workspace_slug, project_id, item_name, target_locale, provider_config_id,
			 model, push_id, step_id, status, progress, total_blocks, done_blocks, tokens_used, error, created_at, updated_at, workspace_id, via_tm, via_ai, stream, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		job.ID, job.WorkspaceSlug, job.ProjectID, job.ItemName, job.TargetLocale,
		job.ProviderConfigID, job.Model, job.PushID, job.StepID, string(job.Status), job.Progress, job.TotalBlocks,
		job.DoneBlocks, job.TokensUsed, job.Error, now, now, job.WorkspaceID, job.ViaMemory, job.ViaAI, job.Stream, job.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (s *jobStore) GetJob(ctx context.Context, id string) (*TranslationJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, push_id, step_id, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at, workspace_id, via_tm, via_ai, stream, created_by
		 FROM translation_jobs WHERE id = $1`, id)
	return scanJob(row)
}

func (s *jobStore) ListJobs(ctx context.Context, workspaceSlug string, limit int) ([]*TranslationJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, push_id, step_id, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at, workspace_id, via_tm, via_ai, stream, created_by
		 FROM translation_jobs
		 WHERE workspace_slug = $1
		 ORDER BY created_at DESC
		 LIMIT $2`, workspaceSlug, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	return scanJobs(rows)
}

func (s *jobStore) UpdateJobProgress(ctx context.Context, id string, epoch int64, doneBlocks, totalBlocks int) error {
	progress := 0
	if totalBlocks > 0 {
		progress = doneBlocks * 100 / totalBlocks
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET done_blocks = $1, total_blocks = $2, progress = $3, updated_at = NOW()
		 WHERE id = $4 AND status = 'processing' AND claim_epoch = $5`,
		doneBlocks, totalBlocks, progress, id, epoch)
	if err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	return nil
}

func (s *jobStore) UpdateJobMemorySplit(ctx context.Context, id string, epoch int64, viaMemory, viaAI int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET via_tm = $1, via_ai = $2, updated_at = NOW()
		 WHERE id = $3 AND status = 'processing' AND claim_epoch = $4`,
		viaMemory, viaAI, id, epoch)
	if err != nil {
		return fmt.Errorf("update job tm split: %w", err)
	}
	return nil
}

func (s *jobStore) ClaimJob(ctx context.Context, id string) (bool, int64, error) {
	var epoch int64
	err := s.db.QueryRowContext(ctx,
		`UPDATE translation_jobs
		 SET status = 'processing', claim_epoch = claim_epoch + 1, updated_at = NOW()
		 WHERE id = $1 AND status = 'queued'
		 RETURNING claim_epoch`, id).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		// Not queued (already processing/terminal): another worker won.
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("claim job: %w", err)
	}
	return true, epoch, nil
}

func (s *jobStore) RenewLease(ctx context.Context, id string, epoch int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs SET updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $2`, id, epoch)
	if err != nil {
		return false, fmt.Errorf("renew lease: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *jobStore) RetryOrFail(ctx context.Context, id string, epoch int64, maxAttempts int, errMsg string) (bool, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	// A single statement increments attempts and, in the same row, decides
	// whether there is budget left: with budget it goes back to 'queued' (so a
	// redelivery can re-claim it), otherwise it is marked 'failed'. RETURNING
	// reflects the post-update row, so `status = 'queued'` is the retry verdict.
	// The `status = 'processing'` guard makes a race with a completing worker a
	// no-op (ErrNoRows → no retry), and the claim_epoch guard makes a STALE
	// worker's transient error a no-op so it cannot requeue (duplicate-run) the
	// fresh owner's in-flight job or eat its retry budget.
	var requeued bool
	err := s.db.QueryRowContext(ctx,
		`UPDATE translation_jobs
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
		return false, fmt.Errorf("retry-or-fail job: %w", err)
	}
	return requeued, nil
}

func (s *jobStore) DeferJob(ctx context.Context, id string, epoch int64, maxDeferrals int, errMsg string) (bool, error) {
	if maxDeferrals < 1 {
		maxDeferrals = 1
	}
	// Mirrors RetryOrFail's single-statement shape, but increments `deferrals`
	// and leaves `attempts` alone — that is the whole point of a deferral. The
	// status/epoch guards are identical, so a stale worker can no more defer the
	// fresh owner's job than it can retry it.
	var requeued bool
	err := s.db.QueryRowContext(ctx,
		`UPDATE translation_jobs
		 SET deferrals = deferrals + 1,
		     status = CASE WHEN deferrals + 1 < $2 THEN 'queued' ELSE 'failed' END,
		     error  = CASE WHEN deferrals + 1 < $2 THEN error ELSE $3 END,
		     updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $4
		 RETURNING status = 'queued'`,
		id, maxDeferrals, errMsg, epoch).Scan(&requeued)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("defer job: %w", err)
	}
	return requeued, nil
}

func (s *jobStore) SweepStaleProcessing(ctx context.Context, olderThan time.Duration, maxAttempts int) ([]string, int, error) {
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
		`UPDATE translation_jobs
		 SET status = 'queued', attempts = attempts + 1, updated_at = NOW()
		 WHERE status IN ('processing', 'queued') AND updated_at < $1 AND attempts + 1 < $2
		 RETURNING id`,
		cutoff, maxAttempts)
	if err != nil {
		return nil, 0, fmt.Errorf("sweep requeue stale jobs: %w", err)
	}
	var requeued []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("scan requeued job id: %w", scanErr)
		}
		requeued = append(requeued, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		_ = rows.Close()
		return nil, 0, fmt.Errorf("iterate requeued jobs: %w", rowsErr)
	}
	_ = rows.Close()

	// Phase 2: fail stalled jobs that have exhausted their retry budget. The
	// attempts predicate is disjoint from phase 1, so the two never overlap.
	res, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = 'failed', attempts = attempts + 1,
		     error = 'stalled before completion; exceeded max attempts', updated_at = NOW()
		 WHERE status IN ('processing', 'queued') AND updated_at < $1 AND attempts + 1 >= $2`,
		cutoff, maxAttempts)
	if err != nil {
		return requeued, 0, fmt.Errorf("sweep fail exhausted jobs: %w", err)
	}
	failed, _ := res.RowsAffected()
	return requeued, int(failed), nil
}

func (s *jobStore) RevertSweepRequeue(ctx context.Context, id string, staleThreshold time.Duration) error {
	// Age updated_at comfortably past the sweep cutoff (NOW() - staleThreshold) so
	// the next sweep's `updated_at < cutoff` predicate re-selects this row. Undo
	// the sweep's attempts+1 (GREATEST guards against underflow) so a failed
	// enqueue never eats retry budget. status='queued' guard: a no-op if a worker
	// already re-claimed the row (it then owns a lease and must not be disturbed).
	stale := time.Now().UTC().Add(-staleThreshold - time.Minute)
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = 'processing', attempts = GREATEST(attempts - 1, 0), updated_at = $2
		 WHERE id = $1 AND status = 'queued'`,
		id, stale)
	if err != nil {
		return fmt.Errorf("revert sweep requeue: %w", err)
	}
	return nil
}

func (s *jobStore) UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = $1, error = $2, updated_at = NOW()
		 WHERE id = $3`,
		string(status), errMsg, id)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	return nil
}

// SetPushGovernance records the push gate's refusals on the job row.
func (s *jobStore) SetPushGovernance(ctx context.Context, jobID, report string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs SET governance_report = $1, updated_at = NOW() WHERE id = $2`,
		report, jobID)
	if err != nil {
		return fmt.Errorf("record push governance: %w", err)
	}
	return nil
}

// PushGovernance reads back what a push's review gate refused. The report is
// written by the ingest job, the first job a push creates, so the read names
// that row rather than whichever of the push's jobs sorts first.
func (s *jobStore) PushGovernance(ctx context.Context, pushID string) (string, error) {
	var report string
	err := s.db.QueryRowContext(ctx,
		`SELECT governance_report FROM translation_jobs
		 WHERE push_id = $1 AND item_name = $2 AND governance_report <> ''
		 LIMIT 1`, pushID, SyncPushItemName).Scan(&report)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read push governance: %w", err)
	}
	return report, nil
}

func (s *jobStore) CompleteJob(ctx context.Context, id string, epoch int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = 'completed', error = '', updated_at = NOW()
		 WHERE id = $1 AND status = 'processing' AND claim_epoch = $2`, id, epoch)
	if err != nil {
		return false, fmt.Errorf("complete job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *jobStore) CancelJob(ctx context.Context, id, reason string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = 'failed', error = $1, claim_epoch = claim_epoch + 1, updated_at = NOW()
		 WHERE id = $2 AND status IN ('queued', 'processing')`,
		reason, id)
	if err != nil {
		return false, fmt.Errorf("cancel job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *jobStore) FailJob(ctx context.Context, id string, epoch int64, errMsg string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE translation_jobs
		 SET status = 'failed', error = $1, updated_at = NOW()
		 WHERE id = $2 AND status = 'processing' AND claim_epoch = $3`,
		errMsg, id, epoch)
	if err != nil {
		return false, fmt.Errorf("fail job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *jobStore) DeleteJob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM translation_jobs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete job: %w", err)
	}
	return nil
}

func (s *jobStore) ListJobsByPushID(ctx context.Context, pushID string) ([]*TranslationJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_slug, project_id, item_name, target_locale,
				provider_config_id, model, push_id, step_id, status, progress, total_blocks, done_blocks,
				tokens_used, error, created_at, updated_at, workspace_id, via_tm, via_ai, stream, created_by
		 FROM translation_jobs
		 WHERE push_id = $1
		 ORDER BY created_at ASC`, pushID)
	if err != nil {
		return nil, fmt.Errorf("list jobs by push_id: %w", err)
	}
	defer rows.Close()
	return scanJobs(rows)
}

// scanJob scans a single TranslationJob from a sql.Row.
func scanJob(row *sql.Row) (*TranslationJob, error) {
	var j TranslationJob
	var status string
	err := row.Scan(
		&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.TargetLocale,
		&j.ProviderConfigID, &j.Model, &j.PushID, &j.StepID, &status, &j.Progress, &j.TotalBlocks, &j.DoneBlocks,
		&j.TokensUsed, &j.Error, &j.CreatedAt, &j.UpdatedAt, &j.WorkspaceID, &j.ViaMemory, &j.ViaAI, &j.Stream, &j.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("scan job: %w", err)
	}
	j.Status = JobStatus(status)
	return &j, nil
}

// scanJobs scans multiple TranslationJob rows.
func scanJobs(rows *sql.Rows) ([]*TranslationJob, error) {
	var result []*TranslationJob
	for rows.Next() {
		var j TranslationJob
		var status string
		err := rows.Scan(
			&j.ID, &j.WorkspaceSlug, &j.ProjectID, &j.ItemName, &j.TargetLocale,
			&j.ProviderConfigID, &j.Model, &j.PushID, &j.StepID, &status, &j.Progress, &j.TotalBlocks, &j.DoneBlocks,
			&j.TokensUsed, &j.Error, &j.CreatedAt, &j.UpdatedAt, &j.WorkspaceID, &j.ViaMemory, &j.ViaAI, &j.Stream, &j.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("scan job row: %w", err)
		}
		j.Status = JobStatus(status)
		result = append(result, &j)
	}
	return result, rows.Err()
}
