package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/registry"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerPluginMT registers a demo MT engine the way a plugin would and
// removes it again at cleanup.
func registerPluginMT(t *testing.T, id mtprovider.ProviderID) {
	t.Helper()
	mtprovider.RegisterConfigFactory(id, func(_ mtprovider.MTConfig) mtprovider.MTProvider {
		return mtprovider.NewDemoProvider()
	})
	mttools.Providers = append(mttools.Providers, mttools.Provider{ID: id, Label: "Plugin Test MT"})
	t.Cleanup(func() {
		mttools.Providers = mttools.Providers[:len(mttools.Providers)-1]
	})
}

// TestNewTranslateFromConfig_RoutesByProvider pins the unified `translate`
// dispatch: no provider (or any LLM provider) builds the AI translator;
// a plugin-registered MT engine id builds the MT tool; an id known to
// neither registry fails with the unknown-provider error.
func TestNewTranslateFromConfig_RoutesByProvider(t *testing.T) {
	t.Run("default is the LLM translator", func(t *testing.T) {
		tl, err := tools.NewTranslateFromConfig(map[string]any{"apiKey": "test"}, "fr")
		require.NoError(t, err)
		_, isAI := tl.(*tools.AITranslateTool)
		assert.True(t, isAI, "no provider must default to the LLM translator")
		assert.Equal(t, "translate", tl.Name())
	})

	t.Run("explicit LLM provider stays on the LLM engine", func(t *testing.T) {
		tl, err := tools.NewTranslateFromConfig(map[string]any{"provider": "openai", "apiKey": "test"}, "fr")
		require.NoError(t, err)
		_, isAI := tl.(*tools.AITranslateTool)
		assert.True(t, isAI)
	})

	t.Run("MT engine id routes to the MT tool", func(t *testing.T) {
		registerPluginMT(t, "plugin-routing-mt")
		tl, err := tools.NewTranslateFromConfig(map[string]any{"provider": "plugin-routing-mt", "apiKey": "test"}, "fr")
		require.NoError(t, err)
		_, isAI := tl.(*tools.AITranslateTool)
		assert.False(t, isAI, "an MT provider id must not build the LLM translator")
		assert.Equal(t, "translate", tl.Name(), "MT backend still reports the unified tool name")
	})

	t.Run("unknown engine errors", func(t *testing.T) {
		_, err := tools.NewTranslateFromConfig(map[string]any{"provider": "no-such-engine"}, "fr")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown AI provider")
	})
}

// TestNewCheckFromConfig_ModeDispatch pins the `qa` mode discriminator: rules
// mode (and the no-provider default) builds the deterministic checker, ai
// mode (and the legacy provider-set heuristic) builds the LLM judge.
func TestNewCheckFromConfig_ModeDispatch(t *testing.T) {
	isLLMJudge := func(config map[string]any) bool {
		tl, err := tools.NewCheckFromConfig(config, "fr")
		require.NoError(t, err)
		_, ok := tl.(*tools.AICheckTool)
		return ok
	}

	assert.False(t, isLLMJudge(map[string]any{}), "default is deterministic rules")
	assert.False(t, isLLMJudge(map[string]any{"mode": "rules", "provider": "anthropic", "apiKey": "k"}),
		"explicit rules mode wins even with a provider set")
	assert.True(t, isLLMJudge(map[string]any{"mode": "ai", "apiKey": "k"}), "explicit ai mode builds the LLM judge")
	assert.True(t, isLLMJudge(map[string]any{"provider": "anthropic", "apiKey": "k"}),
		"legacy heuristic: provider set without a mode means AI")
}

// TestTranslateGroupRegistration is the tool-group registration sanity check:
// RegisterAll wires the translate and qa groups into the registry, the
// composed translate schema carries the engine discriminator (LLM default,
// MT member), and the registry builds a working tool from group config.
func TestTranslateGroupRegistration(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	for _, name := range []string{"translate", "qa"} {
		require.Truef(t, reg.Has(registry.ToolID(name)), "group %q must be registered", name)
	}

	s := tools.TranslateSchema()
	require.NotNil(t, s)
	engine, ok := s.Properties["engine"]
	require.True(t, ok, "composed translate schema must expose the engine discriminator")
	assert.Equal(t, "llm", engine.Default, "LLM is the default engine")
	var values []any
	for _, o := range engine.Options {
		values = append(values, o.Value)
	}
	assert.ElementsMatch(t, []any{"llm", "mt"}, values)

	// Provider options include the registered LLM providers.
	provider, ok := s.Properties["provider"]
	require.True(t, ok)
	var providerValues []any
	for _, o := range provider.Options {
		providerValues = append(providerValues, o.Value)
	}
	for _, p := range aiprovider.Providers() {
		assert.Contains(t, providerValues, any(string(p.Name)))
	}

	// End to end through the registry: group config builds a runnable tool.
	tl, err := reg.NewToolWithConfig("translate", map[string]any{"engine": "llm", "apiKey": "test"}, "fr")
	require.NoError(t, err)
	assert.Equal(t, "translate", tl.Name())
}
