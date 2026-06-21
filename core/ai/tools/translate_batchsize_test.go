package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Local providers (e.g. on-device models) must translate per-block rather than
// batched into one structured-JSON generation. "demo" and "ollama" are
// registered Local:true in providers/ai; "anthropic" is not.
func TestLocalProviderForcesPerBlock(t *testing.T) {
	// Default (unset) batch size on a local provider → per-block.
	local := NewAITranslateTool(nil, AITranslateConfig{Provider: "demo"})
	assert.Equal(t, 1, local.batchSize, "local provider should default to per-block")

	// Cloud provider keeps the batched default.
	cloud := NewAITranslateTool(nil, AITranslateConfig{Provider: "anthropic"})
	assert.Equal(t, DefaultBatchSize, cloud.batchSize, "cloud provider keeps batched default")

	// An explicit smaller batch on a local provider is respected (not overridden).
	explicit := NewAITranslateTool(nil, AITranslateConfig{Provider: "demo", BatchSize: 8})
	assert.Equal(t, 8, explicit.batchSize)
}
