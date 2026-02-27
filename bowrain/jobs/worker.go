package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gokapi/gokapi/bowrain/credentials"
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

// providerRateLimits maps provider types to their default rate limits (requests/sec).
var providerRateLimits = map[string]rate.Limit{
	"openai":    10,
	"anthropic": 5,
	"ollama":    100, // effectively unlimited
}

// RunWorker runs the translation worker loop. It dequeues job IDs, loads the
// corresponding TranslationJob, processes blocks through the AI translate tool,
// and stores results back. It blocks until ctx is cancelled.
func RunWorker(ctx context.Context, jobStore JobStore, contentStore store.ContentStore, credStore *credentials.Store, queue Queue) error {
	log.Println("translation worker started")
	defer log.Println("translation worker stopped")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		jobID, ack, _, err := queue.Dequeue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("dequeue error: %v", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		if processErr := processJob(ctx, jobStore, contentStore, credStore, jobID); processErr != nil {
			log.Printf("job %s failed: %v", jobID, processErr)
		}
		// Always ack: processJob marks failed jobs in the database.
		// Nacking would cause infinite retries for permanent errors.
		ack()
	}
}

func processJob(ctx context.Context, jobStore JobStore, cs store.ContentStore, credStore *credentials.Store, jobID string) error {
	job, err := jobStore.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}

	// Skip jobs that aren't queued (already processing, completed, or failed).
	if job.Status != StatusQueued {
		log.Printf("job %s has status %s, skipping", jobID, job.Status)
		return nil
	}

	// Mark as processing.
	if err := jobStore.UpdateJobStatus(ctx, jobID, StatusProcessing, ""); err != nil {
		return fmt.Errorf("set processing: %w", err)
	}

	// Run the translation; on failure, mark as failed.
	if err := executeTranslation(ctx, jobStore, cs, credStore, job); err != nil {
		_ = jobStore.UpdateJobStatus(ctx, jobID, StatusFailed, err.Error())
		return err
	}

	// Mark as completed.
	if err := jobStore.UpdateJobStatus(ctx, jobID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	return nil
}

func executeTranslation(ctx context.Context, jobStore JobStore, cs store.ContentStore, credStore *credentials.Store, job *TranslationJob) error {
	proj, err := cs.GetProject(ctx, job.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: job.ProjectID,
		ItemName:  job.ItemName,
	})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	totalBlocks := len(storedBlocks)
	if err := jobStore.UpdateJobProgress(ctx, job.ID, 0, totalBlocks); err != nil {
		return fmt.Errorf("set total blocks: %w", err)
	}

	// Resolve AI provider.
	prov, err := credentials.NewProvider(credStore, job.ProviderConfigID)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	// Get rate limit for provider type.
	cfg, _ := credStore.Get(job.ProviderConfigID)
	limiter := rate.NewLimiter(providerRateLimit(cfg.ProviderType), 1)

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(job.TargetLocale),
	})

	// Process blocks in batches to report progress.
	const batchSize = 10
	var allOutParts []*model.Part

	for i := 0; i < totalBlocks; i += batchSize {
		end := i + batchSize
		if end > totalBlocks {
			end = totalBlocks
		}
		batch := storedBlocks[i:end]

		// Rate limit.
		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}

		parts := storedBlocksToParts(batch)
		outParts, err := runToolOnParts(ctx, translateTool, parts)
		if err != nil {
			return fmt.Errorf("translate batch %d-%d: %w", i, end, err)
		}
		allOutParts = append(allOutParts, outParts...)

		// Update progress.
		if err := jobStore.UpdateJobProgress(ctx, job.ID, end, totalBlocks); err != nil {
			log.Printf("warning: update progress for %s: %v", job.ID, err)
		}
	}

	// Store translated blocks.
	blocks := partsToBlocks(allOutParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocksForItem(ctx, job.ProjectID, job.ItemName, blocks); err != nil {
			return fmt.Errorf("store blocks: %w", err)
		}
	}

	return nil
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
