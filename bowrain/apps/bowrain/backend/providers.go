package backend

import "errors"

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

// ListProviderConfigs returns all saved AI provider configurations from the server.
func (a *App) ListProviderConfigs() ([]ProviderConfigInfo, error) {
	if !a.isConnected() {
		return nil, errors.New("AI provider management requires a server connection")
	}
	a.mu.RLock()
	ws := a.activeWS
	a.mu.RUnlock()
	return a.remote.ListProviderConfigs(ws)
}

// SaveProviderConfig creates or updates a provider configuration on the server.
func (a *App) SaveProviderConfig(req SaveProviderRequest) (*ProviderConfigInfo, error) {
	if !a.isConnected() {
		return nil, errors.New("AI provider management requires a server connection")
	}
	a.mu.RLock()
	ws := a.activeWS
	a.mu.RUnlock()
	return a.remote.SaveProviderConfig(ws, req)
}

// DeleteProviderConfig removes a provider configuration on the server.
func (a *App) DeleteProviderConfig(id string) error {
	if !a.isConnected() {
		return errors.New("AI provider management requires a server connection")
	}
	a.mu.RLock()
	ws := a.activeWS
	a.mu.RUnlock()
	return a.remote.DeleteProviderConfig(ws, id)
}

// TestProviderConfig tests a provider configuration via the server.
func (a *App) TestProviderConfig(req SaveProviderRequest) error {
	if !a.isConnected() {
		return errors.New("AI provider management requires a server connection")
	}
	a.mu.RLock()
	ws := a.activeWS
	a.mu.RUnlock()
	return a.remote.TestProviderConfig(ws, req)
}
