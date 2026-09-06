package tools_test

import (
	"maps"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/registry"
	libtools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aiToolRegistry is the registry a host populates: the deterministic tools
// first, the model-backed ones over them.
func aiToolRegistry() *registry.ToolRegistry {
	reg := registry.NewToolRegistry()
	libtools.RegisterAll(reg)
	aitools.RegisterAll(reg)
	return reg
}

// injectedProviderTools are the registered tools that call a model and take
// their provider from the host when it grants one.
var injectedProviderTools = []struct {
	name   registry.ToolID
	config map[string]any
}{
	{name: "translate"},
	{name: "qa", config: map[string]any{"mode": "ai"}},
	{name: "review"},
	{name: "voice-check"},
	{name: "voice-infer"},
	{name: "term-extract"},
	{name: "media-refine"},
	{name: "entity-extract"},
}

func TestInjectedProviderIsTheOneTheToolCalls(t *testing.T) {
	reg := aiToolRegistry()
	for _, tc := range injectedProviderTools {
		t.Run(string(tc.name), func(t *testing.T) {
			granted := aiprovider.NewMockProvider()
			config := map[string]any{aitools.ProviderKey: granted}
			maps.Copy(config, tc.config)

			built, err := reg.NewToolWithConfig(tc.name, config, "fr")
			require.NoError(t, err)
			assert.Same(t, granted, aitools.ToolProvider(built),
				"tool %q must call the provider the host granted", tc.name)
			assert.NotContains(t, config, aitools.ProviderKey,
				"the provider must be taken out of the config before it is decoded")
		})
	}
}

// A step that names its own provider keeps it: the grant fills a gap rather
// than overriding an author's choice.
func TestNamedProviderWinsOverTheGrant(t *testing.T) {
	reg := aiToolRegistry()
	for _, tc := range injectedProviderTools {
		t.Run(string(tc.name), func(t *testing.T) {
			config := map[string]any{
				aitools.ProviderKey: aiprovider.NewMockProvider(),
				"provider":          string(aiprovider.Demo),
			}
			maps.Copy(config, tc.config)

			built, err := reg.NewToolWithConfig(tc.name, config, "fr")
			require.NoError(t, err)
			prov := aitools.ToolProvider(built)
			require.NotNil(t, prov)
			assert.Equal(t, aiprovider.Demo, prov.Name())
		})
	}
}

func TestToolBuildsItsOwnProviderWithoutAGrant(t *testing.T) {
	reg := aiToolRegistry()
	for _, tc := range injectedProviderTools {
		t.Run(string(tc.name), func(t *testing.T) {
			config := map[string]any{"provider": string(aiprovider.Demo)}
			maps.Copy(config, tc.config)

			built, err := reg.NewToolWithConfig(tc.name, config, "fr")
			require.NoError(t, err)
			prov := aitools.ToolProvider(built)
			require.NotNil(t, prov)
			assert.Equal(t, aiprovider.Demo, prov.Name())
		})
	}
}

// The backends that call no model read the same config map, and a live handle
// left in it fails the JSON round trip their own config takes.
func TestGrantedProviderNeverReachesALocalBackend(t *testing.T) {
	reg := aiToolRegistry()
	cases := []struct {
		name   string
		tool   registry.ToolID
		config map[string]any
	}{
		{name: "check in rules mode", tool: "qa", config: map[string]any{"mode": "rules"}},
		{name: "entity extract on the local model", tool: "entity-extract", config: map[string]any{"engine": "ner"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{aitools.ProviderKey: aiprovider.NewMockProvider()}
			maps.Copy(config, tc.config)
			built, err := reg.NewToolWithConfig(tc.tool, config, "fr")
			if err != nil {
				// entity-extract's local model is absent in this environment;
				// what matters is that the config round trip was not the reason.
				assert.NotContains(t, err.Error(), "json")
				return
			}
			assert.Nil(t, aitools.ToolProvider(built))
		})
	}
}

// An MT engine takes its own config round trip, so a granted LLM provider must
// be out of the map before it decodes.
func TestGrantedProviderNeverReachesAnMTEngine(t *testing.T) {
	registerPluginMT(t, "plugin-mt-grant")
	reg := aiToolRegistry()

	config := map[string]any{
		aitools.ProviderKey: aiprovider.NewMockProvider(),
		"provider":          "plugin-mt-grant",
	}
	built, err := reg.NewToolWithConfig("translate", config, "fr")
	require.NoError(t, err)
	assert.Nil(t, aitools.ToolProvider(built))
	assert.NotContains(t, config, aitools.ProviderKey)
}
