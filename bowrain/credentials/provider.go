package credentials

import (
	"fmt"

	"github.com/neokapi/neokapi/providers/ai"
)

// NewProvider creates an LLMProvider from a saved config by looking up the API key in the keychain.
func NewProvider(store *Store, configID string) (aiprovider.LLMProvider, error) {
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
func NewProviderFromConfig(cfg ProviderConfig, apiKey string) aiprovider.LLMProvider {
	pcfg := aiprovider.Config{
		APIKey:  apiKey,
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
	}
	switch cfg.ProviderType {
	case "anthropic":
		return aiprovider.NewAnthropicProvider(pcfg)
	case "openai":
		return aiprovider.NewOpenAIProvider(pcfg)
	case "azureopenai", "azure_openai":
		return aiprovider.NewAzureOpenAIProvider(pcfg)
	case "ollama":
		return aiprovider.NewOllamaProvider(pcfg)
	default:
		return aiprovider.NewMockProvider()
	}
}
