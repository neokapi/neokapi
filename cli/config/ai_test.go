package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyAIToolDefaults(t *testing.T) {
	cfg := NewAppConfig()
	cfg.Set(KeyAIProvider, "ollama")
	cfg.Set(KeyAIModel, "llama3.2:3b")

	t.Run("fills provider+model for an AI tool when absent", func(t *testing.T) {
		got := ApplyAIToolDefaults(cfg, "translate", []string{"credentials"}, map[string]any{})
		assert.Equal(t, "ollama", got["provider"])
		assert.Equal(t, "llama3.2:3b", got["model"])
	})

	t.Run("explicit provider wins and model is not forced", func(t *testing.T) {
		got := ApplyAIToolDefaults(cfg, "translate", []string{"credentials"}, map[string]any{"provider": "openai"})
		assert.Equal(t, "openai", got["provider"])
		_, hasModel := got["model"]
		assert.False(t, hasModel, "model default must not attach to an explicitly chosen provider")
	})

	t.Run("MT tools are untouched (provider encoded in name)", func(t *testing.T) {
		got := ApplyAIToolDefaults(cfg, "deepl-translate", []string{"credentials"}, map[string]any{})
		_, ok := got["provider"]
		assert.False(t, ok)
	})

	t.Run("non-credential tools are untouched", func(t *testing.T) {
		got := ApplyAIToolDefaults(cfg, "word-count", []string{}, map[string]any{})
		_, ok := got["provider"]
		assert.False(t, ok)
	})

	t.Run("no default configured → no change", func(t *testing.T) {
		got := ApplyAIToolDefaults(NewAppConfig(), "translate", []string{"credentials"}, map[string]any{})
		_, ok := got["provider"]
		assert.False(t, ok)
	})
}

func TestIsMTToolName(t *testing.T) {
	assert.True(t, isMTToolName("deepl-translate"))
	assert.True(t, isMTToolName("google-translate"))
	assert.False(t, isMTToolName("translate"), "the unified LLM translate tool is not MT")
	assert.False(t, isMTToolName("ai-translate"), "legacy LLM tool name is not MT")
	assert.False(t, isMTToolName("qa"))
	assert.False(t, isMTToolName("word-count"))
}
