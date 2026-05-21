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

	for _, name := range []string{"ai-translate", "ai-qa", "ai-review", "brand-voice-check", "ai-terminology"} {
		assert.Truef(t, reg.Has(registry.ToolID(name)), "tool %q should be registered", name)
	}
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
