package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gokapi/gokapi/bowrain/credentials"
	"github.com/gokapi/gokapi/providers/ai"
	"github.com/gokapi/gokapi/core/ai/tools"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/platform/store"
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
	Queue        Queue
	QuotaStore   QuotaStore             // optional; nil disables quota enforcement
	Platform     *PlatformProviderConfig // optional; nil disables platform provider
}

// providerRateLimits maps provider types to their default rate limits (requests/sec).
var providerRateLimits = map[string]rate.Limit{
	"openai":      10,
	"azureopenai": 10,
	"anthropic":   5,
	"ollama":      100, // effectively unlimited
}

// RunWorker runs the translation worker loop. It dequeues job IDs, loads the
// corresponding TranslationJob, processes blocks through the AI translate tool,
// and stores results back. It blocks until ctx is cancelled.
func RunWorker(ctx context.Context, jobStore JobStore, contentStore store.ContentStore, credStore *credentials.Store, queue Queue) error {
	return RunWorkerWithDeps(ctx, &WorkerDeps{
		JobStore:     jobStore,
		ContentStore: contentStore,
		CredStore:    credStore,
		Queue:        queue,
	})
}

// RunWorkerWithDeps runs the translation worker loop with full dependency injection.
func RunWorkerWithDeps(ctx context.Context, deps *WorkerDeps) error {
	log.Println("translation worker started")
	if deps.Platform != nil {
		log.Printf("platform Azure OpenAI enabled: %s", deps.Platform.Endpoint)
	}
	if deps.QuotaStore != nil {
		log.Println("AI quota enforcement enabled")
	}
	defer log.Println("translation worker stopped")

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
			log.Printf("dequeue error: %v", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		if processErr := processJobWithDeps(ctx, deps, jobID); processErr != nil {
			log.Printf("job %s failed: %v", jobID, processErr)
		}
		// Always ack: processJob marks failed jobs in the database.
		// Nacking would cause infinite retries for permanent errors.
		ack()
	}
}

func processJobWithDeps(ctx context.Context, deps *WorkerDeps, jobID string) error {
	job, err := deps.JobStore.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}

	// Skip jobs that aren't queued (already processing, completed, or failed).
	if job.Status != StatusQueued {
		log.Printf("job %s has status %s, skipping", jobID, job.Status)
		return nil
	}

	// Check quota before starting.
	if deps.QuotaStore != nil {
		remaining, err := deps.QuotaStore.CheckQuota(ctx, job.WorkspaceSlug)
		if err != nil {
			log.Printf("warning: quota check failed for %s: %v", job.WorkspaceSlug, err)
		} else if remaining <= 0 {
			_ = deps.JobStore.UpdateJobStatus(ctx, jobID, StatusFailed, "workspace AI quota exceeded")
			return fmt.Errorf("workspace %s quota exceeded", job.WorkspaceSlug)
		}
	}

	// Mark as processing.
	if err := deps.JobStore.UpdateJobStatus(ctx, jobID, StatusProcessing, ""); err != nil {
		return fmt.Errorf("set processing: %w", err)
	}

	// Run the translation; on failure, mark as failed.
	if err := executeTranslationWithDeps(ctx, deps, job); err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, jobID, StatusFailed, err.Error())
		return err
	}

	// Mark as completed.
	if err := deps.JobStore.UpdateJobStatus(ctx, jobID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	return nil
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

	// Resolve AI provider.
	prov, limiter, err := resolveProvider(ctx, deps, job)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	// Default batch/concurrency for automation jobs if not explicitly set.
	batchSz := job.BatchSize
	concurrency := job.Concurrency
	if batchSz < 1 {
		batchSz = 20
	}
	if concurrency < 1 {
		concurrency = 5
	}

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(job.TargetLocale),
		BatchSize:    batchSz,
		Concurrency:  concurrency,
	})

	// Process blocks in progress-reporting chunks. The tool handles
	// internal batching + concurrency; we chunk for progress updates.
	const progressChunk = 50
	var allOutParts []*model.Part
	totalTokensUsed := 0

	for i := 0; i < totalBlocks; i += progressChunk {
		end := i + progressChunk
		if end > totalBlocks {
			end = totalBlocks
		}
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

		// Estimate token usage (rough: ~4 chars per token for source + target).
		chunkTokens := estimateTokens(chunk)
		totalTokensUsed += chunkTokens

		// Record usage per chunk so quota is updated incrementally.
		if deps.QuotaStore != nil {
			_ = deps.QuotaStore.RecordUsage(ctx, AIUsageRecord{
				WorkspaceSlug: job.WorkspaceSlug,
				ProjectID:     job.ProjectID,
				JobID:         job.ID,
				Model:         job.Model,
				TotalTokens:   chunkTokens,
			})
		}

		// Update progress.
		if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, end, totalBlocks); err != nil {
			log.Printf("warning: update progress for %s: %v", job.ID, err)
		}
	}

	// Update total token usage on the job.
	job.TokensUsed = totalTokensUsed

	// Store translated blocks — they already have internal IDs from GetBlocks.
	blocks := partsToBlocks(allOutParts)
	if len(blocks) > 0 {
		if err := deps.ContentStore.StoreBlocks(ctx, job.ProjectID, "main", blocks); err != nil {
			return fmt.Errorf("store blocks: %w", err)
		}
	}

	return nil
}

// resolveProvider creates the appropriate LLM provider for the job.
func resolveProvider(ctx context.Context, deps *WorkerDeps, job *TranslationJob) (provider.LLMProvider, *rate.Limiter, error) {
	if job.IsPlatformProvider() {
		if deps.Platform == nil {
			return nil, nil, fmt.Errorf("platform provider not configured (set BOWRAIN_OPENAI_ENDPOINT)")
		}
		deployment := job.Model
		if deployment == "" {
			deployment = "gpt-4o"
		}
		prov, err := NewPlatformProvider(*deps.Platform, deployment)
		if err != nil {
			return nil, nil, err
		}
		limiter := rate.NewLimiter(providerRateLimit("azureopenai"), 1)
		return prov, limiter, nil
	}

	// User-configured provider via credential store.
	prov, err := credentials.NewProvider(deps.CredStore, job.ProviderConfigID)
	if err != nil {
		return nil, nil, err
	}
	cfg, _ := deps.CredStore.Get(job.ProviderConfigID)
	limiter := rate.NewLimiter(providerRateLimit(cfg.ProviderType), 1)
	return prov, limiter, nil
}

// estimateTokens provides a rough token count estimate for a batch of blocks.
// Uses ~4 characters per token as a heuristic (covers source + target).
func estimateTokens(blocks []*store.StoredBlock) int {
	totalChars := 0
	for _, sb := range blocks {
		if sb.Block != nil && len(sb.Block.Source) > 0 {
			for _, seg := range sb.Block.Source {
				if seg.Content != nil {
					totalChars += len(seg.Content.Text()) * 2 // source + target estimate
				}
			}
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
func runToolOnParts(ctx context.Context, t tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	if err := t.Process(ctx, in, out); err != nil {
		return nil, err
	}
	close(out)

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
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
