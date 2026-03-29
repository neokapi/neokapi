package backend

import (
	"fmt"
)

// ProviderInfo is the frontend-facing provider summary (no API key).
type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
}

// ProviderSaveRequest is sent from the frontend to save a provider with its key.
type ProviderSaveRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
}

// ListProviders returns all stored provider configs (no keys).
func (a *App) ListProviders() []ProviderInfo {
	if a.credentials == nil {
		return nil
	}
	var infos []ProviderInfo
	for _, c := range a.credentials.List() {
		infos = append(infos, ProviderInfo{
			ID:           c.ID,
			Name:         c.Name,
			ProviderType: c.ProviderType,
			Model:        c.Model,
			BaseURL:      c.BaseURL,
		})
	}
	return infos
}

// SaveProvider saves a provider config and optionally stores the API key in the OS keychain.
func (a *App) SaveProvider(req ProviderSaveRequest) (*ProviderInfo, error) {
	if a.credentials == nil {
		return nil, fmt.Errorf("credential store not initialized")
	}

	cfg := a.credentials.Upsert(ProviderConfig{
		ID:           req.ID,
		Name:         req.Name,
		ProviderType: req.ProviderType,
		Model:        req.Model,
		BaseURL:      req.BaseURL,
	})

	if req.APIKey != "" {
		if err := a.credentials.SetAPIKey(cfg.ID, req.APIKey); err != nil {
			return nil, fmt.Errorf("save API key: %w", err)
		}
	}

	return &ProviderInfo{
		ID:           cfg.ID,
		Name:         cfg.Name,
		ProviderType: cfg.ProviderType,
		Model:        cfg.Model,
		BaseURL:      cfg.BaseURL,
	}, nil
}

// DeleteProvider removes a provider config and its API key.
func (a *App) DeleteProvider(id string) error {
	if a.credentials == nil {
		return fmt.Errorf("credential store not initialized")
	}
	_ = a.credentials.DeleteAPIKey(id) // ignore keychain errors
	return a.credentials.Remove(id)
}

// TestProvider verifies that a provider's API key is accessible from the keychain.
func (a *App) TestProvider(id string) (bool, error) {
	if a.credentials == nil {
		return false, fmt.Errorf("credential store not initialized")
	}
	_, err := a.credentials.Get(id)
	if err != nil {
		return false, err
	}
	key, err := a.credentials.GetAPIKey(id)
	if err != nil {
		return false, fmt.Errorf("API key not found in keychain: %w", err)
	}
	return len(key) > 0, nil
}
