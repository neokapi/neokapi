package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/resilience/aiguard"
	diffcache "github.com/neokapi/neokapi/bowrain/sync"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"golang.org/x/time/rate"
)

// PushTransitionFunc runs fn against one transaction, handing it each store
// bound to that transaction. An error from fn rolls the whole thing back.
type PushTransitionFunc func(ctx context.Context, fn func(store.PushApplier, coreprofile.Store) error) error

// WorkerDeps holds all dependencies for the translation worker.
//
// QueueName is a bounded metric label for this worker's queue
// (translation|extraction|brand-scan). Empty defaults to "translation".
type WorkerDeps struct {
	QueueName    string
	JobStore     JobStore
	ContentStore store.ContentStore
	// PushTransition runs one push's writes on a single transaction spanning
	// every store the push touches — the content store AND the voice store the
	// context reconcile writes profiles to. Both are built from one PgDB, so
	// one transaction covers both.
	//
	// It exists because the reconcile does more than create empty collections:
	// it moves existing collections' coordinates and voice bindings, and it
	// creates and updates the workspace's voice profiles from what the push
	// declared. Run outside the transition, a push that failed afterwards had
	// already changed what governs the workspace on the strength of content
	// that never landed.
	//
	// nil means the stores are not known to share a pool — a test double, or a
	// deployment that cannot — and the push falls back to the content store's
	// own transaction with the reconcile in front of it, which is what it did
	// before this existed.
	PushTransition PushTransitionFunc
	CredStore      *credentials.Store
	// ProviderStore resolves per-workspace BYO AI provider configs from Postgres
	// (Epic 004). It replaces the machine-global keychain/file CredStore for
	// worker translation jobs that carry a saved ProviderConfigID. Optional; when
	// nil, a job that names a saved config cannot be resolved.
	ProviderStore ProviderConfigResolver
	Queue         Queue
	QuotaStore    QuotaStore              // optional; nil disables quota enforcement
	Platform      *PlatformProviderConfig // optional; nil disables platform provider
	// PlatformResolver, when set, is consulted at job time for the current
	// platform provider config so a runtime change (admin switching provider/model
	// in ctrl) takes effect without restarting the worker. Overrides Platform when
	// it returns non-nil.
	PlatformResolver PlatformResolver
	BillingHooks     *billing.UsageHooks // optional; nil disables billing credit deduction
	// LogFunc is called to emit structured automation logs (Bowrain AD-013).
	// Signature: func(stepID, level, message string, data map[string]string).
	// Optional; nil disables run logging.
	LogFunc func(stepID, level, message string, data map[string]string)
	// BlobStore provides access to push payloads for sync processing.
	BlobStore corestorage.BlobStore
	// Decompressor for zstd-compressed sync chunks (Bowrain AD-009). Optional.
	Decompressor interface {
		Decompress(data []byte) ([]byte, error)
	}
	// EventBus publishes events after sync push processing.
	EventBus platev.EventBus
	// SyncCache is the server's diff-negotiation hash cache (Bowrain AD-009).
	// The worker is what changes stored source content, so the worker is what
	// must invalidate: with nobody invalidating, a second push inside the TTL
	// was diffed against pre-apply hashes and its changed blocks were silently
	// skipped. Optional; nil when the deployment runs without Redis.
	SyncCache diffcache.HashCache
	// Tracker captures product analytics events (content_pushed). Optional;
	// nil disables capture. Fire-and-forget — never blocks job processing.
	Tracker EventTracker
	// MaxJobAttempts bounds transient-failure retries before a job is failed.
	// Zero uses defaultMaxJobAttempts.
	MaxJobAttempts int
	// MaxJobDeferrals bounds how many times a job may be parked on an
	// unavailable dependency before it is failed. Zero uses
	// defaultMaxJobDeferrals.
	MaxJobDeferrals int
	// DrainGrace bounds how long a job already running when shutdown is
	// signalled may keep running. Zero uses defaultDrainGrace. It must stay
	// under the orchestrator's stop timeout, or the task is killed mid-job and
	// the grace buys nothing.
	DrainGrace time.Duration
	// MemoryResolver returns the project's server content memory for a workspace, so a
	// convergence translation job can recycle exact/near-exact matches before
	// paying for AI (content memory-first convergence). Optional; when nil the job falls
	// back to the previous AI-only behavior. Mirrors the server's per-workspace
	// workspaceStores.getMemory.
	MemoryResolver MemoryResolver
	// VoiceStore reads voice profiles so a translation job carries the
	// project's voice into the AI prompt — parity with the CLI flow's
	// brand binding. Optional; nil translates without voice.
	VoiceStore coreprofile.Store
	// WorkspaceDefault resolves the workspace-level default voice
	// profile — the base rung of the voicescope resolution ladder that a
	// project/stream/collection binding overrides. Optional; nil skips the
	// workspace rung.
	WorkspaceDefault voicescope.WorkspaceDefault
	// TermsResolver returns the workspace terms so a translation job carries
	// the project's terminology as prompt term rules — parity with the CLI
	// flow's terms binding. Optional; nil translates without terminology.
	TermsResolver TermsResolver
	// ConnectorFetcher performs the fetch+store for durable forge-ingest jobs
	// (webhook/bind-triggered source ingests enqueued by the server). The
	// worker wires a store-backed fetcher that instantiates connectors on
	// demand from their persisted config. Nil fails ingest jobs permanently —
	// a worker without connector wiring must not silently eat them.
	ConnectorFetcher ConnectorFetcher
	// ConnectorConfigs reads persisted (decrypted) connector configs and
	// records ingest outcomes (last-sync / last-error) on the row. Required
	// alongside ConnectorFetcher for forge-ingest jobs.
	ConnectorConfigs ConnectorConfigSource
	// SweepStore persists model recommendation sweep measurements (measured
	// steerability). Required for model-sweep jobs; nil fails them permanently.
	SweepStore ModelSweepStore
	// SweepSettings exposes the model_sweeps.enabled gate and the candidate
	// model list from platform config. Required for model-sweep jobs.
	SweepSettings ModelSweepSettings
	// ModelRecommender, when set, lets platform model resolution prefer a
	// fresh measured recommendation for the job's project+locale — but only
	// when the workspace has not pinned a model (PlatformProviderConfig.
	// ModelPinned) and the recommender's own gates pass (flag on, fresh, model
	// still enabled). Optional; nil keeps the previous resolution exactly.
	ModelRecommender ModelRecommender
}

// EventTracker captures product analytics events (implemented by
// *analytics.PostHogClient).
type EventTracker interface {
	CaptureEvent(distinctID, event string, properties map[string]any)
}

// maxJobAttempts returns the configured retry budget or the default.
func (d *WorkerDeps) maxJobAttempts() int {
	if d.MaxJobAttempts > 0 {
		return d.MaxJobAttempts
	}
	return defaultMaxJobAttempts
}

// maxJobDeferrals returns how many times a job may be parked on an unavailable
// dependency before it is failed. Separate from the retry budget by design:
// waiting out an outage must not consume the attempts the job needs once the
// dependency is back.
func (d *WorkerDeps) maxJobDeferrals() int {
	if d.MaxJobDeferrals > 0 {
		return d.MaxJobDeferrals
	}
	return defaultMaxJobDeferrals
}

// drainGrace returns the configured shutdown grace or the default.
func (d *WorkerDeps) drainGrace() time.Duration {
	if d.DrainGrace > 0 {
		return d.DrainGrace
	}
	return defaultDrainGrace
}

// queueLabel is this worker's bounded metric label.
func (d *WorkerDeps) queueLabel() string {
	if d.QueueName == "" {
		return "translation"
	}
	return d.QueueName
}

// jobHeartbeatInterval is how often a running translation refreshes its job's
// updated_at while it still holds the lease. It sits comfortably below the
// stale-job sweeper threshold (defaultStaleJobThreshold, 15m) so a slow-but-live
// job — e.g. one chunk stuck behind a rate-limited model — is never mistaken for
// a crashed worker and swept out from under itself.
const jobHeartbeatInterval = 2 * time.Minute

// translationProgressChunk is how many blocks a translation bills, persists and
// reports progress for at a time. It is also the unit a retry resumes on.
const translationProgressChunk = 50

// defaultDrainGrace is how long a job in flight when SIGTERM arrives may keep
// running. It sits just under the orchestrator's stop timeout, so the job
// either finishes or is parked deliberately — rather than being killed with
// the task and left for the fifteen-minute stale sweeper on another instance.
const defaultDrainGrace = 25 * time.Second

// drainableJobContext detaches a job body from the shutdown signal, then bounds
// how long it may outlive it.
//
// Deriving the job's context from the signal context is what made every deploy
// freeze the work in flight: the provider call came back context.Canceled —
// deliberately permanent, so not retried — the failure write went to the same
// dead context, and the queue message was deleted anyway. The stop func
// releases the watchdog and cancels, and blocks until it has exited.
func drainableJobContext(parent context.Context, grace time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			return
		case <-parent.Done():
		}
		t := time.NewTimer(grace)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
			cancel()
		}
	}()
	return ctx, func() {
		cancel()
		<-done
	}
}

// errLeaseLost signals that the worker lost ownership of a job mid-run: the
// stale-job sweeper reset it to 'queued' and a fresh worker re-claimed it
// (bumping claim_epoch). The abandoned worker returns this to stop translating,
// billing, and persisting, so the fresh owner's work is not duplicated or
// overwritten. processJobWithDeps treats it as a clean hand-off (no failure).
var errLeaseLost = errors.New("job lease lost (resurrected by stale-job sweeper)")

// providerRateLimits maps provider types to their default rate limits (requests/sec).
var providerRateLimits = map[string]rate.Limit{
	"openai":      10,
	"azureopenai": 10,
	"anthropic":   5,
	"gemini":      5,
	"ollama":      100,  // effectively unlimited (local)
	"demo":        1000, // offline stub; no network
}

// RunWorkerWithDeps runs the translation worker loop with full dependency injection.
func RunWorkerWithDeps(ctx context.Context, deps *WorkerDeps) error {
	slog.InfoContext(ctx, "translation worker started")
	if p := activePlatform(ctx, "", deps.Platform, deps.PlatformResolver); p != nil {
		if p.Provider != "" {
			slog.InfoContext(ctx, "platform AI provider enabled", "provider", p.Provider, "model", p.Model)
		}
	}
	if deps.QuotaStore != nil {
		slog.InfoContext(ctx, "AI quota enforcement enabled")
	}
	defer slog.InfoContext(ctx, "translation worker stopped")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		jobID, ack, nack, err := deps.Queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "dequeue error", "error", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		// The loop's context gates Dequeue and nothing else. A job already in
		// flight runs on its own context, detached from the shutdown signal and
		// bounded by the drain grace, so SIGTERM lets it finish rather than
		// failing it with context.Canceled and leaving the row for the
		// fifteen-minute sweeper on another instance.
		//
		// Seed the job ID as the correlation ID so every log line emitted while
		// processing this job carries request_id=<jobID> — the same ID a user
		// sees for the job/run — and any Sentry capture is tagged with it.
		runCtx, stopDrain := drainableJobContext(ctx, deps.drainGrace())
		jobCtx := observe.WithRequestID(runCtx, jobID)
		// Bookkeeping outlives the shutdown signal too: the row, the broker
		// message and the summons must agree even as the process goes away.
		postCtx := observe.WithRequestID(context.WithoutCancel(ctx), jobID)

		queueLabel := deps.QueueName
		if queueLabel == "" {
			queueLabel = "translation"
		}
		jobStart := time.Now()
		observe.JobsInFlight.WithLabelValues(queueLabel).Inc()
		// One transaction per job. The worker carries the same traces sample
		// rate as the server and, until this, produced nothing: a convergence
		// run that took an hour was exactly as invisible as the 22-second
		// dashboard that prompted instrumenting the HTTP side (#2105, #2109).
		//
		// Named by the QUEUE, not the job: the id is already the correlation
		// tag Transaction reads off the context, and a per-job name would
		// aggregate nothing.
		traceCtx, endTrace := observe.Transaction(jobCtx, "queue.task", queueLabel)
		processErr := processJobWithDeps(traceCtx, deps, jobID)
		endTrace(processErr)
		observe.JobsInFlight.WithLabelValues(queueLabel).Dec()
		stopDrain()
		if processErr != nil {
			if de, ok := errors.AsType[*deferredError](processErr); ok {
				observe.JobsProcessedTotal.WithLabelValues(queueLabel, "deferred").Inc()
				// Parked on a known-down dependency. Re-enqueue with a delay
				// matched to the breaker's cooldown, so the redelivery arrives
				// when a probe is due rather than immediately — an outage must
				// not turn into a queue-spinning hot loop. Same fresh-message
				// rationale as the transient path: never trust nack() to
				// reproduce a delivery whose visibility may already have lapsed.
				if eqErr := enqueueAfter(postCtx, deps.Queue, jobID, de.retryAfter); eqErr != nil {
					slog.WarnContext(postCtx, "job deferred; re-enqueue failed, nacking instead",
						"job_id", jobID, "dependency", de.dependency, "enqueue_error", eqErr)
					nack()
				} else {
					slog.InfoContext(postCtx, "job deferred until dependency recovers",
						"job_id", jobID, "dependency", de.dependency,
						"retry_after", de.retryAfter.Round(time.Second).String())
					ack()
				}
				continue
			}
			if _, ok := errors.AsType[*transientError](processErr); ok {
				observe.JobsProcessedTotal.WithLabelValues(queueLabel, "transient").Inc()
				// Transient upstream failure with retry budget left:
				// processJobWithDeps has already reset the row to 'queued'.
				//
				// Do NOT rely on nack() to reproduce the delivery: if this job
				// ran longer than the broker AckWait, JetStream may have already
				// redelivered THIS message to a peer worker (which saw
				// 'processing', returned nil, and ACKed it), removing the broker
				// message. A nack() on our now-stale delivery is then a no-op and
				// the row is stranded in 'queued' with no message — and the
				// sweeper only recovers 'processing' rows. Instead publish a FRESH
				// message and ACK this one. Enqueue is idempotent: ClaimJob dedups
				// a stray concurrent redelivery. Fall back to nack() only if the
				// fresh enqueue fails, so the broker can still redeliver if it
				// happens to hold the message.
				if eqErr := deps.Queue.Enqueue(postCtx, jobID); eqErr != nil {
					slog.WarnContext(postCtx, "job transient failure; re-enqueue failed, nacking instead",
						"job_id", jobID, "error", processErr, "enqueue_error", eqErr)
					nack()
				} else {
					slog.WarnContext(postCtx, "job transient failure; re-enqueued fresh message for retry",
						"job_id", jobID, "error", processErr)
					ack()
				}
				continue
			}
			slog.ErrorContext(postCtx, "job failed", "job_id", jobID, "error", processErr)
			// Permanent failure (or exhausted retries): report to Sentry, tagged
			// with the job ID so it resolves from the client-visible reference.
			observe.CaptureError(processErr, jobID, map[string]string{"kind": "job", "job_id": jobID})
			observe.ObserveJob(queueLabel, "failed", jobStart)
			// …and tell the people waiting on it. This is the one branch that
			// has ruled out both retry and deferral, so it fires once per job
			// rather than once per attempt — the difference between a summons
			// and a burst of mail on a flaky provider.
			announceJobFailure(postCtx, deps, jobID, processErr)
		} else {
			observe.ObserveJob(queueLabel, "success", jobStart)
		}
		// Success, permanent failure, or exhausted retries: processJobWithDeps
		// has recorded the terminal status in the DB, so ACK to drop the
		// message. Nacking here would cause infinite redelivery of a job that
		// will never succeed.
		ack()
	}
}

func processJobWithDeps(ctx context.Context, deps *WorkerDeps, jobID string) error {
	// Atomically claim the job (queued → processing) and take the lease. If
	// another worker already claimed it, skip without error. This prevents
	// double-processing when multiple workers dequeue the same job ID. The
	// returned epoch is our lease token: it invalidates our writes if the job is
	// later swept and re-claimed by a fresh worker.
	claimed, epoch, err := deps.JobStore.ClaimJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("claim job: %w", err)
	}
	if !claimed {
		slog.DebugContext(ctx, "job already claimed, skipping", "job_id", jobID)
		return nil
	}

	job, err := deps.JobStore.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}

	// Route sync-push jobs to the dedicated handler.
	if job.ItemName == "__sync_push__" {
		return processSyncPushJob(ctx, deps, job)
	}

	// Route durable forge-ingest jobs (webhook/bind-triggered source ingests)
	// to the dedicated handler. They carry no AI work, so the quota check and
	// translation pipeline below do not apply.
	if job.IsForgeIngest() {
		return processForgeIngestJob(ctx, deps, job, epoch)
	}

	// Route model recommendation sweeps (measured steerability) to the
	// dedicated handler. Sweeps are platform QC on the platform key: their
	// usage is recorded (operation "model_sweep") but they bypass the
	// customer-facing quota gate below and never deduct credits.
	if job.IsModelSweep() {
		return processModelSweepJob(ctx, deps, job, epoch)
	}

	// Check the monthly token quota before starting. This is the internal abuse
	// cap (see jobs.QuotaStore) — a hard ceiling to bound runaway usage — not the
	// user-facing credit ledger (bowrain/billing). It applies to platform and BYO
	// jobs alike; credit deduction (below, per chunk) is platform-only.
	if deps.QuotaStore != nil {
		remaining, err := deps.QuotaStore.CheckQuota(ctx, job.WorkspaceSlug)
		if err != nil {
			slog.WarnContext(ctx, "quota check failed", "workspace", job.WorkspaceSlug, "error", err)
		} else if remaining <= 0 {
			_, _ = deps.JobStore.FailJob(ctx, jobID, epoch, "workspace AI quota exceeded")
			return fmt.Errorf("workspace %s quota exceeded", job.WorkspaceSlug)
		}
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Translating %s for %s", job.ItemName, job.TargetLocale),
		map[string]string{"item": job.ItemName, "locale": job.TargetLocale, "model": job.Model})

	// Run the translation. Classify failures: a transient upstream error
	// (provider 5xx/429/529, timeout, network) is retried via the broker; a
	// permanent error (auth/validation/config, missing content) is failed now.
	if err := executeTranslationWithDeps(ctx, deps, job, epoch); err != nil {
		if errors.Is(err, errLeaseLost) {
			// The sweeper resurrected this job and a fresh worker owns it now.
			// Abandon quietly: do NOT mark failed (that would clobber the new
			// owner) and do NOT retry (the fresh delivery drives it). The loop
			// ACKs this stale delivery.
			slog.InfoContext(ctx, "job lease lost mid-run; abandoning to fresh owner",
				"job_id", jobID)
			return nil
		}
		// The drain grace ran out before the job finished. The provider call
		// comes back context.Canceled, which is permanent everywhere else — but
		// here it says only that this process is going away, which is nothing
		// the job did. Park it exactly like a dependency outage: back to
		// 'queued' without spending an attempt, re-enqueued as a fresh message,
		// and resumed by the next worker at the chunk it reached.
		if ctx.Err() != nil {
			return deferInterruptedJob(ctx, deps, job, epoch)
		}
		// Dependency known-down: the breaker rejected the call, so nothing was
		// attempted upstream. Park the job instead of retrying it — waiting is
		// the honest cost of an outage, and spending the retry budget on calls
		// that were never made would fail work that is perfectly translatable
		// the moment the provider returns.
		if d := deferralFor(err); d != nil {
			deferred, derr := deps.JobStore.DeferJob(ctx, jobID, epoch, deps.maxJobDeferrals(),
				"deferred: "+d.dependency+" unavailable")
			if derr != nil {
				slog.WarnContext(ctx, "defer bookkeeping failed", "job_id", jobID, "error", derr)
			}
			if deferred {
				observe.JobsDeferredTotal.WithLabelValues(deps.queueLabel(), d.dependency).Inc()
				emitLog(deps, job.StepID, "warn",
					"Waiting for "+d.dependency+" to recover; this job is queued and will resume automatically.",
					map[string]string{"item": job.ItemName, "locale": job.TargetLocale, "dependency": d.dependency})
				return d
			}
			// Deferral budget exhausted — DeferJob marked the job failed.
			emitLog(deps, job.StepID, "error",
				d.dependency+" did not recover; giving up on this job.",
				map[string]string{"item": job.ItemName, "locale": job.TargetLocale, "dependency": d.dependency})
			return err
		}
		if isTransientError(err) {
			retry, rerr := deps.JobStore.RetryOrFail(ctx, jobID, epoch, deps.maxJobAttempts(), err.Error())
			if rerr != nil {
				slog.WarnContext(ctx, "retry bookkeeping failed", "job_id", jobID, "error", rerr)
			}
			if retry {
				emitLog(deps, job.StepID, "warn",
					"Transient error, retrying: "+err.Error(),
					map[string]string{"item": job.ItemName, "locale": job.TargetLocale})
				// Signal the loop to NAK; the row is back in 'queued'.
				return &transientError{err: err}
			}
			// Retry budget exhausted — RetryOrFail marked the job failed.
			emitLog(deps, job.StepID, "error",
				"Translation failed after retries: "+err.Error(),
				map[string]string{"item": job.ItemName, "locale": job.TargetLocale})
			return err
		}
		// Epoch-guarded terminal write: a stale worker's permanent error must
		// not overwrite a fresh owner's run.
		owner, ferr := deps.JobStore.FailJob(ctx, jobID, epoch, err.Error())
		if ferr != nil {
			slog.WarnContext(ctx, "failure bookkeeping failed", "job_id", jobID, "error", ferr)
		} else if !owner {
			slog.InfoContext(ctx, "job lease lost; leaving fresh owner's run untouched",
				"job_id", jobID)
			return nil
		}
		emitLog(deps, job.StepID, "error",
			"Translation failed: "+err.Error(),
			map[string]string{"item": job.ItemName, "locale": job.TargetLocale})
		return err
	}

	// Mark as completed — under the lease, like every other terminal write. A
	// worker whose lease was taken must not report success: the sweeper's fresh
	// owner is still running the job, and a cancellation is a decision this
	// write would quietly undo.
	owner, err := deps.JobStore.CompleteJob(ctx, jobID, epoch)
	if err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	if !owner {
		slog.InfoContext(ctx, "job lease lost before completion; leaving the row as it stands",
			"job_id", jobID)
		return nil
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Completed %s for %s: %d blocks, %d tokens",
			job.ItemName, job.TargetLocale, job.DoneBlocks, job.TokensUsed),
		map[string]string{"item": job.ItemName, "locale": job.TargetLocale,
			"blocks": strconv.Itoa(job.DoneBlocks), "tokens": strconv.Itoa(job.TokensUsed)})

	return nil
}

// deferInterruptedJob parks a job whose worker was told to stop mid-run. The
// bookkeeping runs on a context detached from the one that carried the
// cancellation, because that one can no longer write.
func deferInterruptedJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64) error {
	book := context.WithoutCancel(ctx)
	cause := errors.New("worker shutting down; job parked for the next worker")
	deferred, derr := deps.JobStore.DeferJob(book, job.ID, epoch, deps.maxJobDeferrals(),
		"deferred: "+cause.Error())
	if derr != nil {
		slog.WarnContext(book, "shutdown defer bookkeeping failed", "job_id", job.ID, "error", derr)
	}
	if !deferred {
		// Out of deferral budget — DeferJob marked the job failed.
		emitLog(deps, job.StepID, "error",
			"Repeatedly interrupted by worker restarts; giving up on this job.",
			map[string]string{"item": job.ItemName, "locale": job.TargetLocale})
		return cause
	}
	observe.JobsDeferredTotal.WithLabelValues(deps.queueLabel(), "worker-shutdown").Inc()
	slog.InfoContext(book, "job parked by worker shutdown; re-enqueued for the next worker",
		"job_id", job.ID, "done_blocks", job.DoneBlocks, "total_blocks", job.TotalBlocks)
	emitLog(deps, job.StepID, "warn",
		"Paused by a worker restart; this job is queued and will resume automatically.",
		map[string]string{"item": job.ItemName, "locale": job.TargetLocale})
	return &deferredError{err: cause, dependency: "worker-shutdown"}
}

func emitLog(deps *WorkerDeps, stepID, level, message string, data map[string]string) {
	if deps.LogFunc != nil && stepID != "" {
		deps.LogFunc(stepID, level, message, data)
	}
}

func executeTranslationWithDeps(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64) error {
	// Lease heartbeat: while we translate, periodically refresh updated_at so a
	// slow-but-live job is never swept as stale (the primary defense against a
	// duplicate run). Runs until the function returns; a lost lease is caught
	// authoritatively by the per-chunk gate below (which stops before billing).
	stopHeartbeat := startLeaseHeartbeat(ctx, deps.JobStore, job.ID, epoch)
	defer stopHeartbeat()

	proj, err := deps.ContentStore.GetProject(ctx, job.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// The job's stream scopes both the read (target overlays hydrate from it)
	// and the writes below. Empty means "main", so stream-naive callers and
	// pre-stream rows keep their behavior.
	jobStream := job.Stream
	if jobStream == "" {
		jobStream = "main"
	}

	storedBlocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: job.ProjectID,
		Stream:    jobStream,
		ItemName:  job.ItemName,
	})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	// Source-first gate (epic 019): translate only the blocks whose source has
	// settled to (or above) the project's source gate. A block held below the
	// gate is skipped here — never translated into this locale — so a partially
	// settled item translates only its ready segments and an off-brand /
	// un-term-checked source is not fanned out. The orchestrator already refuses
	// to spawn a job for an item with nothing producible; this is the per-block
	// enforcement that also protects the direct auto-translate path.
	storedBlocks = gateBlocksBySource(storedBlocks, store.SourceGateFor(proj))

	srcLocale := proj.DefaultSourceLanguage
	tgtLocale := model.LocaleID(job.TargetLocale)

	// The stream's recorded bases, read once for the job. They decide which blocks
	// this locale still owes a draft (decisionLedger.needsDraft) and which units
	// this job may record a basis for once it has written one.
	//
	// The filter sits here rather than inside the recycle pass because it holds
	// whether or not the project has a content memory: a job with no corpus to
	// recycle from still pays only for the units that are genuinely owed work.
	ledger := loadDecisionLedger(ctx, deps.ContentStore, job.ProjectID, jobStream)
	unitByBlockID := make(map[string]*venue.StoredBlock, len(storedBlocks))
	owed := make([]*venue.StoredBlock, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		if !ledger.needsDraft(sb, tgtLocale) {
			continue
		}
		unitByBlockID[sb.Block.ID] = sb
		owed = append(owed, sb)
	}
	storedBlocks = owed

	totalBlocks := len(storedBlocks)
	if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, 0, totalBlocks); err != nil {
		return fmt.Errorf("set total blocks: %w", err)
	}

	// Lazily resolve the standing voice profile once for draft scoring
	// (persistDraftVoiceScores) — resolved only when a draft actually persists,
	// so a job with nothing to score costs no extra store reads.
	var voiceProfile *coreprofile.VoiceProfile
	voiceProfileResolved := false
	draftProfile := func() *coreprofile.VoiceProfile {
		if !voiceProfileResolved {
			voiceProfile = resolveJobVoiceProfile(ctx, deps, job)
			voiceProfileResolved = true
		}
		return voiceProfile
	}

	// content memory-first convergence (theme A). Before paying for AI, recycle exact/
	// near-exact matches from the project's server content memory — mirroring the built-in
	// `translate` flow's recycle→translate ordering. Only the blocks with no
	// usable content-memory match go to the AI translator below; the content memory-filled ones are
	// persisted straight away. memoryFilled feeds the truthful ViaMemory report.
	memoryFilled := 0
	tm := resolveJobMemory(deps, job)
	if tm != nil {
		res, rerr := recycleBlocks(ctx, tm, storedBlocks, srcLocale, tgtLocale, projectMemoryMinScore(proj), ledger)
		if rerr != nil {
			// A content memory failure must never block the paid translation path — fall
			// back to translating everything, exactly as before.
			slog.WarnContext(ctx, "content memory recycle failed; falling back to AI-only", "job_id", job.ID, "error", rerr)
		} else {
			memoryFilled = res.memoryCount
			if len(res.filled) > 0 {
				if err := deps.ContentStore.StoreBlocks(ctx, job.ProjectID, jobStream, res.filled); err != nil {
					return fmt.Errorf("store recycled blocks: %w", err)
				}
				recordProducedBasis(ctx, deps.ContentStore, job.ProjectID, jobStream, ledger, unitByBlockID, res.filled, tgtLocale)
				emitLog(deps, job.StepID, "info",
					fmt.Sprintf("Recycled %d block(s) from content memory (skipping AI)", memoryFilled),
					map[string]string{"via_tm": strconv.Itoa(memoryFilled)})
				// Score the recycled drafts against the standing voice profile
				// (deterministic vocabulary check, zero AI) so the compliant
				// rate covers content memory output too.
				persistDraftVoiceScores(ctx, deps, job, draftProfile(), res.filled, tgtLocale)
			}
			// Rebuild the stored-block slice for the AI loop as the remainder —
			// only genuinely-new segments cost credits. StoredBlock carries more
			// than the Block, so re-query the remainder by ID to keep the slice
			// shape (StoredBlock) the chunk loop expects.
			storedBlocks = filterStoredByRemainder(storedBlocks, res.remainder)
			totalBlocks = len(storedBlocks)
		}
	}

	// Record the content memory/AI split truthfully on the job so the convergence produce
	// emitter can report "content memory N · AI M" (theme A2). aiFilled is the remainder
	// the AI loop below translates.
	if err := deps.JobStore.UpdateJobMemorySplit(ctx, job.ID, epoch, memoryFilled, totalBlocks); err != nil {
		slog.WarnContext(ctx, "record content memory/AI split failed", "job_id", job.ID, "error", err)
	}

	// Nothing left for AI (everything recycled or already translated): done.
	if totalBlocks == 0 {
		return nil
	}

	// Where this attempt starts. Chunks persist as they complete, so an attempt
	// that died partway left finished translations behind and a done_blocks that
	// says how many — resuming there is what stops a retry from paying to
	// translate them again. Only a remainder of the same size can be resumed
	// into: the blocks are ordered by id and the recycle pass reshapes the
	// slice, so a different total means the offsets no longer name the same
	// blocks, and the attempt starts over rather than guess.
	resumeFrom := 0
	if job.DoneBlocks > 0 && job.DoneBlocks < totalBlocks && job.TotalBlocks == totalBlocks {
		resumeFrom = job.DoneBlocks - job.DoneBlocks%translationProgressChunk
	}
	if resumeFrom > 0 {
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Resuming at block %d of %d", resumeFrom+1, totalBlocks),
			map[string]string{"done": strconv.Itoa(resumeFrom), "total": strconv.Itoa(totalBlocks)})
	}

	// Reset the progress denominator to the AI remainder so a run that recycled
	// most of its blocks doesn't look stuck at N/original.
	if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, resumeFrom, totalBlocks); err != nil {
		return fmt.Errorf("reset total blocks: %w", err)
	}

	// Resolve AI provider. resolved.Source (platform vs byo) is the hybrid-AI
	// billing gate consumed at the credit-deduction site below.
	resolved, err := resolveProvider(ctx, deps, job)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}
	prov := resolved.LLM
	limiter := resolved.Limiter

	cfg := jobTranslateConfig(ctx, deps, job, proj)
	if cfg.Profile != nil || len(cfg.TermRules) > 0 {
		profileName := ""
		if cfg.Profile != nil {
			profileName = cfg.Profile.Name
		}
		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Applying context: voice=%q, term rules=%d", profileName, len(cfg.TermRules)),
			map[string]string{"voice": profileName, "term_rules": strconv.Itoa(len(cfg.TermRules))})
	}
	translateTool := tools.NewAITranslateTool(prov, cfg)

	// Process blocks in progress-reporting chunks. The tool handles
	// internal batching + concurrency; we chunk for progress updates.
	const progressChunk = translationProgressChunk
	totalTokensUsed := 0
	prevUsage := translateTool.TotalUsage()

	for i := resumeFrom; i < totalBlocks; i += progressChunk {
		end := min(i+progressChunk, totalBlocks)
		chunk := storedBlocks[i:end]

		// Rate limit.
		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}

		parts := storedBlocksToParts(chunk)
		outParts, err := runToolOnParts(ctx, translateTool, parts)
		if err != nil {
			return fmt.Errorf("translate chunk %d-%d: %w", i, end, err)
		}

		// Lease gate: only bill + persist this chunk while we still own the job.
		// If a (possibly slow) chunk let the sweeper resurrect the job and a
		// fresh worker re-claimed it (claim_epoch advanced), RenewLease reports
		// !owner — abandon before RecordUsage/DeductTokens so the fresh owner is
		// not double-charged. RenewLease also refreshes updated_at, so a live job
		// making steady chunk progress additionally never looks stale.
		owner, lerr := deps.JobStore.RenewLease(ctx, job.ID, epoch)
		if lerr != nil {
			return fmt.Errorf("renew lease: %w", lerr)
		}
		if !owner {
			return errLeaseLost
		}

		// Read actual token usage from the provider (via tool accumulator).
		// Fall back to estimate if the provider returned zero usage.
		currentUsage := translateTool.TotalUsage()
		chunkTokens := currentUsage.TotalTokens() - prevUsage.TotalTokens()
		chunkInput := currentUsage.InputTokens - prevUsage.InputTokens
		chunkOutput := currentUsage.OutputTokens - prevUsage.OutputTokens
		prevUsage = currentUsage
		if chunkTokens <= 0 {
			chunkTokens = estimateTokens(chunk)
		}
		totalTokensUsed += chunkTokens

		// Record usage per chunk so quota is updated incrementally.
		//
		// Fail-open by policy: the tokens are already spent by the time this
		// runs, so a meter that is down must not also cost the customer the
		// translation. Fail-open is not fail-silent, though — the discarded
		// record is the only trace this spend ever leaves, so it is logged and
		// counted rather than dropped.
		if deps.QuotaStore != nil {
			if err := deps.QuotaStore.RecordUsage(ctx, AIUsageRecord{
				WorkspaceSlug: job.WorkspaceSlug,
				WorkspaceID:   job.WorkspaceID,
				ProjectID:     job.ProjectID,
				JobID:         job.ID,
				Model:         job.Model,
				Operation:     "translate",
				PromptTokens:  chunkInput,
				OutputTokens:  chunkOutput,
				TotalTokens:   chunkTokens,
			}); err != nil {
				observe.MeteringDiscarded(ctx, observe.MeterAITokens, float64(chunkTokens), err,
					"operation", "translate", "job_id", job.ID,
					"workspace", job.WorkspaceSlug, "model", job.Model)
			}
		}

		// Deduct billing credits and report to Stripe Meters — but only for the
		// platform-held key (resolved.Source, the shared hybrid-AI gate from
		// resolveProvider). A workspace bring-your-own key still recorded usage
		// above (the ai_usage abuse cap) but burns NO credits (Epic 004).
		//
		// The reference id names the WORK, not its position: "<jobID>:<first
		// block id of the chunk>". A retried attempt covers whatever the job
		// still owes, so the chunk at offset 0 the second time is a different
		// set of blocks from the chunk at offset 0 the first time; keying on
		// the offset made the Stripe meter's idempotency suppress a real
		// deduction and the job cost less than the work it did. Keyed on the
		// block, a chunk that is genuinely redone still dedupes and one that is
		// not still bills.
		if deps.BillingHooks != nil && job.WorkspaceID != "" && resolved.Source == ProviderSourcePlatform {
			deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, chunkTokens, "ai_translation",
				chunkBillingRef(job.ID, chunk, i))
		}

		// Store this chunk's translations before its progress is recorded, so
		// done_blocks never claims more than the overlay table holds and a
		// resumed attempt cannot skip a block it never wrote. Targets land in
		// the `translations` overlay table via StoreBlocks — no separate overlay
		// write is needed: `ContentStore.StoreBlocks` extracts
		// `block.Targets[locale]` and upserts to the translations table
		// directly.
		blocks := partsToBlocks(outParts)
		if len(blocks) > 0 {
			if err := deps.ContentStore.StoreBlocks(ctx, job.ProjectID, jobStream, blocks); err != nil {
				return fmt.Errorf("store blocks: %w", err)
			}
			// The source each draft was translated from, so the next pass can
			// tell a translation of the current wording from one left over from
			// wording that has since been rewritten.
			recordProducedBasis(ctx, deps.ContentStore, job.ProjectID, jobStream, ledger, unitByBlockID, blocks, tgtLocale)
			// Score the AI drafts against the standing voice profile (deterministic
			// vocabulary check, zero AI) so the dashboard's compliance rate is
			// voice-informed for every drafted block.
			persistDraftVoiceScores(ctx, deps, job, draftProfile(), blocks, tgtLocale)
			// AI drafts do NOT enter the content memory. The corpus has one door
			// in — wording a decision approved (PromoteDecisionsToMemory) — so a
			// guess can never be offered back as approved wording. Draft reuse
			// within a run is the block store's job: identical source means an
			// identical content hash, exact-match only, labeled for what it is.
		}

		// Update progress.
		if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, end, totalBlocks); err != nil {
			slog.WarnContext(ctx, "update progress failed", "job_id", job.ID, "error", err)
		}
		job.DoneBlocks = end
		job.TotalBlocks = totalBlocks

		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Translated blocks %d-%d of %d (%d tokens)", i+1, end, totalBlocks, chunkTokens),
			map[string]string{"done": strconv.Itoa(end), "total": strconv.Itoa(totalBlocks)})
	}

	// Update total token usage on the job.
	job.TokensUsed = totalTokensUsed

	return nil
}

// leaseRenewer is the lease-heartbeat surface shared by every leased job
// store (translation, context scan).
type leaseRenewer interface {
	RenewLease(ctx context.Context, id string, epoch int64) (owner bool, err error)
}

// startLeaseHeartbeat launches a goroutine that refreshes the job's updated_at
// on jobHeartbeatInterval while the caller holds the lease (epoch). It keeps a
// slow-but-live job from being swept as stale. The returned stop func blocks
// until the goroutine has fully exited, so the worker's goleak check stays
// green. Ownership loss is not acted on here (it only stops the heartbeat and
// logs) — the per-chunk RenewLease gate is the authoritative place that halts
// billing/persistence, so cancellation semantics stay simple.
func startLeaseHeartbeat(ctx context.Context, store leaseRenewer, jobID string, epoch int64) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(jobHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				owner, err := store.RenewLease(ctx, jobID, epoch)
				if err != nil {
					slog.WarnContext(ctx, "job lease heartbeat failed", "job_id", jobID, "error", err)
					continue
				}
				if !owner {
					// Lost the lease; stop refreshing. The per-chunk gate will
					// return errLeaseLost at the next boundary.
					slog.WarnContext(ctx, "job lease lost (heartbeat); will abandon at next chunk boundary",
						"job_id", jobID)
					return
				}
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

// resolveProvider creates the appropriate LLM provider for the job and reports
// its billing source (Epic 004 hybrid AI). A job with an empty or "platform"
// ProviderConfigID uses the env-configured platform provider (metered in
// credits). A job that names a saved config resolves it from the per-workspace
// Postgres store — scoped to the job's workspace — and its BYO key burns no
// credits. This deliberately no longer touches the machine-global keychain/file
// CredStore for saved provider keys.
func resolveProvider(ctx context.Context, deps *WorkerDeps, job *TranslationJob) (*ResolvedProvider, error) {
	if job.IsPlatformProvider() {
		platform := activePlatform(ctx, job.WorkspaceID, deps.Platform, deps.PlatformResolver)
		if platform == nil {
			return nil, errors.New("platform provider not configured " +
				"(set BOWRAIN_PLATFORM_PROVIDER + key for self-hosted/local, " +
				"or BOWRAIN_OPENAI_ENDPOINT for Azure OpenAI)")
		}
		// Measured steerability (EV-4): when the workspace has NOT pinned a
		// model (customer model choice), prefer a fresh, qualifying measured
		// recommendation for this job's project+locale over the platform
		// default. The recommender itself enforces the model_sweeps.enabled
		// flag, the freshness bound, the adherence floor, and that the model is
		// still ctrl-enabled — all disabled paths fall through to the previous
		// resolution unchanged.
		if deps.ModelRecommender != nil && !platform.ModelPinned {
			if rec, ok := deps.ModelRecommender.RecommendedModel(ctx, job.ProjectID, job.TargetLocale); ok && rec != "" {
				p := *platform
				p.Model = rec
				platform = &p
				slog.DebugContext(ctx, "platform model resolved from measured recommendation",
					"job_id", job.ID, "project", job.ProjectID, "locale", job.TargetLocale, "model", rec)
			}
		}
		prov, ptype, err := platform.Build(job.Model)
		if err != nil {
			return nil, err
		}
		return &ResolvedProvider{
			LLM:     prov,
			Limiter: rate.NewLimiter(providerRateLimit(ptype), 1),
			Source:  ProviderSourcePlatform,
		}, nil
	}

	// Per-workspace BYO provider: resolve the saved config (with its sealed key
	// decrypted) from Postgres, scoped strictly to this job's durable billing
	// workspace id (now persisted on the job). Slug is deliberately not used — it
	// is mutable and reusable, so it must never authorize secret retrieval.
	if deps.ProviderStore == nil {
		return nil, errors.New("per-workspace provider store not configured; " +
			"cannot resolve saved provider config " + job.ProviderConfigID)
	}
	cfg, err := deps.ProviderStore.Resolve(ctx, job.WorkspaceID, job.ProviderConfigID)
	if err != nil {
		return nil, fmt.Errorf("resolve provider config %q: %w", job.ProviderConfigID, err)
	}

	model := cfg.Model
	if model == "" {
		model = job.Model
	}
	prov, err := aiguard.NewProvider(aiprovider.ProviderID(cfg.Type), aiprovider.Config{
		APIKey:  cfg.APIKey,
		Model:   model,
		BaseURL: cfg.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("build provider %q: %w", cfg.Type, err)
	}
	return &ResolvedProvider{
		LLM:     prov,
		Limiter: rate.NewLimiter(providerRateLimit(cfg.Type), 1),
		Source:  ProviderSourceBYO,
	}, nil
}

// estimateTokens provides a rough token count estimate for a batch of blocks.
// Uses ~4 characters per token as a heuristic (covers source + target).
func estimateTokens(blocks []*venue.StoredBlock) int {
	totalChars := 0
	for _, sb := range blocks {
		if sb.Block != nil && len(sb.Block.Source) > 0 {
			totalChars += len(sb.Block.SourceText()) * 2 // source + target estimate
		}
	}
	return totalChars / 4
}

// jobTranslateConfig builds the AI translate tool config for a translation job:
// the locale/batching plumbing this queue decides, and the governing context
// the shared assembly binds (BuildTranslateConfig), so a queued translation and
// an interactive one carry the same voice, terminology, protected terms,
// content memory, neighbourhood and point.
func jobTranslateConfig(ctx context.Context, deps *WorkerDeps, job *TranslationJob, proj *store.Project) tools.AITranslateConfig {
	// Default batch/concurrency for automation jobs if not explicitly set.
	batchSz := 20
	if job.BatchSize > 0 {
		batchSz = job.BatchSize
	}
	concurrency := 5
	if job.Concurrency > 0 {
		concurrency = job.Concurrency
	}

	b := jobTranslateBinding(deps, job, proj)
	b.BatchSize = batchSz
	b.BatchConcurrency = concurrency
	return BuildTranslateConfig(ctx, b)
}

// jobTranslateBinding gathers what a job's translation is governed by: the
// stores the worker was wired with, the project it runs over, and the item
// whose collection places the content.
func jobTranslateBinding(deps *WorkerDeps, job *TranslationJob, proj *store.Project) TranslateBinding {
	b := TranslateBinding{
		Project:      proj,
		WorkspaceID:  job.WorkspaceID,
		ProjectID:    job.ProjectID,
		Stream:       jobStreamName(job),
		ItemName:     job.ItemName,
		TargetLocale: model.LocaleID(job.TargetLocale),
	}
	if deps == nil {
		return b
	}
	b.Store = deps.ContentStore
	b.Voice = deps.VoiceStore
	b.WorkspaceDefault = deps.WorkspaceDefault
	b.Terms = resolveJobTerms(deps, job)
	b.Memory = resolveJobMemory(deps, job)
	return b
}

// jobStreamName is the stream a job reads and writes. Empty means "main", so an
// old row and a stream-naive caller keep their behavior.
func jobStreamName(job *TranslationJob) string {
	if job == nil || job.Stream == "" {
		return "main"
	}
	return job.Stream
}

// ProjectDNTTerms reads the project's do-not-translate terms from settings
// (comma-separated in Properties["dnt_terms"]) — product names, trademarks, and
// code identifiers that must survive verbatim into every target. The AI
// translate tool masks and restores these spans so they cannot be translated.
// Empty when unset.
//
// It is the single derivation shared by every server-side translation
// surface — the worker's jobs (jobTranslateConfig) and the synchronous editor
// translate in bowrain/server — so both protect identical terms.
func ProjectDNTTerms(proj *store.Project) []string {
	if proj == nil || proj.Properties == nil {
		return nil
	}
	raw := strings.TrimSpace(proj.Properties["dnt_terms"])
	if raw == "" {
		return nil
	}
	var terms []string
	for t := range strings.SplitSeq(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			terms = append(terms, t)
		}
	}
	return terms
}

// gateBlocksBySource keeps only the blocks whose source clears the source-first
// gate (epic 019): a block the settle phase explicitly held below the gate is
// dropped so it is never translated. It is the per-block refinement of the
// orchestrator's item-level gate, so a partially-settled item translates only
// its ready segments.
//
// A block that was never settled (SourceStatusNew — no committed status) is
// PASSED THROUGH: the worker is not the settle phase, and holding an unstamped
// block here would silently strand every non-source-first path (auto-translate,
// projects that never ran a settle pass). The gate holds only what settlement
// committed below it — an authored/checked block a settle pass demoted or left
// short. A non-translatable block (no source to gate) and a disabled gate
// (SourceGateNone) always pass.
func gateBlocksBySource(blocks []*venue.StoredBlock, gate model.SourceGateLevel) []*venue.StoredBlock {
	if gate == model.SourceGateNone {
		return blocks
	}
	kept := blocks[:0:0]
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		status := sb.Block.SourceStatus
		if !sb.Block.Translatable || status == model.SourceStatusNew || gate.Admits(status) {
			kept = append(kept, sb)
		}
	}
	return kept
}

// storedBlocksToParts converts stored blocks to Part slice (same as editor.go).
func storedBlocksToParts(storedBlocks []*venue.StoredBlock) []*model.Part {
	parts := make([]*model.Part, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		parts = append(parts, &model.Part{
			Type:     model.PartBlock,
			Resource: sb.Block,
		})
	}
	return parts
}

// chunkBillingRef is the deduction's idempotency key: the job and the first
// block the chunk covers. offset is the fallback for a chunk whose first block
// carries no id, which no store-backed run produces.
func chunkBillingRef(jobID string, chunk []*venue.StoredBlock, offset int) string {
	if len(chunk) > 0 && chunk[0] != nil && chunk[0].Block != nil && chunk[0].Block.ID != "" {
		return jobID + ":" + chunk[0].Block.ID
	}
	return fmt.Sprintf("%s:%d", jobID, offset)
}

// partsToBlocks extracts model.Block objects from parts (same as editor.go).
func partsToBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		if block, ok := pt.Resource.(*model.Block); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// runToolOnParts executes a tool on parts using channels (same as editor.go).
//
// Process runs in its own goroutine while the caller drains the output channel
// concurrently, so a fan-out tool that emits more parts than it consumes cannot
// deadlock on a bounded buffer.
func runToolOnParts(ctx context.Context, t tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	return tool.RunOnParts(ctx, t, parts)
}

func providerRateLimit(providerType string) rate.Limit {
	if l, ok := providerRateLimits[providerType]; ok {
		return l
	}
	return 5 // conservative default
}

// sleepCtx sleeps for the duration or until ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// ProcessSyncPushJobForTest is an exported wrapper for testing the sync push worker.
func ProcessSyncPushJobForTest(ctx context.Context, deps *WorkerDeps, jobID string) error {
	claimed, _, err := deps.JobStore.ClaimJob(ctx, jobID)
	if err != nil || !claimed {
		return err
	}
	job, err := deps.JobStore.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	return processSyncPushJob(ctx, deps, job)
}
