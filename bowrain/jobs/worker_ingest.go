package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/forge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// processForgeIngestJob executes one durable forge-ingest job (webhook- or
// bind-triggered source ingest, enqueued by the server): re-read the persisted
// connector config, fetch+ingest through the shared ForgeIngest core, and
// settle the job row.
//
// Failure policy: a missing connector config (removed between enqueue and
// execution) is terminal — retrying cannot help. Every fetch failure is
// retried through the shared attempt budget (RetryOrFail): mid-deploy network
// blips, forge hiccups, and rate limits — exactly the failures the durable
// queue exists to absorb — are indistinguishable from permanent config errors
// in git's stringly output, and the bounded budget keeps a genuinely broken
// clone from looping forever. The last failure is recorded on the connector
// row (ForgeIngest does that), so the status path surfaces it while retries
// run and after they exhaust.
//
// At-least-once is safe here: the ingest is idempotent (see ForgeIngest), so a
// sweeper resurrection or broker redelivery at worst re-fetches the same tree.
func processForgeIngestJob(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64) error {
	// Lease heartbeat: a clone of a large repository can be slow; keep the row
	// fresh so the stale-job sweeper never resurrects a live ingest.
	stopHeartbeat := startLeaseHeartbeat(ctx, deps.JobStore, job.ID, epoch)
	defer stopHeartbeat()

	connectorID := job.IngestConnectorID()
	if deps.ConnectorFetcher == nil || deps.ConnectorConfigs == nil {
		err := errors.New("forge ingest: worker has no connector fetcher/config store wired")
		_, _ = deps.JobStore.FailJob(ctx, job.ID, epoch, err.Error())
		return err
	}
	if connectorID == "" || job.ProjectID == "" {
		err := fmt.Errorf("forge ingest: job %s carries no connector/project", job.ID)
		_, _ = deps.JobStore.FailJob(ctx, job.ID, epoch, err.Error())
		return err
	}

	cfg, err := deps.ConnectorConfigs.Get(ctx, job.WorkspaceID, connectorID)
	if err != nil {
		if errors.Is(err, bstore.ErrConnectorConfigNotFound) {
			// The connector was removed while the job was queued: a normal
			// race, not an operational failure. Record and stop.
			slog.InfoContext(ctx, "forge ingest: connector removed before ingest ran; dropping job",
				"job_id", job.ID, "connector", connectorID)
			_, _ = deps.JobStore.FailJob(ctx, job.ID, epoch, "connector no longer exists")
			return nil
		}
		return retryOrFailIngest(ctx, deps, job, epoch, fmt.Errorf("load connector config: %w", err))
	}

	params := ForgeIngestParams{
		WorkspaceID: job.WorkspaceID,
		ConnectorID: connectorID,
		ProjectID:   job.ProjectID,
		Repo:        ingestRepoPath(cfg),
		Branch:      ingestBranch(cfg),
		Source:      job.IngestSource(),
	}
	ingestDeps := ForgeIngestDeps{
		Fetcher:  deps.ConnectorFetcher,
		Recorder: deps.ConnectorConfigs,
		EventBus: deps.EventBus,
	}
	if err := ForgeIngest(ctx, ingestDeps, params); err != nil {
		return retryOrFailIngest(ctx, deps, job, epoch, err)
	}

	if err := deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusCompleted, ""); err != nil {
		return fmt.Errorf("set completed: %w", err)
	}
	slog.InfoContext(ctx, "forge ingest: completed",
		"job_id", job.ID, "connector", connectorID, "project", job.ProjectID, "source", params.Source)
	return nil
}

// retryOrFailIngest applies the shared transient-failure bookkeeping to an
// ingest job: with attempt budget left the row goes back to 'queued' and a
// transientError tells the worker loop to re-deliver; once the budget is
// exhausted RetryOrFail has marked it failed.
func retryOrFailIngest(ctx context.Context, deps *WorkerDeps, job *TranslationJob, epoch int64, cause error) error {
	retry, rerr := deps.JobStore.RetryOrFail(ctx, job.ID, epoch, deps.maxJobAttempts(), cause.Error())
	if rerr != nil {
		slog.WarnContext(ctx, "forge ingest: retry bookkeeping failed", "job_id", job.ID, "error", rerr)
	}
	if retry {
		return &transientError{err: cause}
	}
	return cause
}

// ingestBranch reads the connector's tracked branch (the webhook handlers only
// enqueue for pushes to it, so it is also the pushed branch).
func ingestBranch(cfg bstore.ConnectorConfig) string {
	if b := cfg.Config["branch"]; b != "" {
		return b
	}
	return "main"
}

// ingestRepoPath derives the owner/name repo path for the push event from the
// connector's configured repo URL; the raw config value is the fallback for
// an unparseable remote.
func ingestRepoPath(cfg bstore.ConnectorConfig) string {
	if repo, err := forge.ParseRepo(cfg.Config["repo"]); err == nil {
		return repo.Path
	}
	return cfg.Config["repo"]
}
