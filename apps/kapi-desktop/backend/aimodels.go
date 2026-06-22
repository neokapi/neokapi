package backend

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	appconfig "github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/registry"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// DefaultModelInfo is the configured default AI model and the provider it
// implies — the shared ai.provider/ai.model convention both kapi surfaces read.
type DefaultModelInfo struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// AIModelOption is one selectable model for the "AI Models" picker. The list is
// model-first: the user chooses a Model and Provider follows.
type AIModelOption struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Label    string `json:"label"` // provider display label (e.g. "Ollama")
	// Local is true for on-device providers (Ollama) that need no API key.
	Local bool `json:"local"`
	// Installed (local only) reports the model is already present on the Ollama
	// server; not-installed local models are pulled on first use.
	Installed bool `json:"installed"`
	// NeedsKey is true for a cloud model with no saved credential yet.
	NeedsKey bool `json:"needs_key"`
	// Note is an optional one-line rationale (recommended local models).
	Note string `json:"note,omitempty"`
	// IsDefault marks the currently configured default.
	IsDefault bool `json:"is_default"`
}

// GetDefaultModel returns the configured default AI provider+model (empty when
// none is set — AI tools then fall back to the built-in provider default).
func (a *App) GetDefaultModel() DefaultModelInfo {
	if a.aiConfig == nil {
		return DefaultModelInfo{}
	}
	return DefaultModelInfo{
		Provider: a.aiConfig.GetString(appconfig.KeyAIProvider),
		Model:    a.aiConfig.GetString(appconfig.KeyAIModel),
	}
}

// SetDefaultModel persists the default AI model (and the provider it implies) to
// the shared kapi config, so the kapi CLI honors it too. When provider is empty
// it is inferred from the model name (convention over configuration). Pass an
// empty model to clear the default.
func (a *App) SetDefaultModel(model, provider string) error {
	model = strings.TrimSpace(model)
	provider = strings.TrimSpace(provider)

	if model == "" {
		return a.writeDefaultModel("", "")
	}
	if provider == "" {
		inferred, ok := aiprovider.ProviderForModel(model)
		if !ok {
			return fmt.Errorf("could not infer a provider for model %q; choose a provider explicitly", model)
		}
		provider = string(inferred)
	}
	return a.writeDefaultModel(provider, model)
}

// writeDefaultModel persists ai.provider/ai.model to the global config file and
// mirrors the values into the in-memory config the preprocessor reads.
func (a *App) writeDefaultModel(provider, model string) error {
	if err := appconfig.SetGlobalConfig(appconfig.KeyAIProvider, provider); err != nil {
		return fmt.Errorf("set %s: %w", appconfig.KeyAIProvider, err)
	}
	if err := appconfig.SetGlobalConfig(appconfig.KeyAIModel, model); err != nil {
		return fmt.Errorf("set %s: %w", appconfig.KeyAIModel, err)
	}
	if a.aiConfig != nil {
		a.aiConfig.Set(appconfig.KeyAIProvider, provider)
		a.aiConfig.Set(appconfig.KeyAIModel, model)
	}
	return nil
}

// ListAIModels returns the model-first catalog for the picker: local Ollama
// models (installed first, then recommended) followed by each cloud provider's
// model (flagged when no credential is saved yet).
func (a *App) ListAIModels() []AIModelOption {
	defProvider, defModel := "", ""
	if a.aiConfig != nil {
		defProvider = a.aiConfig.GetString(appconfig.KeyAIProvider)
		defModel = a.aiConfig.GetString(appconfig.KeyAIModel)
	}
	isDefault := func(provider, model string) bool {
		return strings.EqualFold(provider, defProvider) && strings.EqualFold(model, defModel)
	}

	var out []AIModelOption

	// Local · Ollama — installed models (best-effort; server may be down).
	installed := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	models, err := aiprovider.NewOllamaManager("").List(ctx)
	cancel()
	if err == nil {
		for _, m := range models {
			installed[m.Name] = true
			out = append(out, AIModelOption{
				Model: m.Name, Provider: string(aiprovider.Ollama), Label: "Ollama",
				Local: true, Installed: true, IsDefault: isDefault(string(aiprovider.Ollama), m.Name),
			})
		}
	}
	// Recommended local picks not already installed (pulled on first use).
	for _, r := range aiprovider.RecommendedOllamaModels {
		if installed[r.Name] {
			continue
		}
		out = append(out, AIModelOption{
			Model: r.Name, Provider: string(aiprovider.Ollama), Label: "Ollama",
			Local: true, Note: r.Note, IsDefault: isDefault(string(aiprovider.Ollama), r.Name),
		})
	}

	// Cloud providers — one default model each, flagged when no key is saved.
	for _, p := range aiprovider.Providers() {
		if p.Local || p.DefaultModel == "" {
			continue
		}
		needsKey := a.credentials == nil || len(a.credentials.FindByType(string(p.Name))) == 0
		out = append(out, AIModelOption{
			Model: p.DefaultModel, Provider: string(p.Name), Label: p.Label,
			NeedsKey: needsKey, IsDefault: isDefault(string(p.Name), p.DefaultModel),
		})
	}
	return out
}

// AINeedsModelChoice reports whether running a flow should prompt the user to
// pick an AI model first: the flow uses a provider-backed AI tool, no default
// provider is configured, and credential auto-detect wouldn't resolve on its
// own (zero or several saved credentials). The frontend checks this before
// launching so the user chooses up front instead of hitting a run error.
func (a *App) AINeedsModelChoice(tabID, flowName string) bool {
	op := a.getOpenProject(tabID)
	if op == nil {
		return false
	}
	spec := op.Project.Flow(flowName)
	if spec == nil {
		return false
	}
	info := flow.BuildToolInfoMap(a.toolReg)
	usesAI := false
	for _, step := range spec.Steps {
		if slices.Contains(info[registry.ToolID(step.Tool)].Requires, "credentials") {
			usesAI = true
			break
		}
	}
	if !usesAI {
		return false
	}
	// A configured default resolves everything.
	if a.aiConfig != nil && a.aiConfig.GetString(appconfig.KeyAIProvider) != "" {
		return false
	}
	// No default: a single saved credential auto-detects fine; zero or several
	// need an explicit choice.
	if a.credentials != nil && len(a.credentials.List()) == 1 {
		return false
	}
	return true
}
