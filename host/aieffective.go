package host

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	appconfig "github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// A layered merge decides which provider and model an AI step uses:
// `buildFlowTools` → `applyBindings` → the tool registry's config
// preprocessor → the tool's own fallback. The merge applies the precedence
// during a run, so its answer exists only once a run is under way.
//
// EffectiveAIModel states the same precedence once, as a query. Both the CLI and
// Kapi Desktop read it to display the active provider/model, so what the UI
// promises and what the run does cannot drift apart.

// AIModelOrigin says which layer supplied the effective value.
type AIModelOrigin string

const (
	// AIModelFromLocalePreset is `defaults.locales.<lang>.tools.<tool>` in the recipe.
	AIModelFromLocalePreset AIModelOrigin = "locale-preset"
	// AIModelFromProjectPreset is `defaults.tools.<tool>` in the recipe.
	AIModelFromProjectPreset AIModelOrigin = "project-preset"
	// AIModelFromAppConfig is the stored `ai.provider` / `ai.model` (the file at
	// config.GlobalConfigFilePath, or the KAPI_AI_* environment overrides), which
	// is what `kapi models setup` and `kapi models default` write.
	AIModelFromAppConfig AIModelOrigin = "app-config"
	// AIModelFromBuiltIn means nothing is configured; the tool's own default applies.
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
	// Configured is false when neither half is configured anywhere: the
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

	// Lowest configured layer first, so each higher layer overwrites and
	// records itself, and the precedence reads top-to-bottom in one direction.
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

// AISetupKind says what a flow needs before its AI steps can run.
type AISetupKind string

const (
	// AISetupNone means every AI step's credentials resolve.
	AISetupNone AISetupKind = ""
	// AISetupChooseModel means an AI step names no provider and the stored
	// default does not resolve to a usable one. Choosing a model, which sets
	// the stored default, fixes it.
	AISetupChooseModel AISetupKind = "choose_model"
	// AISetupMissingCredential means the recipe names a provider (or a saved
	// credential) for an AI step and no credential for it resolves. A stored
	// default never overrides a named provider, so only a credential for that
	// provider fixes it.
	AISetupMissingCredential AISetupKind = "missing_credential"
)

// AISetupNeed is what running a flow needs before its AI steps can run.
type AISetupNeed struct {
	Kind AISetupKind `json:"kind"`
	// Flow and Tool name the step a missing credential belongs to.
	Flow string `json:"flow,omitempty"`
	Tool string `json:"tool,omitempty"`
	// Provider is the provider the recipe names, for a missing credential.
	Provider string `json:"provider,omitempty"`
	// Message is the error the run would fail with, for a missing credential:
	// the same text `kapi up` prints.
	Message string `json:"message,omitempty"`
}

// Err returns a missing credential as the error a run reports, or nil for
// any other kind.
func (n AISetupNeed) Err() error {
	if n.Kind != AISetupMissingCredential {
		return nil
	}
	return fmt.Errorf("flow %q: tool %q: %s", n.Flow, n.Tool, n.Message)
}

// ProjectFlowAISetup reports what running a flow needs before its AI steps
// can run, after the recipe and locale presets and the stored default apply.
// A step on a keyless provider, or on a provider with a usable credential
// (inline, named, in the environment or the single saved match), needs
// nothing. A missing credential for a provider the recipe names outranks a
// model choice, because no model choice can fix it. An empty flow name
// selects convergence. The CLI (`kapi up`) and Kapi Desktop both ask this
// function, so they prompt, or refuse, in the same cases.
func (a *App) ProjectFlowAISetup(proj *project.KapiProject, projectPath, flowName string) (AISetupNeed, error) {
	if proj == nil {
		return AISetupNeed{}, nil
	}
	root := filepath.Dir(projectPath)
	var spec *flow.StepsSpec
	label := flowName
	if flowName == "" {
		if !proj.DeclaresTargetLanguages() || !proj.WritesTargets() {
			return AISetupNeed{}, nil
		}
		resolved, err := convergeFlowSpec(proj, root)
		if err != nil {
			return AISetupNeed{}, err
		}
		spec, label = resolved.spec, resolved.name
	} else {
		resolved, err := ResolveProjectFlow(proj, root, flowName, nil)
		if err != nil || resolved == nil {
			return AISetupNeed{}, err
		}
		spec = resolved.Spec
	}
	a.InitRegistries()
	locales := []model.LocaleID{""}
	if len(proj.Defaults.Locales) > 0 {
		files, err := project.NewProjectContext(proj, projectPath).ResolveContent(a.FormatReg)
		if err != nil {
			return AISetupNeed{}, err
		}
		if targets := contentTargetLocales(proj.Defaults, files); len(targets) > 0 {
			locales = targets
		}
	}
	bindings := recipePresets(proj)
	need := AISetupNeed{}
	for _, step := range spec.Steps {
		name := registry.ToolID(step.Tool)
		for _, locale := range locales {
			cfg := a.applyBindingsFor(bindings, step.Tool, a.ToolReg.Schema(name), step.Config, string(locale))
			info := a.ToolReg.ResolveToolInfo(name, cfg)
			if info == nil || !slices.Contains(info.Requires, schema.RequiresCredentials) {
				continue
			}
			err := a.stepMissingCredential(string(name), info.Requires, cfg)
			if err == nil {
				continue
			}
			named := presetString(cfg, "provider") != "" || presetString(cfg, "credential") != ""
			if !named {
				need.Kind = AISetupChooseModel
				continue
			}
			return AISetupNeed{
				Kind:     AISetupMissingCredential,
				Flow:     label,
				Tool:     string(name),
				Provider: presetString(cfg, "provider"),
				Message:  err.Error(),
			}, nil
		}
	}
	return need, nil
}

// stepMissingCredential returns the error an AI step's credential resolution
// would fail with, or nil when it resolves the way the run resolves it: the
// stored ai.provider/ai.model default fills a step that names no provider
// (appconfig.ApplyAIToolDefaults), then credentials.MissingCredential applies
// the order of credentials.ResolveCredentials. A keyless provider (demo,
// ollama, claude-code) always resolves. Nothing is read from the keychain.
func (a *App) stepMissingCredential(toolName string, requires []string, cfg map[string]any) error {
	// ApplyAIToolDefaults writes into the map it is given, and cfg can be the
	// recipe's own step config.
	cfg = appconfig.ApplyAIToolDefaults(a.Config, toolName, requires, maps.Clone(cfg))
	return credentials.MissingCredential(a.Credentials, toolName, requires, cfg)
}
