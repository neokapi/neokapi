package host

import (
	"strings"

	"github.com/neokapi/neokapi/core/project"
	appconfig "github.com/neokapi/neokapi/host/config"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// Which provider and model an AI step will actually use is decided by a layered
// merge (`buildFlowTools` → `applyBindings` → the tool registry's config
// preprocessor → the tool's own fallback). That is the right place to *apply*
// the precedence, but it means the answer only exists once a run is under way —
// so a surface that wants to *tell* the user what is configured has, until now,
// had to re-derive it, and every re-derivation was a chance to disagree with the
// run.
//
// EffectiveAIModel states the same precedence once, as a query. Both the CLI and
// Kapi Desktop read it to display the active provider/model, so what the UI
// promises and what the run does cannot drift apart.

// AIModelOrigin says which layer supplied the effective value.
type AIModelOrigin string

const (
	// AIModelFromLocalePreset — `defaults.locales.<lang>.tools.<tool>` in the recipe.
	AIModelFromLocalePreset AIModelOrigin = "locale-preset"
	// AIModelFromProjectPreset — `defaults.tools.<tool>` in the recipe.
	AIModelFromProjectPreset AIModelOrigin = "project-preset"
	// AIModelFromAppConfig — the stored `ai.provider` / `ai.model` (the file at
	// config.GlobalConfigFilePath, or the KAPI_AI_* environment overrides), which
	// is what `kapi models setup` and `kapi models default` write.
	AIModelFromAppConfig AIModelOrigin = "app-config"
	// AIModelFromBuiltIn — nothing configured; the tool's own default applies.
	AIModelFromBuiltIn AIModelOrigin = "built-in"
)

// AIModelResolution is the provider/model an AI tool will run with, plus where
// each half came from.
type AIModelResolution struct {
	// Provider is the effective provider id ("anthropic", "ollama", …).
	Provider string `json:"provider"`
	// Model is the effective model name. Empty means "the provider's own
	// default", which is the honest answer when nothing pins a model.
	Model string `json:"model"`
	// ProviderOrigin / ModelOrigin name the layer each value came from, so a UI
	// can explain *why* this is active ("from your recipe", "from kapi models
	// default").
	ProviderOrigin AIModelOrigin `json:"provider_origin"`
	ModelOrigin    AIModelOrigin `json:"model_origin"`
	// Configured is false when neither half is configured anywhere — the
	// built-in fallback is carrying the run, which is worth surfacing rather
	// than presenting as a deliberate choice.
	Configured bool `json:"configured"`
}

// EffectiveAIModel reports the provider and model an AI tool will run with,
// applying the same precedence the run does:
//
//	per-locale recipe preset → project recipe preset → app config → built-in
//
// (A step's own inline config and an explicit CLI flag sit above all of these;
// they are per-invocation, so a standing "what is configured" answer cannot
// include them. Callers that hold one should report it in preference.)
//
// toolName is the AI tool the answer is about ("translate" for the default
// flow); locale is the target locale whose per-locale preset applies, and may be
// empty. proj may be nil (no project open), as may cfg.
func EffectiveAIModel(
	cfg *appconfig.AppConfig,
	proj *project.KapiProject,
	toolName, locale string,
) AIModelResolution {
	res := AIModelResolution{
		ProviderOrigin: AIModelFromBuiltIn,
		ModelOrigin:    AIModelFromBuiltIn,
	}

	// Lowest configured layer first, so each higher layer simply overwrites and
	// records itself — the precedence reads top-to-bottom in one direction.
	//
	// The app-config layer comes from appconfig.ResolveAIDefault, not from reading
	// ai.provider / ai.model here: this query and the run must agree about what is
	// stored, and they can only be guaranteed to agree by asking the same
	// function. (ResolveAIDefault also attributes KAPI_AI_PROVIDER /
	// KAPI_AI_MODEL explicitly, rather than leaving them to viper's
	// AutomaticEnv.)
	if def := appconfig.ResolveAIDefault(cfg); def.Any() {
		if def.Provider != "" {
			res.Provider, res.ProviderOrigin = def.Provider, AIModelFromAppConfig
		}
		if def.Model != "" {
			res.Model, res.ModelOrigin = def.Model, AIModelFromAppConfig
		}
	}

	if proj != nil {
		apply := func(preset map[string]any, origin AIModelOrigin) {
			if v := presetString(preset, "provider"); v != "" {
				res.Provider, res.ProviderOrigin = v, origin
			}
			if v := presetString(preset, "model"); v != "" {
				res.Model, res.ModelOrigin = v, origin
			}
		}
		apply(proj.Defaults.Tools[toolName], AIModelFromProjectPreset)
		if locale != "" {
			if ld, ok := proj.Defaults.Locales[locale]; ok {
				apply(ld.Tools[toolName], AIModelFromLocalePreset)
			}
		}
	}

	res.Configured = res.Provider != "" || res.Model != ""

	// Nothing pinned a provider: the tool falls back to its built-in default,
	// and the honest display is that default rather than a blank.
	if res.Provider == "" {
		res.Provider = string(aiprovider.Anthropic)
		res.ProviderOrigin = AIModelFromBuiltIn
	}
	return res
}

// presetString reads a string value out of a recipe tool preset. YAML decoding
// can land a scalar as a non-string, so anything that is not a string is ignored
// rather than coerced.
func presetString(preset map[string]any, key string) string {
	if preset == nil {
		return ""
	}
	s, ok := preset[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
