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
}

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

		jobID, ack, _, err := deps.Queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "dequeue error", "error", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		if processErr := processJobWithDeps(ctx, deps, jobID); processErr != nil {
			slog.ErrorContext(ctx, "job failed", "job_id", jobID, "error", processErr)
		}
		// Always ack: processJob marks failed jobs in the database.
		// Nacking would cause infinite retries for permanent errors.
		ack()
	}
}

func processJobWithDeps(ctx context.Context, deps *WorkerDeps, jobID string) error {
	// Atomically claim the job (queued → processing). If another worker
	// already claimed it, skip without error. This prevents double-processing
	// when multiple workers dequeue the same job ID.
	claimed, err := deps.JobStore.ClaimJob(ctx, jobID)
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
			_ = deps.JobStore.UpdateJobStatus(ctx, jobID, StatusFailed, "workspace AI quota exceeded")
			return fmt.Errorf("workspace %s quota exceeded", job.WorkspaceSlug)
		}
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Translating %s for %s", job.ItemName, job.TargetLocale),
		map[string]string{"item": job.ItemName, "locale": job.TargetLocale, "model": job.Model})

	// Run the translation; on failure, mark as failed.
	if err := executeTranslationWithDeps(ctx, deps, job); err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, jobID, StatusFailed, err.Error())
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

func executeTranslationWithDeps(ctx context.Context, deps *WorkerDeps, job *TranslationJob) error {
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
	if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, 0, totalBlocks); err != nil {
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
		if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, end, totalBlocks); err != nil {
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
		prov, ptype, err := deps.Platform.build(job.Model)
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
	claimed, err := deps.JobStore.ClaimJob(ctx, jobID)
	if err != nil || !claimed {
		return err
	}
	job, err := deps.JobStore.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	return processSyncPushJob(ctx, deps, job)
}
