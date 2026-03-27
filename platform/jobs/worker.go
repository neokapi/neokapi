package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"encoding/json"
	"io"
	"strings"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/tool"
	apiclient "github.com/neokapi/neokapi/platform/client"
	platev "github.com/neokapi/neokapi/platform/event"
	"github.com/neokapi/neokapi/platform/store"
	"github.com/neokapi/neokapi/providers/ai"
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
	JobStore          JobStore
	ContentStore      store.ContentStore
	CredStore         *credentials.Store
	Queue             Queue
	QuotaStore        QuotaStore              // optional; nil disables quota enforcement
	Platform          *PlatformProviderConfig // optional; nil disables platform provider
	BillingHooks      *billing.UsageHooks     // optional; nil disables billing credit deduction
	// LogFunc is called to emit structured automation logs (AD-035).
	// Signature: func(stepID, level, message string, data map[string]string).
	// Optional; nil disables run logging.
	LogFunc func(stepID, level, message string, data map[string]string)
	// BlobStore provides access to push payloads for async sync processing (AD-037).
	BlobStore corestorage.BlobStore
	// EventBus publishes events after sync push processing (AD-037).
	EventBus platev.EventBus
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
	// Atomically claim the job (queued → processing). If another worker
	// already claimed it, skip without error. This prevents double-processing
	// when multiple workers dequeue the same job ID.
	claimed, err := deps.JobStore.ClaimJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("claim job: %w", err)
	}
	if !claimed {
		log.Printf("job %s already claimed by another worker, skipping", jobID)
		return nil
	}

	job, err := deps.JobStore.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}

	// Route sync-push jobs to dedicated handlers.
	if job.ItemName == "__sync_push__" {
		return processSyncPushJob(ctx, deps, job)
	}
	if job.ItemName == "__sync_push_v2__" {
		return processSyncPushV2Job(ctx, deps, job)
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

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Translating %s for %s", job.ItemName, job.TargetLocale),
		map[string]string{"item": job.ItemName, "locale": job.TargetLocale, "model": job.Model})

	// Run the translation; on failure, mark as failed.
	if err := executeTranslationWithDeps(ctx, deps, job); err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, jobID, StatusFailed, err.Error())
		emitLog(deps, job.StepID, "error",
			fmt.Sprintf("Translation failed: %s", err.Error()),
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
			"blocks": fmt.Sprintf("%d", job.DoneBlocks), "tokens": fmt.Sprintf("%d", job.TokensUsed)})

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
		SourceLocale: proj.DefaultSourceLanguage,
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

		// Deduct billing credits and report to Stripe Meters.
		if deps.BillingHooks != nil && job.WorkspaceID != "" {
			deps.BillingHooks.DeductTokens(ctx, job.WorkspaceID, chunkTokens, "ai_translation", job.ID)
		}

		// Update progress.
		if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, end, totalBlocks); err != nil {
			log.Printf("warning: update progress for %s: %v", job.ID, err)
		}
		job.DoneBlocks = end
		job.TotalBlocks = totalBlocks

		emitLog(deps, job.StepID, "info",
			fmt.Sprintf("Translated blocks %d-%d of %d (%d tokens)", i+1, end, totalBlocks, chunkTokens),
			map[string]string{"done": fmt.Sprintf("%d", end), "total": fmt.Sprintf("%d", totalBlocks)})
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

// syncPushPayload wraps the push request with context needed by the worker.
// Must match the struct in server/handlers_sync.go.
type syncPushPayload struct {
	Request apiclient.SyncPushRequest `json:"request"`
	UserID  string                    `json:"user_id"`
	WsSlug  string                    `json:"ws_slug"`
}

// processSyncPushJob handles async content ingestion (AD-037).
// It reads the push payload from blob storage, groups blocks by item,
// stores them, ensures items exist, and publishes EventPushCompleted.
func processSyncPushJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob) error {
	blobKey := job.Model          // blob storage key stored in Model field
	stream := job.TargetLocale    // stream stored in TargetLocale field
	projectID := job.ProjectID
	pushID := job.PushID

	if deps.BlobStore == nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "blob store not configured")
		return fmt.Errorf("blob store not configured for sync-push job")
	}

	emitLog(deps, job.StepID, "info", "Processing async push",
		map[string]string{"project": projectID, "push_id": pushID})

	// Download the push payload from blob storage.
	reader, err := deps.BlobStore.Download(ctx, blobKey)
	if err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "blob download failed: "+err.Error())
		return fmt.Errorf("download push blob %s: %w", blobKey, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "blob read failed: "+err.Error())
		return fmt.Errorf("read push blob: %w", err)
	}

	var payload syncPushPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, "invalid push payload: "+err.Error())
		return fmt.Errorf("unmarshal push payload: %w", err)
	}
	req := payload.Request

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Processing %d blocks for %s", len(req.Blocks), projectID),
		map[string]string{"blocks": fmt.Sprintf("%d", len(req.Blocks))})

	// Build item metadata map.
	itemMetaMap := make(map[string]apiclient.ItemMeta, len(req.Items))
	for _, im := range req.Items {
		itemMetaMap[im.Name] = im
	}

	// Group blocks by item_name.
	itemGroups := map[string][]*model.Block{}
	itemCollections := map[string]string{}
	for _, bi := range req.Blocks {
		b := &model.Block{
			ID:           bi.ID,
			Name:         bi.Name,
			Type:         bi.Type,
			Translatable: true,
		}
		b.SetSourceText(bi.Text)
		itemGroups[bi.ItemName] = append(itemGroups[bi.ItemName], b)
		if bi.Collection != "" && bi.ItemName != "" {
			itemCollections[bi.ItemName] = bi.Collection
		}
	}

	// Auto-create non-main streams.
	if stream != "main" {
		if _, err := deps.ContentStore.GetStream(ctx, projectID, stream); err != nil {
			baseCursor, _ := deps.ContentStore.LatestCursor(ctx, projectID, "main")
			_ = deps.ContentStore.CreateStream(ctx, &store.Stream{
				ProjectID:  projectID,
				Name:       stream,
				Parent:     "main",
				BaseCursor: baseCursor,
				Visibility: store.StreamPublic,
			})
		}
	}

	// Resolve collection names to IDs.
	collectionCache := map[string]string{}
	for _, collName := range itemCollections {
		if _, seen := collectionCache[collName]; seen {
			continue
		}
		coll, err := deps.ContentStore.GetCollectionByName(ctx, projectID, collName, stream)
		if err != nil {
			coll = &store.Collection{
				ProjectID: projectID,
				Name:      collName,
				Kind:      store.CollectionUploaded,
				ItemLabel: "item",
			}
			if createErr := deps.ContentStore.CreateCollection(ctx, coll); createErr == nil {
				collectionCache[collName] = coll.ID
			}
		} else {
			collectionCache[collName] = coll.ID
		}
	}

	// Store blocks per item.
	totalStored := 0
	for itemName, blocks := range itemGroups {
		if itemName != "" {
			if err := deps.ContentStore.StoreBlocksForItem(ctx, projectID, stream, itemName, blocks); err != nil {
				emitLog(deps, job.StepID, "error",
					fmt.Sprintf("Failed to store blocks for %s: %s", itemName, err.Error()), nil)
				_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
				return fmt.Errorf("store blocks for %s: %w", itemName, err)
			}
			// Ensure item exists in ContentStore for the editor UI.
			item := &store.Item{
				Name:     itemName,
				Format:   detectFormat(itemName),
				ItemType: "file",
			}
			if meta, ok := itemMetaMap[itemName]; ok {
				item.BlockIndex = meta.BlockIndex
				item.PreviewHTML = meta.PreviewHTML
				if meta.Format != "" {
					item.Format = meta.Format
				}
			}
			if collName, ok := itemCollections[itemName]; ok {
				item.CollectionID = collectionCache[collName]
			}
			_ = deps.ContentStore.StoreItem(ctx, projectID, stream, item)
		} else {
			if err := deps.ContentStore.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
				_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusFailed, err.Error())
				return fmt.Errorf("store blocks: %w", err)
			}
		}
		totalStored += len(blocks)
	}

	// Auto-set project default stream on first push.
	if totalStored > 0 {
		proj, projErr := deps.ContentStore.GetProject(ctx, projectID)
		if projErr == nil && proj.DefaultStream == "" {
			proj.DefaultStream = stream
			_ = deps.ContentStore.UpdateProject(ctx, proj)
		}
	}

	// Mark completed and clean up blob.
	_ = deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, "")
	_ = deps.BlobStore.Delete(ctx, blobKey)

	// Publish EventPushCompleted to trigger automations.
	if totalStored > 0 && deps.EventBus != nil {
		var itemNames []string
		for name := range itemGroups {
			if name != "" {
				itemNames = append(itemNames, name)
			}
		}
		deps.EventBus.Publish(platev.Event{
			Type:      platev.EventPushCompleted,
			Source:    "sync-worker",
			ProjectID: projectID,
			Actor:     payload.UserID,
			Data: map[string]string{
				"items":          strings.Join(itemNames, ","),
				"push_id":        pushID,
				"workspace_slug": payload.WsSlug,
			},
		})
	}

	emitLog(deps, job.StepID, "info",
		fmt.Sprintf("Async push completed: %d blocks across %d items", totalStored, len(itemGroups)),
		map[string]string{"blocks": fmt.Sprintf("%d", totalStored), "items": fmt.Sprintf("%d", len(itemGroups))})

	return nil
}

// detectFormat infers a format name from a file extension.
func detectFormat(name string) string {
	switch {
	case strings.HasSuffix(name, ".json"):
		return "json"
	case strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml"):
		return "yaml"
	case strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".xliff"):
		return "xml"
	case strings.HasSuffix(name, ".po"):
		return "po"
	case strings.HasSuffix(name, ".properties"):
		return "properties"
	case strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm"):
		return "html"
	case strings.HasSuffix(name, ".md"):
		return "markdown"
	case strings.HasSuffix(name, ".csv"):
		return "csv"
	case strings.HasSuffix(name, ".txt"):
		return "plaintext"
	default:
		return "json"
	}
}
