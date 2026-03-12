package credentials

import (
	"fmt"

	"github.com/neokapi/neokapi/providers/ai"
)

// NewProvider creates an LLMProvider from a saved config by looking up the API key in the keychain.
func NewProvider(store *Store, configID string) (provider.LLMProvider, error) {
	cfg, err := store.Get(configID)
	if err != nil {
		return nil, err
	}

	apiKey, err := store.GetAPIKey(configID)
	if err != nil {
		// Ollama doesn't require an API key, so continue with empty string
		if cfg.ProviderType != "ollama" {
			return nil, fmt.Errorf("get API key for %q: %w", cfg.Name, err)
		}
	}

	return NewProviderFromConfig(cfg, apiKey), nil
}

// NewProviderFromConfig creates an LLMProvider from an explicit config and API key.
func NewProviderFromConfig(cfg ProviderConfig, apiKey string) provider.LLMProvider {
	pcfg := provider.Config{
		APIKey:  apiKey,
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
	}
	switch cfg.ProviderType {
	case "anthropic":
		return provider.NewAnthropicProvider(pcfg)
	case "openai":
		return provider.NewOpenAIProvider(pcfg)
	case "azureopenai", "azure_openai":
		return provider.NewAzureOpenAIProvider(pcfg)
	case "ollama":
		return provider.NewOllamaProvider(pcfg)
	default:
		return provider.NewMockProvider()
	}
}
