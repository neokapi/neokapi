package config

import (
	"slices"
	"strings"
)

// ApplyAIToolDefaults fills in the default AI provider/model for
// provider-backed AI tools when the caller set neither a flag, inline config,
// nor a recipe default. It is the single chokepoint — shared by the kapi CLI and
// the Kapi Desktop backend — that lets `kapi translate` / a desktop flow run
// with no explicit provider use the configured default (e.g. the local
// "ollama"), so both surfaces honor the same stored setting.
//
// The default comes from [ResolveAIDefault], the one resolver; reading
// ai.provider / ai.model here directly is what let this path disagree with what
// `kapi models default` reported.
//
// Precedence is preserved by only filling absent keys: an explicit flag/inline
// value or a recipe default is already in the config map by the time this runs,
// so it is never overridden. An explicit provider short-circuits the whole
// function, so a stored ai.model never attaches to an explicitly chosen
// different provider. MT tools (named "<provider>-translate") encode their
// provider in the tool name and are left untouched.
func ApplyAIToolDefaults(cfg *AppConfig, toolName string, requires []string, c map[string]any) map[string]any {
	if cfg == nil || !slices.Contains(requires, "credentials") || isMTToolName(toolName) {
		return c
	}
	if _, ok := c["provider"]; ok {
		return c // explicit flag / inline / recipe default wins
	}
	def := ResolveAIDefault(cfg)
	if !def.Any() {
		return c
	}
	if c == nil {
		c = map[string]any{}
	}
	if def.Provider != "" {
		c["provider"] = def.Provider
	}
	if _, ok := c["model"]; !ok && def.Model != "" {
		c["model"] = def.Model
	}
	return c
}

// isMTToolName reports whether toolName is a machine-translation tool, which
// encodes its provider in the name ("<provider>-translate", as registered by
// MT plugins). "ai-translate" is the legacy LLM tool name and is explicitly
// NOT MT.
func isMTToolName(name string) bool {
	return name != "ai-translate" && strings.HasSuffix(name, "-translate")
}
