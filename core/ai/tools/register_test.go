package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAllRegistersBrandAndTerminology(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	for _, name := range []string{"translate", "qa", "ai-review", "brand-voice-check", "ai-terminology"} {
		assert.Truef(t, reg.Has(registry.ToolID(name)), "tool %q should be registered", name)
	}
}

// TestTranslateDispatch verifies the unified translate tool builds the LLM
// backend by default and the MT backend when --provider names an engine.
func TestTranslateDispatch(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	// Default (no provider) → LLM translator.
	tl, err := reg.NewToolWithConfig("translate", map[string]any{"apiKey": "test"}, "fr")
	require.NoError(t, err)
	require.NotNil(t, tl)
	assert.Equal(t, "translate", tl.Name())

	// MT engine → machine-translation tool, still reported as "translate".
	mtTool, err := reg.NewToolWithConfig("translate", map[string]any{"provider": "deepl", "apiKey": "test"}, "fr")
	require.NoError(t, err)
	require.NotNil(t, mtTool)
	assert.Equal(t, "translate", mtTool.Name())
}

// TestQADispatch verifies qa runs deterministic rule checks with no provider
// (and needs no credentials) and the LLM judge when a provider is set.
func TestQADispatch(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	// No provider → deterministic rule checks, constructible without credentials.
	det, err := reg.NewToolWithConfig("qa", map[string]any{}, "fr")
	require.NoError(t, err)
	require.NotNil(t, det)

	// Provider set → LLM-judged QA.
	llm, err := reg.NewToolWithConfig("qa", map[string]any{"provider": "anthropic", "apiKey": "test"}, "fr")
	require.NoError(t, err)
	require.NotNil(t, llm)
}

func TestBrandVoiceCheckFromConfig(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	tl, err := reg.NewToolWithConfig("brand-voice-check", map[string]any{"provider": "anthropic", "apiKey": "test"}, "")
	require.NoError(t, err)
	require.NotNil(t, tl)
}

func TestAITerminologyFromConfig(t *testing.T) {
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)

	tl, err := reg.NewToolWithConfig("ai-terminology", map[string]any{"provider": "anthropic", "apiKey": "test", "domain": "technology"}, "")
	require.NoError(t, err)
	require.NotNil(t, tl)
}
