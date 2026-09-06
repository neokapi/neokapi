package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
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
	KnownTermsLoader   KnownTermsLoader        // optional; nil disables known term filtering
	NERProvider        ner.Provider            // optional; nil disables NER pass
	Platform           *PlatformProviderConfig // optional; nil disables platform provider
	PlatformResolver   PlatformResolver        // optional; consulted at job time for runtime config changes (overrides Platform)
	// BillingHooks deducts platform credits for what an extraction spends.
	// Optional; nil (self-hosted / unbilled) disables credit deduction.
	BillingHooks *billing.UsageHooks
	// QuotaStore is the internal AI abuse cap (ai_usage / ai_quotas):
	// extractions check the hard monthly token ceiling before the first model
	// call and record what they spend, exactly like translation jobs, so an
	// extraction is never invisible to abuse detection. Distinct from the
	// credit ledger (BillingHooks). Optional; nil disables quota enforcement.
	QuotaStore QuotaStore
	LogFunc    func(stepID, level, message string, data map[string]string) // optional (Bowrain AD-013)
	// EventBus publishes flow.failed when an extraction job fails, so the
	// failure reaches the people waiting on it rather than only the job row.
	// Optional; nil records the failure and summons nobody.
	EventBus platev.EventBus
	// MaxJobAttempts bounds transient-failure retries before a job is failed.
	// Zero uses defaultMaxJobAttempts.
	MaxJobAttempts int
	// DrainGrace bounds how long a job already running when shutdown is
	// signalled may keep running. Zero uses defaultDrainGrace.
	DrainGrace time.Duration
}

// maxJobAttempts returns the configured retry budget or the default.
func (d *ExtractionWorkerDeps) maxJobAttempts() int {
	if d.MaxJobAttempts > 0 {
		return d.MaxJobAttempts
	}
	return defaultMaxJobAttempts
}

// drainGrace returns the configured shutdown grace or the default.
func (d *ExtractionWorkerDeps) drainGrace() time.Duration {
	if d.DrainGrace > 0 {
		return d.DrainGrace
	}
	return defaultDrainGrace
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
//
// The ack/nack branches mirror the translation loop's, because the failure
// modes are the same: a provider 503 used to strand the job in 'processing'
// forever — nack discarded, ack unconditional, no attempts column and no
// sweeper — and the push-completion tracker then reported the push as in
// progress until its thirty-minute timeout.
func RunExtractionWorker(ctx context.Context, deps *ExtractionWorkerDeps) error {
	slog.Info("extraction worker started")
	defer slog.Info("extraction worker stopped")

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
			slog.Info("extraction dequeue error", "error", err)
			sleepCtx(ctx, 2*time.Second)
			continue
		}

		// The loop's context gates Dequeue; the job body runs detached from
		// the shutdown signal and bounded by the drain grace.
		// One transaction per job, named by the queue so it aggregates; the job
		// id rides as the correlation tag, which is also what puts request_id
		// on this job's log lines.
		runCtx, stopDrain := drainableJobContext(ctx, deps.drainGrace())
		traceCtx, endTrace := observe.Transaction(observe.WithRequestID(runCtx, jobID), "queue.task", "extraction")
		processErr := processExtractionJob(traceCtx, deps, jobID)
		endTrace(processErr)
		stopDrain()
		postCtx := context.WithoutCancel(ctx)

		if processErr != nil {
			if _, ok := errors.AsType[*transientError](processErr); ok {
				// The row is back in 'queued'. Publish a FRESH message rather
				// than trusting nack() to reproduce a delivery whose visibility
				// may already have lapsed; ClaimExtractionJob dedups a stray
				// concurrent redelivery.
				if eqErr := deps.Queue.Enqueue(postCtx, jobID); eqErr != nil {
					slog.Warn("extraction transient failure; re-enqueue failed, nacking instead",
						"job_id", jobID, "error", processErr, "enqueue_error", eqErr)
					nack()
				} else {
					slog.Warn("extraction transient failure; re-enqueued fresh message for retry",
						"job_id", jobID, "error", processErr)
					ack()
				}
				continue
			}
			slog.Error("extraction job failed", "job_id", jobID, "error", processErr)
		}
		ack()
	}
}

func processExtractionJob(ctx context.Context, deps *ExtractionWorkerDeps, jobID string) error {
	// Atomically claim the job (queued → processing) and take the lease.
	claimed, epoch, err := deps.ExtractionJobStore.ClaimExtractionJob(ctx, jobID)
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

	// Lease heartbeat: a long extraction must not be swept out from under
	// itself while it is making progress.
	stopHeartbeat := startLeaseHeartbeat(ctx, deps.ExtractionJobStore, jobID, epoch)
	defer stopHeartbeat()

	emitExtractionLog(deps, job.StepID, "info",
		"Extracting entities from "+job.ItemName,
		map[string]string{"item": job.ItemName, "locale": job.Locale})

	if err := executeExtraction(ctx, deps, job, epoch); err != nil {
		// A cancellation or the sweeper's re-claim took the lease mid-run.
		// Abandon quietly: do NOT mark failed (that would clobber the reason a
		// person's cancel wrote, or the fresh owner's run) and do NOT retry.
		if errors.Is(err, errLeaseLost) {
			slog.InfoContext(ctx, "extraction lease lost mid-run; abandoning",
				"job_id", jobID)
			return nil
		}
		// Shutdown reached the job before it finished: nothing the job did, so
		// put it back rather than failing it. Bookkeeping runs detached,
		// because the context that carried the cancellation cannot write.
		if ctx.Err() != nil {
			book := context.WithoutCancel(ctx)
			if _, rerr := deps.ExtractionJobStore.RetryOrFail(book, jobID, epoch, deps.maxJobAttempts(),
				"interrupted by worker shutdown"); rerr != nil {
				slog.Warn("extraction shutdown bookkeeping failed", "job_id", jobID, "error", rerr)
			}
			return &transientError{err: err}
		}
		if isTransientError(err) {
			retry, rerr := deps.ExtractionJobStore.RetryOrFail(ctx, jobID, epoch, deps.maxJobAttempts(), err.Error())
			if rerr != nil {
				slog.Warn("extraction retry bookkeeping failed", "job_id", jobID, "error", rerr)
			}
			if retry {
				emitExtractionLog(deps, job.StepID, "warn",
					"Transient error, retrying: "+err.Error(),
					map[string]string{"item": job.ItemName})
				return &transientError{err: err}
			}
			// Retry budget exhausted — RetryOrFail marked the job failed.
			emitExtractionLog(deps, job.StepID, "error",
				"Extraction failed after retries: "+err.Error(),
				map[string]string{"item": job.ItemName})
			publishExtractionJobFailed(deps.EventBus, job, err.Error())
			return err
		}
		owner, ferr := deps.ExtractionJobStore.FailExtractionJob(ctx, jobID, epoch, err.Error())
		if ferr != nil {
			slog.Warn("extraction failure bookkeeping failed", "job_id", jobID, "error", ferr)
		} else if !owner {
			slog.Info("extraction lease lost; leaving fresh owner's run untouched", "job_id", jobID)
			return nil
		}
		emitExtractionLog(deps, job.StepID, "error",
			"Extraction failed: "+err.Error(),
			map[string]string{"item": job.ItemName})
		// The permanent branch has ruled out retry, so this failure is the only
		// one this job will ever have: publishing here is publishing once.
		publishExtractionJobFailed(deps.EventBus, job, err.Error())
		return err
	}

	// Completion is lease-guarded, so a job cancelled while this worker was
	// running it stays cancelled: without the guard the worker wrote
	// 'completed' over the cancellation and the extraction it was stopped for
	// counted as having finished.
	owner, err := deps.ExtractionJobStore.CompleteExtractionJob(ctx, jobID, epoch)
	if err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	if !owner {
		slog.InfoContext(ctx, "extraction lease lost before completion; leaving the record as it stands",
			"job_id", jobID)
		return nil
	}

	emitExtractionLog(deps, job.StepID, "info",
		fmt.Sprintf("Extraction completed: %s, %d review items created", job.ItemName, job.ItemsCreated),
		map[string]string{"item": job.ItemName, "items_created": strconv.Itoa(job.ItemsCreated)})
	return nil
}

func executeExtraction(ctx context.Context, deps *ExtractionWorkerDeps, job *ExtractionJob, epoch int64) error {
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
	resolved, err := resolveExtractionProvider(ctx, deps, job, proj)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}
	limiter := resolved.Limiter

	// The metering scope of this job: every model call the extract tool makes
	// lands in it, and Settle records the usage and deducts the credits once.
	// It settles even when the run fails, because the tokens a run burned
	// before it stopped are spent either way, and under a context that outlives
	// the job's own cancellation.
	aiRun := service.NewAIRun(accountantFor(deps, job), job.WorkspaceID, job.ProjectID, job.ID)
	defer aiRun.Settle(context.WithoutCancel(ctx))

	// The balance is checked here, before the first model call: deduction is
	// post-hoc, so a workspace with nothing to spend must fail before a token
	// is burned rather than after the whole item has been extracted.
	source := spendSourceOf(resolved.Source)
	if err := aiRun.Admit(ctx, source); err != nil {
		return err
	}
	prov := aiRun.Meter(resolved.LLM, usageOpEntityExtract, source)

	locale := model.LocaleID(job.Locale)
	if locale == "" {
		locale = proj.DefaultSourceLanguage
	}

	// Collect known terms from the project's terms to avoid re-proposing.
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
	// The blocks each chunk came back annotated with, kept for the single
	// write-back below. One pass over the item both creates the review items
	// and produces the annotations that are stored.
	annotated := make([]*model.Block, 0, totalBlocks)

	for i := 0; i < totalBlocks; i += progressChunk {
		end := min(i+progressChunk, totalBlocks)
		chunk := storedBlocks[i:end]

		// The chunk boundary is where a cancellation reaches this worker: a
		// cancel bumps claim_epoch, so the renewal reports !owner and the run
		// stops here rather than extracting to the end and creating the items
		// it was stopped for. It also refreshes updated_at, so a job making
		// steady progress never looks stale to the sweeper.
		owner, lerr := deps.ExtractionJobStore.RenewLease(ctx, job.ID, epoch)
		if lerr != nil {
			return fmt.Errorf("renew lease: %w", lerr)
		}
		if !owner {
			return errLeaseLost
		}

		if err := limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limit: %w", err)
		}

		parts := storedBlocksToParts(chunk)
		outParts, err := runToolOnParts(ctx, extractTool, parts)
		if err != nil {
			return fmt.Errorf("extract chunk %d-%d: %w", i, end, err)
		}

		annotated = append(annotated, partsToBlocks(outParts)...)

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

	// The last checkpoint before the annotated blocks are written back, which
	// is the one write a cancelled run must not make.
	owner, lerr := deps.ExtractionJobStore.RenewLease(ctx, job.ID, epoch)
	if lerr != nil {
		return fmt.Errorf("renew lease: %w", lerr)
	}
	if !owner {
		return errLeaseLost
	}

	// Store the blocks the chunk loop annotated.
	if len(annotated) > 0 {
		if storeErr := deps.ContentStore.StoreBlocksForItem(ctx, job.ProjectID, "main", job.ItemName, annotated); storeErr != nil {
			slog.Warn("store annotated blocks failed", "error", storeErr)
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

		// Term candidates (overlay spans).
		if f := block.OverlayOf(model.OverlayTermCandidate); f != nil {
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

		// Entities (overlay spans).
		if f := block.OverlayOf(model.OverlayEntity); f != nil {
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

func resolveExtractionProvider(ctx context.Context, deps *ExtractionWorkerDeps, job *ExtractionJob, proj *bstore.Project) (ResolvedProvider, error) {
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
		// Extraction is not translation: it stays on the platform default model
		// (a workspace's chosen translation model must not silently raise
		// extraction cost), so it resolves with no workspace scope.
		platform := activePlatform(ctx, "", deps.Platform, deps.PlatformResolver)
		if platform == nil {
			return ResolvedProvider{}, errors.New("platform provider not configured " +
				"(set BOWRAIN_PLATFORM_PROVIDER + key, or BOWRAIN_OPENAI_ENDPOINT)")
		}
		// Build resolves the generic (e.g. bedrock) or Azure path from the same
		// config; the operator-configured Model wins over the Azure-centric
		// modelName default for non-Azure providers.
		prov, ptype, err := platform.Build(modelName)
		if err != nil {
			return ResolvedProvider{}, err
		}
		return ResolvedProvider{
			LLM:     prov,
			Limiter: rate.NewLimiter(providerRateLimit(ptype), 1),
			Source:  ProviderSourcePlatform,
		}, nil
	}

	prov, err := credentials.NewProvider(deps.CredStore, providerConfigID)
	if err != nil {
		return ResolvedProvider{}, err
	}
	cfg, _ := deps.CredStore.Get(providerConfigID)
	return ResolvedProvider{
		LLM:     prov,
		Limiter: rate.NewLimiter(providerRateLimit(cfg.ProviderType), 1),
		Source:  ProviderSourceBYO,
	}, nil
}

func emitExtractionLog(deps *ExtractionWorkerDeps, stepID, level, message string, data map[string]string) {
	if deps.LogFunc != nil && stepID != "" {
		deps.LogFunc(stepID, level, message, data)
	}
}
