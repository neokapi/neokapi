package config

import (
	"fmt"
	"os"
	"strings"
)

// The default AI provider/model has exactly one resolver and one writer.
//
// Before this, every consumer read `ai.provider` / `ai.model` off an AppConfig
// itself — the CLI's tool-config preprocessor, the `up --plan` annotation, the
// Ollama pre-flight, the setup wizard's detection, `kapi models default`'s
// read-back, and the desktop's model panel. Six call sites, no shared answer to
// "what is configured, and where did it come from", so a scope bug in the
// reader (see NewAppConfig) went unnoticed in all six at once and there was
// nowhere to fix it but everywhere.
//
// ResolveAIDefault is that one answer, and SetAIDefault / ClearAIDefault are the
// one write path. Every surface — CLI, desktop, plugin — calls these.

// AIDefaultSource names where a resolved default came from.
type AIDefaultSource string

const (
	// AIDefaultNone means nothing is configured.
	AIDefaultNone AIDefaultSource = "none"
	// AIDefaultEnv means the value came from KAPI_AI_PROVIDER / KAPI_AI_MODEL.
	AIDefaultEnv AIDefaultSource = "env"
	// AIDefaultConfig means the value came from the global config file.
	AIDefaultConfig AIDefaultSource = "config"
)

// EnvAIProvider / EnvAIModel are the environment overrides for the default AI
// provider and model. They are the viper env-prefix spellings of the config
// keys, named here so a diagnostic can quote them.
const (
	EnvAIProvider = "KAPI_AI_PROVIDER"
	EnvAIModel    = "KAPI_AI_MODEL"
)

// AIDefault is the resolved default AI provider and model.
type AIDefault struct {
	// Provider is the provider id ("ollama", "anthropic", "claude-code", …), or
	// empty when none is configured.
	Provider string `json:"provider,omitempty"`
	// Model is the model id to use with Provider, or empty when unset (the
	// provider's own default then applies).
	Model string `json:"model,omitempty"`
	// Source names where Provider came from, so a message can be accurate about
	// what is configured and where to change it.
	Source AIDefaultSource `json:"source"`
	// File is the config file backing a Source of AIDefaultConfig.
	File string `json:"file,omitempty"`
}

// Configured reports whether a provider is set — the question a caller that
// needs to *name* a provider asks.
func (d AIDefault) Configured() bool { return d.Provider != "" }

// Any reports whether either half is set. A stored `ai.model` with no
// `ai.provider` is unusual but legal (a hand-edited config), and it still says
// something about what a run will do: the model applies and the provider falls
// back to the built-in default. Callers that merge halves ask Any; callers that
// must print a provider ask Configured.
func (d AIDefault) Any() bool { return d.Provider != "" || d.Model != "" }

// Describe renders a one-line, accurate account of the current default — for a
// diagnostic that must never claim nothing is configured when something is.
func (d AIDefault) Describe() string {
	if !d.Any() {
		return "no default AI provider configured"
	}
	var b strings.Builder
	if d.Provider == "" {
		b.WriteString("(no provider)")
	} else {
		b.WriteString(d.Provider)
	}
	if d.Model != "" {
		b.WriteString(" · ")
		b.WriteString(d.Model)
	}
	switch d.Source {
	case AIDefaultEnv:
		fmt.Fprintf(&b, " (from %s)", EnvAIProvider)
	case AIDefaultConfig:
		if d.File != "" {
			fmt.Fprintf(&b, " (from %s)", d.File)
		}
	}
	return b.String()
}

// ResolveAIDefault reports the effective default AI provider/model.
//
// The *value* is whatever cfg resolves, so this function adds no precedence of
// its own: AppConfig already layers an in-session override (the wizard's mirror)
// over KAPI_AI_* over the stored file over the built-in defaults, and a second
// opinion here could only disagree with the run. A nil cfg still resolves the
// environment, so a caller without a loaded config is not a special case.
//
// Source is attribution only, for a message that must say where to make a
// change. It reads "env" when the resolved value is exactly what KAPI_AI_* holds
// — which is either the truth or a tie between two places holding the same
// value, never a claim about a value that is not in effect.
func ResolveAIDefault(cfg *AppConfig) AIDefault {
	envProv := strings.TrimSpace(os.Getenv(EnvAIProvider))
	envModel := strings.TrimSpace(os.Getenv(EnvAIModel))
	if cfg == nil {
		if envProv == "" && envModel == "" {
			return AIDefault{Source: AIDefaultNone}
		}
		return AIDefault{Provider: envProv, Model: envModel, Source: AIDefaultEnv}
	}

	prov := strings.TrimSpace(cfg.GetString(KeyAIProvider))
	modl := strings.TrimSpace(cfg.GetString(KeyAIModel))
	if prov == "" && modl == "" {
		return AIDefault{Source: AIDefaultNone}
	}
	fromEnv := (prov != "" && prov == envProv) ||
		(prov == "" && modl != "" && modl == envModel)
	if fromEnv {
		return AIDefault{Provider: prov, Model: modl, Source: AIDefaultEnv}
	}
	return AIDefault{
		Provider: prov,
		Model:    modl,
		Source:   AIDefaultConfig,
		File:     GlobalConfigFilePath(),
	}
}

// SetAIDefault persists the default provider and model, and mirrors them into
// cfg when non-nil so the running process sees the change immediately (an inline
// setup wizard mid-`kapi up` must take effect without re-reading the file). An
// empty model clears the stored model rather than leaving a stale one attached
// to a newly chosen provider.
func SetAIDefault(cfg *AppConfig, provider, model string) error {
	if strings.TrimSpace(provider) == "" {
		return fmt.Errorf("set %s: provider is empty", KeyAIProvider)
	}
	if err := SetGlobalConfig(KeyAIProvider, provider); err != nil {
		return fmt.Errorf("set %s: %w", KeyAIProvider, err)
	}
	if model == "" {
		if _, err := UnsetGlobalConfig(KeyAIModel); err != nil {
			return fmt.Errorf("unset %s: %w", KeyAIModel, err)
		}
	} else if err := SetGlobalConfig(KeyAIModel, model); err != nil {
		return fmt.Errorf("set %s: %w", KeyAIModel, err)
	}
	if cfg != nil {
		cfg.Set(KeyAIProvider, provider)
		cfg.Set(KeyAIModel, model)
	}
	return nil
}

// ClearAIDefault removes the stored provider and model, restoring "nothing
// configured". It reports whether anything was stored.
func ClearAIDefault(cfg *AppConfig) (bool, error) {
	provCleared, err := UnsetGlobalConfig(KeyAIProvider)
	if err != nil {
		return false, fmt.Errorf("unset %s: %w", KeyAIProvider, err)
	}
	modelCleared, err := UnsetGlobalConfig(KeyAIModel)
	if err != nil {
		return false, fmt.Errorf("unset %s: %w", KeyAIModel, err)
	}
	if cfg != nil {
		cfg.Set(KeyAIProvider, "")
		cfg.Set(KeyAIModel, "")
	}
	return provCleared || modelCleared, nil
}
