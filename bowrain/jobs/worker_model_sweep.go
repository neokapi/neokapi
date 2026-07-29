package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/observe"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"golang.org/x/time/rate"
)

// processModelSweepJob executes one durable model recommendation sweep for a
// (project, locale): derive the fixture set from the project's own content,
// translate it twice per candidate model — once with the project's full brand
// context (voice + glossary + DNT, the #1334 binding) and once bare — score
// both arms deterministically with the real core check tools, and persist one
// (project, locale, model, fixture_digest) row per candidate.
//
// Cost is bounded by construction: ≤ maxSweepFixtures segments × 2 arms ×
// len(candidates) LLM calls' worth of text per job, on the PLATFORM provider.
//
// Billing: this is platform QC, not customer work. Token usage IS recorded to
// ai_usage with the distinct "model_sweep" operation (the abuse cap and the
// ops dashboards must see all traffic), but BillingHooks are deliberately
// NEVER invoked — a sweep must not deduct customer credits.
//
// Failure policy: missing wiring (no sweep store/settings) is terminal —
// retrying cannot help. A disabled flag at execution time completes the job as
// a no-op (the gate is respected at both enqueue and execution). Every other
// failure retries through the shared attempt budget: the sweep is idempotent
// (results upsert by digest), so at-least-once delivery is safe.
func processModelSweepJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64) error {
	stopHeartbeat := startLeaseHeartbeat(ctx, deps.JobStore, job.ID, epoch)
	defer stopHeartbeat()

	if deps.SweepStore == nil || deps.SweepSettings == nil {
		err := errors.New("model sweep: worker has no sweep store/settings wired")
		_, _ = deps.JobStore.FailJob(ctx, job.ID, epoch, err.Error())
		return err
	}
	if job.ProjectID == "" || job.TargetLocale == "" {
		err := fmt.Errorf("model sweep: job %s carries no project/locale", job.ID)
		_, _ = deps.JobStore.FailJob(ctx, job.ID, epoch, err.Error())
		return err
	}
	if !deps.SweepSettings.ModelSweepsEnabled() {
		// The founder turned the flag off between enqueue and execution: a
		// normal race, not an operational failure. Record and stop.
		slog.InfoContext(ctx, "model sweep: model_sweeps.enabled is off; dropping job", "job_id", job.ID)
		return deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, "")
	}

	if err := runModelSweep(ctx, deps, job, epoch); err != nil {
		if errors.Is(err, errLeaseLost) {
			slog.InfoContext(ctx, "model sweep: lease lost mid-run; abandoning to fresh owner", "job_id", job.ID)
			return nil
		}
		return retryOrFailSweep(ctx, deps, job, epoch, err)
	}
	if err := deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	return nil
}

// retryOrFailSweep applies the shared transient-failure bookkeeping to a sweep
// job. Like forge ingest, every failure is retried through the bounded attempt
// budget: provider hiccups (429/5xx/timeouts) are exactly what the durable
// queue absorbs, the sweep is idempotent (upsert by digest), and the bounded
// budget keeps a genuinely broken configuration from looping forever.
func retryOrFailSweep(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64, cause error) error {
	retry, rerr := deps.JobStore.RetryOrFail(ctx, job.ID, epoch, deps.maxJobAttempts(), cause.Error())
	if rerr != nil {
		slog.WarnContext(ctx, "model sweep: retry bookkeeping failed", "job_id", job.ID, "error", rerr)
	}
	if retry {
		return &transientError{err: cause}
	}
	return cause
}

// runModelSweep is the sweep body: fixtures → per-model two-arm translation →
// deterministic scoring → persisted rows.
func runModelSweep(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64) error {
	proj, err := deps.ContentStore.GetProject(ctx, job.ProjectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	// The standing brand context — resolved through the same #1334 binding the
	// translation worker applies to every AI job.
	sc := resolveSweepContext(ctx, deps, job, proj)
	if sc.Empty() {
		slog.InfoContext(ctx, "model sweep: project has no brand context (no profile, glossary, or DNT); nothing to measure",
			"job_id", job.ID, "project", job.ProjectID)
		return nil
	}

	translatable := true
	blocks, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID:    job.ProjectID,
		Stream:       "main",
		Translatable: &translatable,
	})
	if err != nil {
		return fmt.Errorf("get blocks: %w", err)
	}
	fixtures := DeriveSweepFixtures(blocks, sc)
	if len(fixtures) == 0 {
		slog.InfoContext(ctx, "model sweep: no trap segments found; nothing to measure",
			"job_id", job.ID, "project", job.ProjectID, "locale", job.TargetLocale)
		return nil
	}
	digest := SweepFixtureDigest(job.ProjectID, job.TargetLocale, fixtures, sc)

	// The platform provider config is the base every candidate model runs on
	// (recommendation sweeps use the platform machinery — never a BYO key).
	platform := activePlatform(ctx, job.WorkspaceID, deps.Platform, deps.PlatformResolver)
	if platform == nil {
		return errors.New("model sweep: platform provider not configured")
	}
	candidates := deps.SweepSettings.AIEnabledModels()
	if len(candidates) == 0 && platform.Model != "" {
		candidates = []string{platform.Model}
	}
	if len(candidates) == 0 {
		return errors.New("model sweep: no candidate models (enable models in ctrl or set a platform default)")
	}

	if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, 0, len(candidates)); err != nil {
		return fmt.Errorf("set total: %w", err)
	}

	srcLocale := proj.DefaultSourceLanguage
	tgtLocale := model.LocaleID(job.TargetLocale)

	for i, candidate := range candidates {
		// Lease gate before each (potentially slow) candidate: a swept-and-
		// reclaimed job must stop before writing rows the fresh owner will
		// also write.
		owner, lerr := deps.JobStore.RenewLease(ctx, job.ID, epoch)
		if lerr != nil {
			return fmt.Errorf("renew lease: %w", lerr)
		}
		if !owner {
			return errLeaseLost
		}

		p := *platform
		p.Model = candidate
		prov, ptype, err := p.Build("")
		if err != nil {
			return fmt.Errorf("build provider for %q: %w", candidate, err)
		}
		limiter := rate.NewLimiter(providerRateLimit(ptype), 1)

		// Arm 1 — full context: the project's voice profile, glossary, and
		// enforced DNT terms, exactly as jobTranslateConfig binds them.
		ctxArm, ctxTokens, err := runSweepArm(ctx, prov, limiter, fixtures, aitools.AITranslateConfig{
			SourceLocale: srcLocale,
			TargetLocale: tgtLocale,
			Profile:      sc.Profile,
			Glossary:     sc.Glossary,
			DNT:          sc.DNT,
		}, tgtLocale, sc)
		if err != nil {
			return fmt.Errorf("model %q context arm: %w", candidate, err)
		}

		// Arm 2 — bare: the same fixtures with no context at all. The delta is
		// the measured steerability lift.
		bareArm, bareTokens, err := runSweepArm(ctx, prov, limiter, fixtures, aitools.AITranslateConfig{
			SourceLocale: srcLocale,
			TargetLocale: tgtLocale,
		}, tgtLocale, sc)
		if err != nil {
			return fmt.Errorf("model %q bare arm: %w", candidate, err)
		}

		tokens := ctxTokens.TotalTokens() + bareTokens.TotalTokens()

		// Record token usage with a distinct operation so the abuse cap and the
		// ops dashboards see sweep traffic as sweep traffic. BillingHooks are
		// DELIBERATELY not invoked here: measured steerability is platform QC
		// and must never deduct customer credits (contrast the per-chunk
		// DeductTokens in executeTranslationWithDeps).
		//
		// Fail-open by policy, as everywhere else on the metering path: the
		// sweep has already paid for both arms, and losing the measurement over
		// a meter outage would waste that spend twice. The discard is logged and
		// counted so the unrecorded platform spend is still visible.
		if deps.QuotaStore != nil {
			if err := deps.QuotaStore.RecordUsage(ctx, AIUsageRecord{
				WorkspaceSlug: job.WorkspaceSlug,
				WorkspaceID:   job.WorkspaceID,
				ProjectID:     job.ProjectID,
				JobID:         job.ID,
				Model:         candidate,
				Operation:     "model_sweep",
				PromptTokens:  ctxTokens.InputTokens + bareTokens.InputTokens,
				OutputTokens:  ctxTokens.OutputTokens + bareTokens.OutputTokens,
				TotalTokens:   tokens,
			}); err != nil {
				observe.MeteringDiscarded(ctx, observe.MeterAITokens, float64(tokens), err,
					"operation", "model_sweep", "job_id", job.ID,
					"workspace", job.WorkspaceSlug, "model", candidate)
			}
		}

		// Final per-candidate lease gate before persisting the measurement.
		owner, lerr = deps.JobStore.RenewLease(ctx, job.ID, epoch)
		if lerr != nil {
			return fmt.Errorf("renew lease: %w", lerr)
		}
		if !owner {
			return errLeaseLost
		}

		result := &ModelSweepResult{
			ProjectID:     job.ProjectID,
			Locale:        job.TargetLocale,
			Model:         candidate,
			FixtureDigest: digest,
			FixtureCount:  len(fixtures),
			Adherence:     ctxArm.Rate(),
			AdherenceBare: bareArm.Rate(),
			Lift:          ctxArm.Rate() - bareArm.Rate(),
			TokensUsed:    tokens,
			MeasuredAt:    time.Now().UTC(),
		}
		if err := deps.SweepStore.UpsertModelSweepResult(ctx, result); err != nil {
			return fmt.Errorf("persist sweep result for %q: %w", candidate, err)
		}
		slog.InfoContext(ctx, "model sweep: measured candidate",
			"job_id", job.ID, "project", job.ProjectID, "locale", job.TargetLocale,
			"model", candidate, "fixtures", len(fixtures),
			"adherence", result.Adherence, "adherence_bare", result.AdherenceBare,
			"lift", result.Lift, "tokens", tokens)

		if err := deps.JobStore.UpdateJobProgress(ctx, job.ID, epoch, i+1, len(candidates)); err != nil {
			slog.WarnContext(ctx, "model sweep: update progress failed", "job_id", job.ID, "error", err)
		}
	}
	return nil
}

// runSweepArm translates the fixture set once under the given config and
// scores the result deterministically. Fixture blocks are constructed fresh
// per arm (plain-text source), so no arm — and no sweep — ever writes into the
// project's stored content: the translations exist only to be measured.
func runSweepArm(ctx context.Context, prov aiprovider.LLMProvider, limiter *rate.Limiter, fixtures []SweepFixture, cfg aitools.AITranslateConfig, locale model.LocaleID, sc *SweepContext) (sweepArmScore, aiprovider.TokenUsage, error) {
	if err := limiter.Wait(ctx); err != nil {
		return sweepArmScore{}, aiprovider.TokenUsage{}, fmt.Errorf("rate limit: %w", err)
	}
	parts := make([]*model.Part, 0, len(fixtures))
	for _, f := range fixtures {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: model.NewBlock(f.BlockID, f.SourceText)})
	}
	translateTool := aitools.NewAITranslateTool(prov, cfg)
	outParts, err := runToolOnParts(ctx, translateTool, parts)
	if err != nil {
		return sweepArmScore{}, aiprovider.TokenUsage{}, err
	}
	usage := translateTool.TotalUsage()
	score, err := scoreSweepBlocks(ctx, partsToBlocks(outParts), locale, sc)
	return score, usage, err
}
