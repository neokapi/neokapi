package tools_test

import (
	"maps"
	"testing"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/registry"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wrappedProvider stands in for whatever a host wraps a provider with. It
// carries the handle it wrapped, so a test can say which provider the tool
// ended up calling as well as that it passed through the observer.
type wrappedProvider struct {
	aiprovider.LLMProvider
	inner aiprovider.LLMProvider
}

// observerReturning is a host's observer plus a record of what it saw.
func observerReturning() (aitools.ProviderObserver, *[]aiprovider.LLMProvider) {
	var seen []aiprovider.LLMProvider
	return func(inner aiprovider.LLMProvider) aiprovider.LLMProvider {
		seen = append(seen, inner)
		return &wrappedProvider{LLMProvider: inner, inner: inner}
	}, &seen
}

// A tool that builds its own provider from a named id still calls it through
// the host's observer. That is the case a grant cannot reach: the provider is
// constructed inside the factory, where a host counting model calls has nothing
// to hold.
func TestObserverWrapsAProviderTheToolBuiltItself(t *testing.T) {
	reg := aiToolRegistry()
	for _, tc := range injectedProviderTools {
		t.Run(string(tc.name), func(t *testing.T) {
			observe, seen := observerReturning()
			config := map[string]any{
				aitools.ObserverKey: observe,
				"provider":          string(aiprovider.Demo),
			}
			maps.Copy(config, tc.config)

			built, err := reg.NewToolWithConfig(tc.name, config, "fr")
			require.NoError(t, err)

			prov := aitools.ToolProvider(built)
			require.NotNil(t, prov)
			wrapper, ok := prov.(*wrappedProvider)
			require.True(t, ok, "tool %q called a provider the observer never saw", tc.name)
			assert.Equal(t, aiprovider.Demo, wrapper.inner.Name(),
				"the observer wrapped the provider the config named")
			require.Len(t, *seen, 1, "the observer runs once per tool")
			assert.NotContains(t, config, aitools.ObserverKey,
				"the observer must be taken out of the config before it is decoded")
		})
	}
}

// The host's grant passes through the same observer, so one mechanism accounts
// for every provider a tool can end up with.
func TestObserverWrapsTheGrantedProvider(t *testing.T) {
	reg := aiToolRegistry()
	granted := aiprovider.NewMockProvider()
	observe, seen := observerReturning()

	built, err := reg.NewToolWithConfig("translate", map[string]any{
		aitools.ProviderKey: granted,
		aitools.ObserverKey: observe,
	}, "fr")
	require.NoError(t, err)

	wrapper, ok := aitools.ToolProvider(built).(*wrappedProvider)
	require.True(t, ok)
	assert.Same(t, granted, wrapper.inner)
	assert.Len(t, *seen, 1)
}

// With no observer the tool calls the provider directly, which is what every
// host that meters nothing gets.
func TestNoObserverLeavesTheProviderAlone(t *testing.T) {
	reg := aiToolRegistry()
	granted := aiprovider.NewMockProvider()

	built, err := reg.NewToolWithConfig("translate", map[string]any{
		aitools.ProviderKey: granted,
	}, "fr")
	require.NoError(t, err)
	assert.Same(t, granted, aitools.ToolProvider(built))
}

// The backends that call no model read the same config map, and a function
// value left in it fails the JSON round trip their own config takes.
func TestObserverNeverReachesALocalBackend(t *testing.T) {
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
			observe, _ := observerReturning()
			config := map[string]any{aitools.ObserverKey: observe}
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

// An MT engine takes its own config round trip, so the observer must be out of
// the map before it decodes.
func TestObserverNeverReachesAnMTEngine(t *testing.T) {
	registerPluginMT(t, "plugin-mt-observe")
	reg := aiToolRegistry()

	observe, seen := observerReturning()
	config := map[string]any{
		aitools.ObserverKey: observe,
		"provider":          "plugin-mt-observe",
	}
	built, err := reg.NewToolWithConfig("translate", config, "fr")
	require.NoError(t, err)
	assert.Nil(t, aitools.ToolProvider(built))
	assert.NotContains(t, config, aitools.ObserverKey)
	assert.Empty(t, *seen, "an MT engine calls no model, so there is nothing to observe")
}
