package backend

import (
	"errors"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/credentials"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ProviderInfo is the frontend-facing provider summary (no API key).
type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	// Default marks the credential used when its provider has more than one saved.
	Default bool `json:"default,omitempty"`
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
		infos = append(infos, providerInfoFrom(c))
	}
	return infos
}

// SaveProvider saves a provider config and optionally stores the API key in the
// OS keychain. Validation (a rejected typo'd provider type) and the key write
// are the shared host path, so the desktop cannot persist a provider the CLI
// would refuse.
func (a *App) SaveProvider(req ProviderSaveRequest) (*ProviderInfo, error) {
	if a.credentials == nil {
		return nil, errors.New("credential store not initialized")
	}

	cfg, err := host.SaveCredential(a.credentials, credentials.ProviderConfig{
		ID:           req.ID,
		Name:         req.Name,
		ProviderType: req.ProviderType,
		Model:        req.Model,
		BaseURL:      req.BaseURL,
	}, req.APIKey)
	if err != nil {
		return nil, err
	}

	info := providerInfoFrom(cfg)
	return &info, nil
}

// SetProviderDefault marks a credential as the default for its provider type
// (clearing the flag on the provider's other keys). Used when a provider has
// more than one saved key so a run resolves deterministically.
func (a *App) SetProviderDefault(id string) error {
	if a.credentials == nil {
		return errors.New("credential store not initialized")
	}
	return a.credentials.SetDefault(id)
}

// DeleteProvider removes a provider config and its API key.
func (a *App) DeleteProvider(id string) error {
	if a.credentials == nil {
		return errors.New("credential store not initialized")
	}
	return host.RemoveCredential(a.credentials, id)
}

// TestProvider verifies that a provider is usable. Keyless providers (on-device
// ones plus subscription-backed claude-code) pass once the credential record
// exists; cloud providers still require a key in the keychain. The keyless
// decision is the shared host path, so the desktop and the CLI agree.
func (a *App) TestProvider(id string) (bool, error) {
	if a.credentials == nil {
		return false, errors.New("credential store not initialized")
	}
	cfg, err := a.credentials.Get(id)
	if err != nil {
		return false, err
	}
	return host.TestCredential(a.credentials, cfg)
}

// ProviderTypeInfo describes an available AI provider type for the frontend.
type ProviderTypeInfo struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	// Local is true for on-device providers (Ollama, Gemma, Demo) that need no
	// API key; the frontend uses it to hide the API-key field and show a badge.
	Local bool `json:"local"`
	// Keyless is true when the provider needs no API key at all — local ones
	// plus subscription-backed claude-code. Drives hiding the key field.
	Keyless bool `json:"keyless"`
	// Subscription is true when usage bills a personal subscription
	// (claude-code) rather than metered API usage.
	Subscription bool `json:"subscription,omitempty"`
}

// ListProviderTypes returns the canonical list of available AI provider types.
func (a *App) ListProviderTypes() []ProviderTypeInfo {
	providers := aiprovider.Providers()
	out := make([]ProviderTypeInfo, len(providers))
	for i, p := range providers {
		out[i] = ProviderTypeInfo{
			Name:         string(p.Name),
			Label:        p.Label,
			Local:        aiprovider.IsLocalProvider(p.Name),
			Keyless:      !p.NeedsKey(),
			Subscription: p.Subscription,
		}
	}
	return out
}

func providerInfoFrom(c credentials.ProviderConfig) ProviderInfo {
	return ProviderInfo{
		ID:           c.ID,
		Name:         c.Name,
		ProviderType: c.ProviderType,
		Model:        c.Model,
		BaseURL:      c.BaseURL,
		Default:      c.Default,
	}
}
