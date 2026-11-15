package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/time/rate"

	"github.com/neokapi/neokapi/bowrain/resilience/aiguard"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// WorkspaceProviderStore finds and resolves the AI provider configs a
// workspace has saved for its own key. *store.ProviderConfigStore satisfies it.
type WorkspaceProviderStore interface {
	ProviderConfigResolver
	// Latest returns the workspace's most recently updated config, without its
	// key, or store.ErrProviderConfigNotFound when it has saved none.
	Latest(ctx context.Context, workspaceID string) (bstore.ProviderConfig, error)
}

// BYOModel is the model work on a saved provider config uses: the config's own
// model when set, otherwise the provider's catalogued default model. An unknown
// provider type yields "", which leaves the provider to pick its default.
func BYOModel(cfg bstore.ProviderConfig) string {
	if cfg.Model != "" {
		return cfg.Model
	}
	if info, ok := aiprovider.ProviderInfoFor(aiprovider.ProviderID(cfg.Type)); ok {
		return info.DefaultModel
	}
	return ""
}

// workspaceExtractionProvider builds the provider for an extraction job from
// the workspace's most recently updated saved config. ok is false when the
// workspace has none (or no store is wired), and the job runs on the platform.
// A failed lookup is logged and also falls back to the platform; a config that
// is found but cannot be resolved or built is an error.
func workspaceExtractionProvider(ctx context.Context, deps *ExtractionWorkerDeps, job *ExtractionJob) (ResolvedProvider, bool, error) {
	if deps.ProviderStore == nil || job.WorkspaceID == "" {
		return ResolvedProvider{}, false, nil
	}
	latest, err := deps.ProviderStore.Latest(ctx, job.WorkspaceID)
	if err != nil {
		if !errors.Is(err, bstore.ErrProviderConfigNotFound) {
			slog.WarnContext(ctx, "extraction: workspace provider lookup failed; using the platform provider",
				"job_id", job.ID, "workspace_id", job.WorkspaceID, "error", err)
		}
		return ResolvedProvider{}, false, nil
	}
	cfg, err := deps.ProviderStore.Resolve(ctx, job.WorkspaceID, latest.ID)
	if err != nil {
		return ResolvedProvider{}, false, fmt.Errorf("resolve provider config %q: %w", latest.ID, err)
	}
	prov, err := aiguard.NewProvider(aiprovider.ProviderID(cfg.Type), aiprovider.Config{
		APIKey:  cfg.APIKey,
		Model:   BYOModel(cfg),
		BaseURL: cfg.BaseURL,
	})
	if err != nil {
		return ResolvedProvider{}, false, fmt.Errorf("build provider %q: %w", cfg.Type, err)
	}
	return ResolvedProvider{
		LLM:     prov,
		Limiter: rate.NewLimiter(providerRateLimit(cfg.Type), 1),
		Source:  ProviderSourceBYO,
	}, true, nil
}
