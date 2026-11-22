package server

import (
	"context"
	"errors"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// platformJobModel is the model a platform translation job requests when the
// project names none. The platform provider's own default model wins over it
// whenever ctrl sets one (jobs.PlatformProviderConfig.Build).
const platformJobModel = "gpt-4o-mini"

// workspaceProviderConfig returns the AI provider config a workspace has saved
// for its own key, when it has one. A workspace may save several; the one in
// use is the most recently updated (bstore.ProviderConfigStore.Latest), so
// saving or editing a config makes it the active one. The returned config
// carries no API key: the worker resolves the key by id at job time.
//
// A lookup error other than "none saved" is logged and treated as no config,
// so the work falls back to the platform provider rather than not running.
func (s *Server) workspaceProviderConfig(ctx context.Context, workspaceID string) (bstore.ProviderConfig, bool) {
	if workspaceID == "" {
		return bstore.ProviderConfig{}, false
	}
	lookup := s.latestProviderConfig
	if lookup == nil {
		if s.ProviderStore == nil {
			return bstore.ProviderConfig{}, false
		}
		lookup = s.ProviderStore.Latest
	}
	cfg, err := lookup(ctx, workspaceID)
	if err != nil {
		if !errors.Is(err, bstore.ErrProviderConfigNotFound) {
			slog.WarnContext(ctx, "workspace provider config lookup failed; using the platform provider",
				"workspace_id", workspaceID, "error", err)
		}
		return bstore.ProviderConfig{}, false
	}
	return cfg, true
}

// translationJobProvider picks the provider config id and model for the
// translation jobs automation and convergence create for a project. A
// workspace with a saved provider config gets that config and jobs.BYOModel.
// Otherwise the job runs on "platform" with the project's ai_model property,
// or platformJobModel.
func (s *Server) translationJobProvider(ctx context.Context, proj *store.Project) (providerConfigID, model string) {
	if cfg, ok := s.workspaceProviderConfig(ctx, proj.WorkspaceID); ok {
		return cfg.ID, jobs.BYOModel(cfg)
	}
	model = platformJobModel
	if proj.Properties != nil && proj.Properties["ai_model"] != "" {
		model = proj.Properties["ai_model"]
	}
	return "platform", model
}
