package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

const cognitiveServicesScope = "https://cognitiveservices.azure.com/.default"

// PlatformProviderConfig configures the provider used for "platform" translation
// jobs — the jobs created by the built-in auto-translate-on-push automation,
// where the job carries no per-workspace credential (ProviderConfigID is empty
// or "platform").
//
// Two modes are supported:
//
//   - Azure OpenAI via managed identity (the hosted Bowrain cloud): set Endpoint
//     (and optionally ClientID). Authentication uses the ambient managed identity,
//     so no API key is stored.
//   - A generic AI provider (self-hosted / local-dev / bring-your-own key): set
//     Provider (e.g. "gemini", "openai", "anthropic", "ollama", "demo") and APIKey.
//     This lets a self-hosted or local worker translate against any supported
//     upstream with a plain API key — no Azure dependency.
//
// When Provider is non-empty it takes precedence over the Azure endpoint path.
type PlatformProviderConfig struct {
	// Azure OpenAI (managed identity) — hosted Bowrain cloud.
	Endpoint string // Azure OpenAI endpoint URL
	ClientID string // User-assigned managed identity client ID (optional)

	// Generic provider — self-hosted / local-dev / BYO key.
	Provider string // aiprovider ID: "gemini", "openai", "anthropic", "ollama", "demo"
	APIKey   string // API key for the generic provider (empty for keyless providers like ollama/demo)
	Model    string // default model for the generic provider; wins over the job's model when set
	BaseURL  string // optional base URL override (self-hosted endpoint / proxy)
}

// build returns the LLMProvider for a platform translation job plus the provider
// type string used for rate limiting. jobModel is the model requested by the job
// (set by the auto-translate automation). For the generic provider path the
// operator-configured Model wins when set, because the automation's default model
// is Azure/OpenAI-centric and may not name a valid model for other providers.
func (c PlatformProviderConfig) build(jobModel string) (aiprovider.LLMProvider, string, error) {
	if c.Provider != "" {
		model := c.Model
		if model == "" {
			model = jobModel
		}
		prov, err := aiprovider.NewProvider(aiprovider.ProviderID(c.Provider), aiprovider.Config{
			APIKey:  c.APIKey,
			Model:   model,
			BaseURL: c.BaseURL,
		})
		if err != nil {
			return nil, "", fmt.Errorf("build platform provider %q: %w", c.Provider, err)
		}
		return prov, c.Provider, nil
	}

	if c.Endpoint != "" {
		prov, err := NewPlatformProvider(c, jobModel)
		if err != nil {
			return nil, "", err
		}
		return prov, "azureopenai", nil
	}

	return nil, "", errors.New("platform provider not configured")
}

// NewPlatformProvider creates an Azure OpenAI provider that authenticates
// using a managed identity. The deployment name selects which model to use
// (e.g. "gpt-4o", "gpt-4o-mini").
func NewPlatformProvider(cfg PlatformProviderConfig, deployment string) (aiprovider.LLMProvider, error) {
	var cred *azidentity.ManagedIdentityCredential
	var err error

	if cfg.ClientID != "" {
		cred, err = azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
			ID: azidentity.ClientID(cfg.ClientID),
		})
	} else {
		cred, err = azidentity.NewManagedIdentityCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create managed identity credential: %w", err)
	}

	tp := func(ctx context.Context) (string, error) {
		token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{cognitiveServicesScope},
		})
		if err != nil {
			return "", fmt.Errorf("get cognitive services token: %w", err)
		}
		return token.Token, nil
	}

	if deployment == "" {
		deployment = "gpt-4o"
	}

	return aiprovider.NewAzureOpenAITokenProvider(cfg.Endpoint, deployment, tp), nil
}
