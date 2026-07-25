package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/resilience/aiguard"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// PlatformProviderConfig configures the provider used for "platform" translation
// jobs — the jobs created by the built-in auto-translate-on-push automation,
// where the job carries no per-workspace credential (ProviderConfigID is empty
// or "platform").
//
// A generic AI provider is used (hosted platform, self-hosted, or local-dev):
// set Provider (e.g. "bedrock", "gemini", "openai", "anthropic", "ollama",
// "demo") and, for keyed providers, APIKey. This lets any worker translate
// against a supported upstream with a plain API key (or ambient cloud
// credentials for keyless providers like bedrock).
type PlatformProviderConfig struct {
	Provider string // aiprovider ID: "bedrock", "gemini", "openai", "anthropic", "ollama", "demo"
	APIKey   string // API key for the provider (empty for keyless providers like bedrock/ollama/demo)
	Model    string // default model for the provider; wins over the job's model when set
	BaseURL  string // optional base URL override (self-hosted endpoint / proxy)
	// ModelPinned marks Model as an explicit per-workspace choice (customer
	// model choice applied a valid workspace preference), as opposed to the
	// platform default. A pinned model is never overridden by a measured model
	// recommendation (EV-4): the workspace's own pick always wins.
	ModelPinned bool
}

// PlatformResolver returns the current platform provider configuration for a
// given workspace. When a worker deps carries one, it is consulted at job time
// (rather than reading a value fixed at startup), so a runtime config change —
// e.g. an admin switching the platform AI provider or default model in ctrl —
// takes effect on the next job without restarting the worker. The workspaceID
// lets the resolver apply a per-workspace model choice when the platform admin
// has enabled it (an empty workspaceID, or a workspace with no preference, yields
// the platform default). A nil return falls back to the static Platform config on
// the deps.
type PlatformResolver func(ctx context.Context, workspaceID string) *PlatformProviderConfig

// activePlatform picks the resolver's current value for the given workspace when
// a resolver is present and returns non-nil; otherwise it returns the static
// config. Either result may be nil, meaning no platform provider is configured.
func activePlatform(ctx context.Context, workspaceID string, static *PlatformProviderConfig, resolver PlatformResolver) *PlatformProviderConfig {
	if resolver != nil {
		if c := resolver(ctx, workspaceID); c != nil {
			return c
		}
	}
	return static
}

// Build returns the LLMProvider for a platform translation job plus the provider
// type string used for rate limiting. jobModel is the model requested by the job
// (set by the auto-translate automation). For the generic provider path the
// operator-configured Model wins when set, because the automation's default model
// is Azure/OpenAI-centric and may not name a valid model for other providers.
//
// Exported so the server's synchronous editor platform path can resolve the same
// upstream as the async worker from the identical configuration.
func (c PlatformProviderConfig) Build(jobModel string) (aiprovider.LLMProvider, string, error) {
	if c.Provider != "" {
		model := c.Model
		if model == "" {
			model = jobModel
		}
		prov, err := aiguard.NewProvider(aiprovider.ProviderID(c.Provider), aiprovider.Config{
			APIKey:  c.APIKey,
			Model:   model,
			BaseURL: c.BaseURL,
		})
		if err != nil {
			return nil, "", fmt.Errorf("build platform provider %q: %w", c.Provider, err)
		}
		return prov, c.Provider, nil
	}

	return nil, "", errors.New("platform provider not configured")
}
