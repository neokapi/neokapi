package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/cli/config"
)

func TestApplyAIDefaults(t *testing.T) {
	cfg := config.NewAppConfig()
	cfg.Set(config.KeyAIProvider, "gemma")
	cfg.Set(config.KeyAIModel, "gemma-4-e2b")

	t.Run("fills provider+model for an AI tool when absent", func(t *testing.T) {
		got := applyAIDefaults(cfg, "ai-translate", []string{"credentials"}, map[string]any{})
		assert.Equal(t, "gemma", got["provider"])
		assert.Equal(t, "gemma-4-e2b", got["model"])
	})

	t.Run("explicit provider wins and model is not forced", func(t *testing.T) {
		got := applyAIDefaults(cfg, "ai-translate", []string{"credentials"}, map[string]any{"provider": "openai"})
		assert.Equal(t, "openai", got["provider"])
		_, hasModel := got["model"]
		assert.False(t, hasModel, "model default must not attach to an explicitly chosen provider")
	})

	t.Run("MT tools are untouched (provider encoded in name)", func(t *testing.T) {
		got := applyAIDefaults(cfg, "deepl-translate", []string{"credentials"}, map[string]any{})
		_, ok := got["provider"]
		assert.False(t, ok)
	})

	t.Run("non-credential tools are untouched", func(t *testing.T) {
		got := applyAIDefaults(cfg, "word-count", []string{}, map[string]any{})
		_, ok := got["provider"]
		assert.False(t, ok)
	})

	t.Run("no default configured → no change", func(t *testing.T) {
		empty := config.NewAppConfig()
		got := applyAIDefaults(empty, "ai-translate", []string{"credentials"}, map[string]any{})
		_, ok := got["provider"]
		assert.False(t, ok)
	})
}

func TestIsMTTool(t *testing.T) {
	assert.True(t, isMTTool("deepl-translate"))
	assert.True(t, isMTTool("google-translate"))
	assert.False(t, isMTTool("ai-translate"), "ai-translate is the LLM tool, not MT")
	assert.False(t, isMTTool("ai-qa"))
	assert.False(t, isMTTool("word-count"))
}
