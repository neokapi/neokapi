package cli

import (
	"github.com/neokapi/neokapi/cli/config"
)

// applyAIDefaults fills in the default AI provider/model (ai.provider /
// ai.model) for provider-backed AI tools when the caller set neither a flag,
// inline config, nor a recipe default. It is a thin wrapper over the shared
// config.ApplyAIToolDefaults so the kapi CLI and the desktop apply identical
// defaulting from the same ~/.config/kapi/kapi.yaml.
func applyAIDefaults(cfg *config.AppConfig, toolName string, requires []string, c map[string]any) map[string]any {
	return config.ApplyAIToolDefaults(cfg, toolName, requires, c)
}
