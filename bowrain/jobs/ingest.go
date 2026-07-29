package jobs

import (
	"context"
	"errors"
	"log/slog"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
)

// ForgeIngestItemName is the sentinel ItemName that marks a job row as a
// durable forge-ingest job rather than a translation. Like "__sync_push__",
// ingest jobs ride the existing translation job table + queue so they inherit
// the claim/lease/retry/sweeper machinery without a second job system.
const ForgeIngestItemName = "__forge_ingest__"

// NewForgeIngestJob builds the durable job for one webhook- or bind-triggered
// source ingest. Field reuse on TranslationJob (mirroring the sync-push
// precedent, which carries its manifest key in Model and its stream in
// TargetLocale):
//
//   - ItemName     — the ForgeIngestItemName sentinel (routes the worker)
//   - Model        — the connector id (the job's payload key)
//   - TargetLocale — the delivery source label ("forge-webhook", "github-setup")
//   - WorkspaceID  — the connector's workspace (scopes config + fetch)
//   - ProjectID    — the project the connector feeds
//
// Everything else the ingest needs (repo, branch, credentials) is re-read from
// the persisted connector config at execution time, so a config edit between
// enqueue and execution is honored rather than replayed stale.
func NewForgeIngestJob(workspaceID, connectorID, projectID, source string) *TranslationJob {
	return &TranslationJob{
		ID:           id.New(),
		WorkspaceID:  workspaceID,
		ProjectID:    projectID,
		ItemName:     ForgeIngestItemName,
		Model:        connectorID,
		TargetLocale: source,
		Status:       StatusQueued,
	}
}

// IsForgeIngest reports whether the job is a durable forge-ingest job.
func (j *TranslationJob) IsForgeIngest() bool { return j.ItemName == ForgeIngestItemName }

// IngestConnectorID returns the connector a forge-ingest job fetches from.
func (j *TranslationJob) IngestConnectorID() string { return j.Model }

// IngestSource returns the delivery source label recorded at enqueue
// ("forge-webhook", "github-setup").
func (j *TranslationJob) IngestSource() string { return j.TargetLocale }

// ConnectorFetcher performs a connector fetch and stores the fetched content
// into the project — the server's service.ConnectorService satisfies it
// directly; the worker wraps one with on-demand instantiation from the
// persisted config.
type ConnectorFetcher interface {
	Fetch(ctx context.Context, workspaceID, connectorID, projectID string, opts platconn.FetchOptions) ([]*platconn.ContentItem, error)
}

// ConnectorSyncRecorder records ingest outcomes on the persisted connector
// row, so the status path surfaces them. Satisfied by
// *bstore.ConnectorConfigStore. Both writes are best-effort: a recording
// failure never changes the ingest outcome.
type ConnectorSyncRecorder interface {
	TouchLastSync(ctx context.Context, workspaceID, configID string, t time.Time) error
	SetLastError(ctx context.Context, workspaceID, configID, msg string) error
}

// ConnectorConfigSource additionally reads the persisted (decrypted) connector
// config — the worker needs it to instantiate the connector and to derive the
// repo/branch for the push event. Satisfied by *bstore.ConnectorConfigStore.
type ConnectorConfigSource interface {
	ConnectorSyncRecorder
	Get(ctx context.Context, workspaceID, configID string, opts ...bstore.GetOption) (bstore.ConnectorConfig, error)
}

// ForgeIngestDeps is what one ingest execution needs. Recorder and EventBus
// are optional (nil-tolerated): without a recorder the outcome is only logged,
// without a bus no push event goes out (matching the previous in-process
// behavior on minimally-wired servers).
type ForgeIngestDeps struct {
	Fetcher  ConnectorFetcher
	Recorder ConnectorSyncRecorder
	EventBus platev.EventBus
}

// ForgeIngestParams identifies one ingest: which connector to fetch, into
// which project, and the repo/branch/source labels the push event carries.
type ForgeIngestParams struct {
	WorkspaceID string
	ConnectorID string
	ProjectID   string
	Repo        string
	Branch      string
	Source      string
}

// ForgeIngest is the shared fetch+ingest core, extracted from the server's
// former fire-and-forget forgeIngest goroutine so the durable worker path and
// the in-process fallback run the exact same logic. On success the connector's
// last-sync time is stamped (clearing any recorded error) and
// EventPushCompleted goes out — which starts a convergence run under the
// project's on-push policy. On failure the error is recorded on the connector
// row so the status path surfaces it, and returned for the caller's
// retry/failure bookkeeping. The connector stays bound either way.
//
// Idempotency: re-running the same ingest is naturally idempotent — the fetch
// re-reads the same source tree and StoreBlocks resolves each block to an
// existing row by the caller's id scoped to the item (source_id + item_name),
// so a duplicate delivery converges to the same stored state. Stored ids are
// minted by the store and are NOT content-derived: AD-036's content key is a
// separate concept, and reading this comment as "ids are the content hash"
// sent a debugging pass down the wrong path once (#1527). A duplicate
// EventPushCompleted at worst starts an extra
// convergence pass over already-converged state, the same outcome as a forge
// redelivering a webhook. This is what makes at-least-once delivery (broker
// redelivery, the stale-job sweeper) safe for ingest jobs.
func ForgeIngest(ctx context.Context, deps ForgeIngestDeps, p ForgeIngestParams) error {
	if deps.Fetcher == nil {
		return errors.New("forge ingest: no connector fetcher configured")
	}
	if _, err := deps.Fetcher.Fetch(ctx, p.WorkspaceID, p.ConnectorID, p.ProjectID, platconn.FetchOptions{}); err != nil {
		slog.WarnContext(ctx, "forge ingest: fetch failed",
			"source", p.Source, "connector", p.ConnectorID, "project", p.ProjectID, "error", err)
		if deps.Recorder != nil {
			if rerr := deps.Recorder.SetLastError(ctx, p.WorkspaceID, p.ConnectorID, err.Error()); rerr != nil {
				slog.WarnContext(ctx, "forge ingest: record last-error failed", "connector", p.ConnectorID, "error", rerr)
			}
		}
		return err
	}
	if deps.Recorder != nil {
		if err := deps.Recorder.TouchLastSync(ctx, p.WorkspaceID, p.ConnectorID, time.Now().UTC()); err != nil {
			slog.WarnContext(ctx, "forge ingest: record last-sync failed", "connector", p.ConnectorID, "error", err)
		}
	}
	if deps.EventBus == nil {
		return nil
	}
	deps.EventBus.Publish(platev.Event{
		ID:        id.New(),
		Type:      platev.EventPushCompleted,
		Source:    p.Source,
		ProjectID: p.ProjectID,
		Timestamp: time.Now().UTC(),
		Data: map[string]string{
			"connector_id": p.ConnectorID,
			"repo":         p.Repo,
			"branch":       p.Branch,
		},
	})
	return nil
}
