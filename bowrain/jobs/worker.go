package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"golang.org/x/time/rate"
)

// WorkerConfig holds configuration for the translation worker.
type WorkerConfig struct {
	DatabaseURL         string
	ServiceBusConn      string
	QueueName           string
	CredentialStorePath string
}

// WorkerDeps holds all dependencies for the translation worker.
type WorkerDeps struct {
	JobStore     JobStore
	ContentStore store.ContentStore
	CredStore    *credentials.Store
	// ProviderStore resolves per-workspace BYO AI provider configs from Postgres
	// (Epic 004). It replaces the machine-global keychain/file CredStore for
	// worker translation jobs that carry a saved ProviderConfigID. Optional; when
	// nil, a job that names a saved config cannot be resolved.
	ProviderStore ProviderConfigResolver
	Queue         Queue
	QuotaStore    QuotaStore              // optional; nil disables quota enforcement
	Platform      *PlatformProviderConfig // optional; nil disables platform provider
	BillingHooks  *billing.UsageHooks     // optional; nil disables billing credit deduction
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
	// MaxJobAttempts bounds transient-failure retries before a job is failed.
	// Zero uses defaultMaxJobAttempts.
	MaxJobAttempts int
}

// maxJobAttempts returns the configured retry budget or the default.
func (d *WorkerDeps) maxJobAttempts() int {
	if d.MaxJobAttempts > 0 {
		return d.MaxJobAttempts
	}
	return defaultMaxJobAttempts
}

// jobHeartbeatInterval is how often a running translation refreshes its job's
// updated_at while it still holds the lease. It sits comfortably below the
// stale-job sweeper threshold (defaultStaleJobThreshold, 15m) so a slow-but-live
// job — e.g. one chunk stuck behind a rate-limited model — is never mistaken for
// a crashed worker and swept out from under itself.
const jobHeartbeatInterval = 2 * time.Minute

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
	if deps.Platform != nil {
		slog.InfoContext(ctx, "platform Azure OpenAI enabled", "endpoint", deps.Platform.Endpoint)
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

		processErr := processJobWithDeps(ctx, deps, jobID)
		if processErr != nil {
			var te *transientError
			if errors.As(processErr, &te) {
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
				if eqErr := deps.Queue.Enqueue(ctx, jobID); eqErr != nil {
					slog.WarnContext(ctx, "job transient failure; re-enqueue failed, nacking instead",
						"job_id", jobID, "error", processErr, "enqueue_error", eqErr)
					nack()
				} else {
					slog.WarnContext(ctx, "job transient failure; re-enqueued fresh message for retry",
						"job_id", jobID, "error", processErr)
					ack()
				}
				continue
			}
			slog.ErrorContext(ctx, "job failed", "job_id", jobID, "error", processErr)
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

	// Mark as completed.
	if err := deps.JobStore.UpdateJobStatus(ctx, jobID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Completed %s for %s — %d blocks, %d tokens",
			job.ItemName, job.TargetLocale, job.DoneBlocks, job.TokensUsed),
		map[string]string{"item": job.ItemName, "locale": job.TargetLocale,
			"blocks": strconv.Itoa(job.DoneBlocks), "tokens": strconv.Itoa(job.TokensUsed)})

	return nil
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

	storedBlocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: job.ProjectID,
		Stream:    "main",
		ItemName:  job.ItemName,
	})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	totalBlocks := len(storedBlocks)
	if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, 0, totalBlocks); err != nil {
		return fmt.Errorf("set total blocks: %w", err)
	}

	// Resolve AI provider. resolved.Source (platform vs byo) is the hybrid-AI
	// billing gate consumed at the credit-deduction site below.
	resolved, err := resolveProvider(ctx, deps, job)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}
	prov := resolved.LLM
	limiter := resolved.Limiter

	// Default batch/concurrency for automation jobs if not explicitly set.
	batchSz := 20
	if job.BatchSize > 0 {
		batchSz = job.BatchSize
	}
	concurrency := 5
	if job.Concurrency > 0 {
		concurrency = job.Concurrency
	}

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale:     proj.DefaultSourceLanguage,
		TargetLocale:     model.LocaleID(job.TargetLocale),
		BatchSize:        batchSz,
		BatchConcurrency: concurrency,
	})

	// Process blocks in progress-reporting chunks. The tool handles
	// internal batching + concurrency; we chunk for progress updates.
	const progressChunk = 50
	var allOutParts []*model.Part
	totalTokensUsed := 0
	prevUsage := translateTool.TotalUsage()

	for i := 0; i < totalBlocks; i += progressChunk {
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
		allOutParts = append(allOutParts, outParts...)

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
		if deps.QuotaStore != nil {
			_ = deps.QuotaStore.RecordUsage(ctx, AIUsageRecord{
				WorkspaceSlug: job.WorkspaceSlug,
				WorkspaceID:   job.WorkspaceID,
				ProjectID:     job.ProjectID,
				JobID:         job.ID,
				Model:         job.Model,
				Operation:     "translate",
				PromptTokens:  chunkInput,
				OutputTokens:  chunkOutput,
				TotalTokens:   chunkTokens,
			})
		}

		// Deduct billing credits and report to Stripe Meters — but only for the
		// platform-held key (resolved.Source, the shared hybrid-AI gate from
		// resolveProvider). A workspace bring-your-own key still recorded usage
		// above (the ai_usage abuse cap) but burns NO credits (Epic 004). The
		// per-chunk reference id ("<jobID>:<chunkOffset>") is deterministic and
		// unique per deduction, so the Stripe meter idempotency key neither
		// collides across chunks nor double-reports on a retried chunk.
		if deps.BillingHooks != nil && job.WorkspaceID != "" && resolved.Source == ProviderSourcePlatform {
			deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, chunkTokens, "ai_translation",
				fmt.Sprintf("%s:%d", job.ID, i))
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

	// Final ownership check before persisting: if the lease was lost after the
	// last chunk gate, do not write the translations — the fresh owner will, and
	// overwriting its output (or marking the job completed) would corrupt the
	// resurrected run.
	owner, lerr := deps.JobStore.RenewLease(ctx, job.ID, epoch)
	if lerr != nil {
		return fmt.Errorf("renew lease: %w", lerr)
	}
	if !owner {
		return errLeaseLost
	}

	// Store translated blocks. Targets land in the `translations`
	// overlay table via StoreBlocks (#405) — no separate overlay
	// write is needed: `ContentStore.StoreBlocks` now extracts
	// `block.Targets[locale]` and upserts to the translations table
	// directly. The former #404 dual-write against
	// blockstore.PutOverlay is retired along with `blocks.targets_json`.
	blocks := partsToBlocks(allOutParts)
	if len(blocks) > 0 {
		if err := deps.ContentStore.StoreBlocks(ctx, job.ProjectID, "main", blocks); err != nil {
			return fmt.Errorf("store blocks: %w", err)
		}
	}

	return nil
}

// leaseRenewer is the lease-heartbeat surface shared by every leased job
// store (translation, brand scan).
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
		if deps.Platform == nil {
			return nil, errors.New("platform provider not configured " +
				"(set BOWRAIN_PLATFORM_PROVIDER + key for self-hosted/local, " +
				"or BOWRAIN_OPENAI_ENDPOINT for Azure OpenAI)")
		}
		prov, ptype, err := deps.Platform.Build(job.Model)
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
	prov, err := aiprovider.NewProvider(aiprovider.ProviderID(cfg.Type), aiprovider.Config{
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
func estimateTokens(blocks []*store.StoredBlock) int {
	totalChars := 0
	for _, sb := range blocks {
		if sb.Block != nil && len(sb.Block.Source) > 0 {
			totalChars += len(sb.Block.SourceText()) * 2 // source + target estimate
		}
	}
	return totalChars / 4
}

// storedBlocksToParts converts stored blocks to Part slice (same as editor.go).
func storedBlocksToParts(storedBlocks []*store.StoredBlock) []*model.Part {
	parts := make([]*model.Part, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		parts = append(parts, &model.Part{
			Type:     model.PartBlock,
			Resource: sb.Block,
		})
	}
	return parts
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
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	errCh := make(chan error, 1)
	go func() {
		err := t.Process(ctx, in, out)
		close(out)
		errCh <- err
	}()

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
	}
	if err := <-errCh; err != nil {
		return nil, err
	}
	return result, nil
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
