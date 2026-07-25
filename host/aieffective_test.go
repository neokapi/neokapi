package host

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/project"
	appconfig "github.com/neokapi/neokapi/host/config"
)

// cfgWith builds an in-memory app config carrying the given ai.* keys — the same
// two keys `kapi models setup` / `kapi models default` and the desktop's model
// picker all write.
func cfgWith(provider, model string) *appconfig.AppConfig {
	c := appconfig.NewAppConfig()
	if provider != "" {
		c.Set(appconfig.KeyAIProvider, provider)
	}
	if model != "" {
		c.Set(appconfig.KeyAIModel, model)
	}
	return c
}

func TestEffectiveAIModel_NothingConfigured(t *testing.T) {
	res := EffectiveAIModel(nil, nil, "translate", "")
	assert.False(t, res.Configured)
	// Honest about the fallback rather than reporting a blank.
	assert.Equal(t, "anthropic", res.Provider)
	assert.Equal(t, AIModelFromBuiltIn, res.ProviderOrigin)
	assert.Empty(t, res.Model)
	assert.Equal(t, AIModelFromBuiltIn, res.ModelOrigin)
}

func TestEffectiveAIModel_FromAppConfig(t *testing.T) {
	res := EffectiveAIModel(cfgWith("ollama", "gemma4:e2b"), nil, "translate", "")
	assert.True(t, res.Configured)
	assert.Equal(t, "ollama", res.Provider)
	assert.Equal(t, "gemma4:e2b", res.Model)
	assert.Equal(t, AIModelFromAppConfig, res.ProviderOrigin)
	assert.Equal(t, AIModelFromAppConfig, res.ModelOrigin)
}

func TestEffectiveAIModel_RecipePresetBeatsAppConfig(t *testing.T) {
	proj := &project.KapiProject{
		Defaults: project.Defaults{
			Tools: map[string]map[string]any{
				"translate": {"provider": "anthropic", "model": "claude-sonnet-4-6"},
			},
		},
	}
	res := EffectiveAIModel(cfgWith("ollama", "gemma4:e2b"), proj, "translate", "")
	assert.Equal(t, "anthropic", res.Provider)
	assert.Equal(t, "claude-sonnet-4-6", res.Model)
	assert.Equal(t, AIModelFromProjectPreset, res.ProviderOrigin)
	assert.Equal(t, AIModelFromProjectPreset, res.ModelOrigin)
}

func TestEffectiveAIModel_LocalePresetBeatsProjectPreset(t *testing.T) {
	proj := &project.KapiProject{
		Defaults: project.Defaults{
			Tools: map[string]map[string]any{
				"translate": {"provider": "anthropic", "model": "claude-sonnet-4-6"},
			},
			Locales: map[string]project.LocaleDefaults{
				"nb": {Tools: map[string]map[string]any{
					"translate": {"model": "claude-opus-4-1"},
				}},
			},
		},
	}
	// The locale preset pins only the model, so the provider still comes from the
	// project preset — each half resolves on its own layer.
	res := EffectiveAIModel(cfgWith("ollama", "gemma4:e2b"), proj, "translate", "nb")
	assert.Equal(t, "anthropic", res.Provider)
	assert.Equal(t, AIModelFromProjectPreset, res.ProviderOrigin)
	assert.Equal(t, "claude-opus-4-1", res.Model)
	assert.Equal(t, AIModelFromLocalePreset, res.ModelOrigin)

	// A locale with no entry is unaffected.
	other := EffectiveAIModel(cfgWith("ollama", "gemma4:e2b"), proj, "translate", "de")
	assert.Equal(t, "claude-sonnet-4-6", other.Model)
	assert.Equal(t, AIModelFromProjectPreset, other.ModelOrigin)
}

func TestEffectiveAIModel_PresetForAnotherToolIsIgnored(t *testing.T) {
	proj := &project.KapiProject{
		Defaults: project.Defaults{
			Tools: map[string]map[string]any{
				"qa": {"provider": "openai", "model": "gpt-5"},
			},
		},
	}
	res := EffectiveAIModel(cfgWith("ollama", "gemma4:e2b"), proj, "translate", "")
	assert.Equal(t, "ollama", res.Provider)
	assert.Equal(t, "gemma4:e2b", res.Model)
	assert.Equal(t, AIModelFromAppConfig, res.ProviderOrigin)
}

func TestEffectiveAIModel_ModelOnlyConfigKeepsBuiltInProvider(t *testing.T) {
	res := EffectiveAIModel(cfgWith("", "claude-sonnet-4-6"), nil, "translate", "")
	assert.True(t, res.Configured)
	assert.Equal(t, "claude-sonnet-4-6", res.Model)
	assert.Equal(t, AIModelFromAppConfig, res.ModelOrigin)
	assert.Equal(t, "anthropic", res.Provider)
	assert.Equal(t, AIModelFromBuiltIn, res.ProviderOrigin)
}

func TestEffectiveAIModel_NonStringPresetValuesAreIgnored(t *testing.T) {
	proj := &project.KapiProject{
		Defaults: project.Defaults{
			Tools: map[string]map[string]any{
				"translate": {"provider": 42, "model": nil},
			},
		},
	}
	res := EffectiveAIModel(cfgWith("ollama", "gemma4:e2b"), proj, "translate", "")
	assert.Equal(t, "ollama", res.Provider, "a non-string preset must not overwrite a real value")
	assert.Equal(t, "gemma4:e2b", res.Model)
}
