package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/host"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AIDetectionResult is the desktop-facing report of the AI options available
// on this machine — which keyless providers are present, which API keys are
// already in the environment, what is saved in the vault, and the configured
// default. It powers the first-open "Connect your AI" card and the Settings →
// AI Models "Detected" section.
type AIDetectionResult struct {
	// Detected lists the keyless providers found on this machine, best first.
	Detected []DetectedAIProvider `json:"detected"`
	// EnvKeyProviders are provider ids whose conventional *_API_KEY env var is
	// set (usable with no vault entry).
	EnvKeyProviders []string `json:"env_key_providers,omitempty"`
	// SavedCredentialProviders are provider ids with a key in the vault.
	SavedCredentialProviders []string `json:"saved_credential_providers,omitempty"`
	// DefaultProvider / DefaultModel are the configured shared default, if any.
	DefaultProvider string `json:"default_provider,omitempty"`
	DefaultModel    string `json:"default_model,omitempty"`
	// Configured is true when any provider is already usable without setup —
	// the first-open card only shows when this is false.
	Configured bool `json:"configured"`
}

// DetectedAIProvider is one keyless provider present on this machine.
type DetectedAIProvider struct {
	Provider string `json:"provider"` // provider id (e.g. "claude-code")
	Label    string `json:"label"`    // display name (e.g. "Claude Code")
	Model    string `json:"model"`    // default model selected with it
	// Detail is the one-line card subtitle (e.g. "signed in on this Mac ·
	// uses your Claude subscription").
	Detail string `json:"detail"`
	// Subscription is true when usage bills a personal subscription rather
	// than metered API usage (claude-code).
	Subscription bool `json:"subscription"`
}

// DetectAIProviders reports the machine's AI options via the shared CLI
// detection (one detection path for kapi and the desktop). Detection is
// instant and offline: Claude Code by binary presence (auth is verified at
// first call), Ollama by a bounded local port probe.
func (a *App) DetectAIProviders() AIDetectionResult {
	shared := host.App{Credentials: a.credentials, Config: a.aiConfig}
	det := shared.DetectAIOptions(context.Background(), "")

	res := AIDetectionResult{
		EnvKeyProviders:          det.EnvKeyProviders,
		SavedCredentialProviders: det.SavedCredentialProviders,
		DefaultProvider:          det.DefaultProvider,
		DefaultModel:             det.DefaultModel,
		Configured:               det.Configured(),
	}
	if det.ClaudeCode {
		res.Detected = append(res.Detected, DetectedAIProvider{
			Provider:     string(aiprovider.ClaudeCode),
			Label:        "Claude Code",
			Model:        aiprovider.DefaultClaudeCodeModel,
			Detail:       "signed in on this Mac · uses your Claude subscription",
			Subscription: true,
		})
	}
	if det.Ollama {
		res.Detected = append(res.Detected, DetectedAIProvider{
			Provider: string(aiprovider.Ollama),
			Label:    "Ollama",
			Model:    aiprovider.DefaultOllamaModel,
			Detail:   "running locally · content stays on this machine",
		})
	}
	return res
}

// SelectAIProvider persists providerID (with an optional model — the
// provider's default when empty) as the app-level default AI provider that
// the flow runner and review AI already read (shared ai.provider/ai.model).
// One click from a detected card → configured.
func (a *App) SelectAIProvider(providerID, model string) error {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return fmt.Errorf("provider id is required")
	}
	info, ok := aiprovider.ProviderInfoFor(aiprovider.ProviderID(providerID))
	if !ok {
		return fmt.Errorf("unknown AI provider: %s", providerID)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = info.DefaultModel
	}
	return a.writeDefaultModel(providerID, model)
}
