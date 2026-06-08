package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"golang.org/x/time/rate"
)

// KnownTermsLoader loads known terms for a project and locale.
// Used to avoid re-proposing already-approved terms during extraction.
type KnownTermsLoader interface {
	LoadKnownTerms(ctx context.Context, projectID string, locale string) ([]string, error)
}

// ExtractionWorkerDeps holds dependencies for the extraction worker.
type ExtractionWorkerDeps struct {
	ExtractionJobStore ExtractionJobStore
	ContentStore       bstore.ContentStore
	CredStore          *credentials.Store
	Queue              Queue
	ReviewQueueCreator ReviewQueueCreator
	KnownTermsLoader   KnownTermsLoader                                            // optional; nil disables known term filtering
	NERProvider        ner.Provider                                                // optional; nil disables NER pass
	Platform           *PlatformProviderConfig                                     // optional; nil disables platform provider
	LogFunc            func(stepID, level, message string, data map[string]string) // optional (Bowrain AD-013)
}

// ReviewQueueCreator creates review queue items from extraction results.
// This is implemented by the review queue store.
type ReviewQueueCreator interface {
	CreateReviewItem(ctx context.Context, item *ReviewQueueItem) error
	IsTermRejected(ctx context.Context, projectID, text, locale string) (bool, error)
}

// ReviewQueueItem is a lightweight struct for creating review items from the worker.
// It maps to bstore.ReviewItem but avoids importing the bowrain/store package.
type ReviewQueueItem struct {
	ProjectID   string
	Type        string // "term_candidate" or "entity_review"
	PushID      string
	Data        json.RawMessage
	Occurrences json.RawMessage
	Confidence  float64
	Locale      string
}

// RunExtractionWorker runs the extraction worker loop. It blocks until ctx is cancelled.
func RunExtractionWorker(ctx context.Context, deps *ExtractionWorkerDeps) error {
	slog.Info("extraction worker started")
	defer slog.Info("extraction worker stopped")

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
			slog.Info("extraction dequeue error", "error", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		if processErr := processExtractionJob(ctx, deps, jobID); processErr != nil {
			slog.Error("extraction job failed", "job_id", jobID, "error", processErr)
		}
		ack()
	}
}

func processExtractionJob(ctx context.Context, deps *ExtractionWorkerDeps, jobID string) error {
	// Atomically claim the job (queued → processing).
	claimed, err := deps.ExtractionJobStore.ClaimExtractionJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("claim extraction job: %w", err)
	}
	if !claimed {
		slog.Info("extraction job already claimed, skipping", "job_id", jobID)
		return nil
	}

	job, err := deps.ExtractionJobStore.GetExtractionJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load extraction job: %w", err)
	}

	emitExtractionLog(deps, job.StepID, "info",
		"Extracting entities from "+job.ItemName,
		map[string]string{"item": job.ItemName, "locale": job.Locale})

	if err := executeExtraction(ctx, deps, job); err != nil {
		_ = deps.ExtractionJobStore.UpdateExtractionJobStatus(ctx, jobID, ExtractionStatusFailed, err.Error())
		emitExtractionLog(deps, job.StepID, "error",
			"Extraction failed: "+err.Error(),
			map[string]string{"item": job.ItemName})
		return err
	}

	if err := deps.ExtractionJobStore.UpdateExtractionJobStatus(ctx, jobID, ExtractionStatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}

	emitExtractionLog(deps, job.StepID, "info",
		fmt.Sprintf("Extraction completed: %s — %d review items created", job.ItemName, job.ItemsCreated),
		map[string]string{"item": job.ItemName, "items_created": strconv.Itoa(job.ItemsCreated)})
	return nil
}

func executeExtraction(ctx context.Context, deps *ExtractionWorkerDeps, job *ExtractionJob) error {
	proj, err := deps.ContentStore.GetProject(ctx, job.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	storedBlocks, err := deps.ContentStore.GetBlocks(ctx, bstore.BlockQuery{
		ProjectID: job.ProjectID,
		Stream:    "main",
		ItemName:  job.ItemName,
	})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}

	totalBlocks := len(storedBlocks)
	if totalBlocks == 0 {
		return nil
	}

	if err := deps.ExtractionJobStore.UpdateExtractionJobProgress(ctx, job.ID, 0, totalBlocks, 0); err != nil {
		return fmt.Errorf("set total blocks: %w", err)
	}

	// Resolve AI provider.
	prov, limiter, err := resolveExtractionProvider(ctx, deps, job, proj)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	locale := model.LocaleID(job.Locale)
	if locale == "" {
		locale = proj.DefaultSourceLanguage
	}

	// Collect known terms from the project's termbase to avoid re-proposing.
	var knownTerms []string
	if deps.KnownTermsLoader != nil {
		loaded, err := deps.KnownTermsLoader.LoadKnownTerms(ctx, job.ProjectID, string(locale))
		if err != nil {
			slog.Info("extraction: failed to load known terms for", "id", job.ProjectID, "error", err)
		} else {
			knownTerms = loaded
		}
	}

	extractTool := tools.NewAIEntityExtractTool(prov, deps.NERProvider, tools.AIEntityExtractConfig{
		Locale:      locale,
		KnownTerms:  knownTerms,
		BatchSize:   10,
		Concurrency: 3,
	})

	// Process blocks through extraction tool.
	const progressChunk = 50
	var itemsCreated int

	for i := 0; i < totalBlocks; i += progressChunk {
		end := i + progressChunk
		if end > totalBlocks {
			end = totalBlocks
		}
		chunk := storedBlocks[i:end]

		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}

		parts := storedBlocksToParts(chunk)
		outParts, err := runToolOnParts(ctx, extractTool, parts)
		if err != nil {
			return fmt.Errorf("extract chunk %d-%d: %w", i, end, err)
		}

		// Create review queue items from annotations.
		created, err := createReviewItemsFromParts(ctx, deps, job, outParts, string(locale))
		if err != nil {
			slog.Warn("create review items for chunk failed", "start", i, "end", end, "error", err)
		}
		itemsCreated += created

		if err := deps.ExtractionJobStore.UpdateExtractionJobProgress(ctx, job.ID, end, totalBlocks, itemsCreated); err != nil {
			slog.Info("warning: update extraction progress for", "id", job.ID, "error", err)
		}
	}

	// Store annotated blocks back.
	allParts := storedBlocksToParts(storedBlocks)
	outParts, err := runToolOnParts(ctx, extractTool, allParts)
	if err == nil {
		blocks := partsToBlocks(outParts)
		if len(blocks) > 0 {
			if storeErr := deps.ContentStore.StoreBlocksForItem(ctx, job.ProjectID, "main", job.ItemName, blocks); storeErr != nil {
				slog.Warn("store annotated blocks failed", "error", storeErr)
			}
		}
	}

	return nil
}

// createReviewItemsFromParts extracts annotations from processed parts and creates review queue items.
func createReviewItemsFromParts(ctx context.Context, deps *ExtractionWorkerDeps, job *ExtractionJob, parts []*model.Part, locale string) (int, error) {
	if deps.ReviewQueueCreator == nil {
		return 0, nil
	}

	var created int
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}

		// Term candidates (positional facet spans).
		if f := block.FacetOf(model.FacetTermCandidate); f != nil {
			for _, span := range f.Spans {
				a, ok := span.Value.(*model.TermCandidateAnnotation)
				if !ok {
					continue
				}
				// Skip rejected terms.
				rejected, _ := deps.ReviewQueueCreator.IsTermRejected(ctx, job.ProjectID, a.Text, locale)
				if rejected {
					continue
				}

				data, _ := json.Marshal(a)
				ps, pe := span.Range.ByteSpan(block.Source)
				occ, _ := json.Marshal([]map[string]any{{
					"block_id": block.ID,
					"start":    ps,
					"end":      pe,
					"context":  block.SourceText(),
				}})

				if err := deps.ReviewQueueCreator.CreateReviewItem(ctx, &ReviewQueueItem{
					ProjectID:   job.ProjectID,
					Type:        "term_candidate",
					PushID:      job.PushID,
					Data:        data,
					Occurrences: occ,
					Confidence:  a.Confidence,
					Locale:      locale,
				}); err != nil {
					slog.Info("warning: create term candidate review item", "error", err)
					continue
				}
				created++
			}
		}

		// Entities (positional facet spans).
		if f := block.FacetOf(model.FacetEntity); f != nil {
			for _, span := range f.Spans {
				a, ok := span.Value.(*model.EntityAnnotation)
				if !ok {
					continue
				}

				data, _ := json.Marshal(a)
				ps, pe := span.Range.ByteSpan(block.Source)
				occ, _ := json.Marshal([]map[string]any{{
					"block_id": block.ID,
					"start":    ps,
					"end":      pe,
					"context":  block.SourceText(),
				}})

				if err := deps.ReviewQueueCreator.CreateReviewItem(ctx, &ReviewQueueItem{
					ProjectID:   job.ProjectID,
					Type:        "entity_review",
					PushID:      job.PushID,
					Data:        data,
					Occurrences: occ,
					Confidence:  0.9, // entities from LLM/NER are high-confidence
					Locale:      locale,
				}); err != nil {
					slog.Info("warning: create entity review item", "error", err)
					continue
				}
				created++
			}
		}
	}

	return created, nil
}

func resolveExtractionProvider(ctx context.Context, deps *ExtractionWorkerDeps, job *ExtractionJob, proj *bstore.Project) (aiprovider.LLMProvider, *rate.Limiter, error) {
	// Check for project-level AI provider config.
	providerConfigID := "platform"
	if proj.Properties != nil && proj.Properties["extraction_provider"] != "" {
		providerConfigID = proj.Properties["extraction_provider"]
	}

	modelName := job.Model
	if modelName == "" && proj.Properties != nil && proj.Properties["extraction_model"] != "" {
		modelName = proj.Properties["extraction_model"]
	}
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	if providerConfigID == "" || providerConfigID == "platform" {
		if deps.Platform == nil {
			return nil, nil, errors.New("platform provider not configured (set BOWRAIN_OPENAI_ENDPOINT)")
		}
		prov, err := NewPlatformProvider(*deps.Platform, modelName)
		if err != nil {
			return nil, nil, err
		}
		limiter := rate.NewLimiter(providerRateLimit("azureopenai"), 1)
		return prov, limiter, nil
	}

	prov, err := credentials.NewProvider(deps.CredStore, providerConfigID)
	if err != nil {
		return nil, nil, err
	}
	cfg, _ := deps.CredStore.Get(providerConfigID)
	limiter := rate.NewLimiter(providerRateLimit(cfg.ProviderType), 1)
	return prov, limiter, nil
}

func emitExtractionLog(deps *ExtractionWorkerDeps, stepID, level, message string, data map[string]string) {
	if deps.LogFunc != nil && stepID != "" {
		deps.LogFunc(stepID, level, message, data)
	}
}
