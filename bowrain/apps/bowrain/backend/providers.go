package backend

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/ai/provider"
	"github.com/gokapi/gokapi/bowrain/credentials"
)

// ProviderConfigInfo is the frontend-facing provider config (no API key).
type ProviderConfigInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
}

// SaveProviderRequest is used to create/update a provider with an optional API key.
type SaveProviderRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
}

func toProviderConfigInfo(c credentials.ProviderConfig) ProviderConfigInfo {
	return ProviderConfigInfo{
		ID:           c.ID,
		Name:         c.Name,
		ProviderType: c.ProviderType,
		Model:        c.Model,
		BaseURL:      c.BaseURL,
	}
}

func (r SaveProviderRequest) toCredentials() credentials.ProviderConfig {
	return credentials.ProviderConfig{
		ID:           r.ID,
		Name:         r.Name,
		ProviderType: r.ProviderType,
		Model:        r.Model,
		BaseURL:      r.BaseURL,
	}
}

// ListProviderConfigs returns all saved AI provider configurations.
func (a *App) ListProviderConfigs() []ProviderConfigInfo {
	configs := a.credentials.List()
	out := make([]ProviderConfigInfo, len(configs))
	for i, c := range configs {
		out[i] = toProviderConfigInfo(c)
	}
	return out
}

// SaveProviderConfig creates or updates a provider configuration.
// If the API key is non-empty, it is stored in the OS keychain.
func (a *App) SaveProviderConfig(req SaveProviderRequest) (*ProviderConfigInfo, error) {
	saved := a.credentials.Upsert(req.toCredentials())

	if req.APIKey != "" {
		if err := a.credentials.SetAPIKey(saved.ID, req.APIKey); err != nil {
			return nil, fmt.Errorf("save API key: %w", err)
		}
	}

	result := toProviderConfigInfo(saved)
	return &result, nil
}

// DeleteProviderConfig removes a provider configuration and its API key.
func (a *App) DeleteProviderConfig(id string) error {
	if err := a.credentials.Remove(id); err != nil {
		return err
	}
	// Best-effort delete from keyring (may not exist)
	_ = a.credentials.DeleteAPIKey(id)
	return nil
}

// TestProviderConfig tests a provider configuration by sending a simple chat message.
func (a *App) TestProviderConfig(req SaveProviderRequest) error {
	cfg := req.toCredentials()
	p := credentials.NewProviderFromConfig(cfg, req.APIKey)
	defer p.Close()

	_, err := p.Chat(context.Background(), []provider.Message{
		{Role: "user", Content: "Hello, respond with OK."},
	})
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}
