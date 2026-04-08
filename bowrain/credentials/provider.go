package credentials

import (
	"fmt"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// NewProvider creates an LLMProvider from a saved config by looking up the API key in the keychain.
func NewProvider(store *Store, configID string) (aiprovider.LLMProvider, error) {
	cfg, err := store.Get(configID)
	if err != nil {
		return nil, fmt.Errorf("get provider config %q: %w", configID, err)
	}

	apiKey, err := store.GetAPIKey(configID)
	if err != nil {
		// Ollama doesn't require an API key, so continue with empty string
		if cfg.ProviderType != string(aiprovider.Ollama) {
			return nil, fmt.Errorf("get API key for %q: %w", cfg.Name, err)
		}
	}

	return NewProviderFromConfig(cfg, apiKey), nil
}

// NewProviderFromConfig creates an LLMProvider from an explicit config and API key.
// Uses the global provider registry, so plugin-provided providers are supported.
func NewProviderFromConfig(cfg ProviderConfig, apiKey string) aiprovider.LLMProvider {
	pcfg := aiprovider.Config{
		APIKey:  apiKey,
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
	}
	provider, err := aiprovider.NewProvider(aiprovider.ProviderID(cfg.ProviderType), pcfg)
	if err != nil {
		// Fall back to mock for unknown providers (preserves existing behavior).
		return aiprovider.NewMockProvider()
	}
	return provider
}
