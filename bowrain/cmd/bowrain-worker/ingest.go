package main

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"

	bowconn "github.com/neokapi/neokapi/bowrain/connector"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/forge"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
)

// newIngestFetcher builds the worker's connector fetcher for durable forge-
// ingest jobs: the server-side connector registry (forge + remote, with the
// GitHub App wired when configured, mirroring bowrain-server) over a
// ConnectorService whose instances are created on demand from the persisted
// connector config. The server rehydrates every connector at boot; the worker
// instead instantiates lazily per job, so it never holds clones for
// connectors it will never ingest.
//
// The registry must stay the *server* one. The worker builds its connectors
// from the same persisted, tenant-supplied config the server does, so a
// registry that knew the local-filesystem connectors would reopen there
// exactly what RegisterServer closes on the server.
func newIngestFetcher(cs store.ContentStore, configs *bstore.ConnectorConfigStore) *workerConnectorFetcher {
	formatReg := registry.NewFormatRegistry()
	formats.RegisterAll(formatReg)
	connReg := platconn.NewRegistry()
	bowconn.RegisterServer(connReg, formatReg)

	// GitHub App (auth: app forge connectors). Env contract mirrors
	// bowrain-server: PEM text, or a file path under systemd/compose.
	appID := os.Getenv("GITHUB_APP_ID")
	appKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	if appKey == "" {
		if path := os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"); path != "" {
			if pemBytes, err := os.ReadFile(path); err != nil {
				slog.Error("GITHUB_APP_PRIVATE_KEY_FILE unreadable, app-mode forge ingest disabled", "error", err)
			} else {
				appKey = string(pemBytes)
			}
		}
	}
	if appID != "" || appKey != "" {
		app, err := forge.NewGitHubApp(appID, appKey, os.Getenv("GITHUB_APP_WEBHOOK_SECRET"))
		if err != nil {
			slog.Error("github app misconfigured, app-mode forge ingest disabled", "error", err)
		} else {
			bowconn.RegisterForgeApp(connReg, formatReg, app)
			slog.Info("github app enabled for forge ingest", "app_id", appID)
		}
	}

	return &workerConnectorFetcher{
		svc:     service.NewConnectorService(cs, connReg),
		configs: configs,
	}
}

// workerConnectorFetcher satisfies jobs.ConnectorFetcher: it instantiates the
// connector from its persisted (decrypted) config on first use, then delegates
// to ConnectorService.Fetch — the exact fetch+store the server runs. Instances
// are cached by the service across jobs; a config edit lands on the next
// process (matching the server, where an update re-instantiates via its own
// handler — the worker's cache is refreshed below on every job to keep the two
// from drifting).
type workerConnectorFetcher struct {
	svc     *service.ConnectorService
	configs *bstore.ConnectorConfigStore
}

func (f *workerConnectorFetcher) Fetch(ctx context.Context, workspaceID, connectorID, projectID string, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	// Re-instantiate from the persisted config on every job: an ingest is a
	// clone/pull anyway, so constructor cost is noise, and it guarantees the
	// job always runs against the config as currently stored (secrets edits,
	// branch changes) instead of a stale cached instance.
	cfg, err := f.configs.Get(ctx, workspaceID, connectorID)
	if err != nil {
		if errors.Is(err, bstore.ErrConnectorConfigNotFound) {
			// Fall back to a live instance if one exists (in-memory-only
			// deployments); otherwise surface the lookup failure.
			if _, lerr := f.svc.GetConnector(workspaceID, connectorID); lerr == nil {
				return f.svc.Fetch(ctx, workspaceID, connectorID, projectID, opts)
			}
		}
		return nil, err
	}
	config := map[string]string{}
	maps.Copy(config, cfg.Config)
	// Pin the persisted id so the instance is addressable under the same id
	// regardless of how the connector derives one (mirrors rehydration).
	config["id"] = cfg.ID
	_ = f.svc.RemoveConnector(workspaceID, cfg.ID)
	if _, err := f.svc.AddConnector(workspaceID, cfg.Type, config); err != nil {
		return nil, err
	}
	return f.svc.Fetch(ctx, workspaceID, connectorID, projectID, opts)
}

// Assert the fetcher satisfies the worker dependency at compile time.
var _ jobs.ConnectorFetcher = (*workerConnectorFetcher)(nil)
