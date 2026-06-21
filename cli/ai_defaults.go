package cli

import (
	"slices"
	"strings"

	"github.com/neokapi/neokapi/cli/config"
)

// applyAIDefaults fills in the default AI provider/model (from app config:
// ai.provider / ai.model) for provider-backed AI tools when the caller set
// neither a flag, inline config, nor a recipe default. It is the single
// chokepoint that lets `kapi ai-translate` (no --provider) use the user's
// configured default — e.g. the local "ollama" — instead of the built-in
// anthropic, and it covers both standalone tool runs and flows.
//
// Precedence is preserved by only filling absent keys: an explicit flag/inline
// value or a recipe default is already in the config map by the time this runs,
// so it is never overridden. MT tools (named "<provider>-translate") encode
// their provider in the tool name and are left untouched. The model default is
// applied only alongside a defaulted provider, so a stored ai.model never
// attaches to an explicitly chosen different provider.
func applyAIDefaults(cfg *config.AppConfig, toolName string, requires []string, c map[string]any) map[string]any {
	if cfg == nil || !slices.Contains(requires, "credentials") || isMTTool(toolName) {
		return c
	}
	if _, ok := c["provider"]; ok {
		return c // explicit flag / inline / recipe default wins
	}
	prov := cfg.GetString(config.KeyAIProvider)
	if prov == "" {
		return c
	}
	if c == nil {
		c = map[string]any{}
	}
	c["provider"] = prov
	if _, ok := c["model"]; !ok {
		if m := cfg.GetString(config.KeyAIModel); m != "" {
			c["model"] = m
		}
	}
	return c
}

// isMTTool reports whether toolName is a machine-translation tool, which encodes
// its provider in the name ("deepl-translate"); "ai-translate" is the LLM tool
// and is NOT an MT tool.
func isMTTool(name string) bool {
	return name != "ai-translate" && strings.HasSuffix(name, "-translate")
}
