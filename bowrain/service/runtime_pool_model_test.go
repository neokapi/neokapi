package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureRuntime records the ContainerConfig passed to Spawn.
type captureRuntime struct {
	mockRuntime
	lastCfg ContainerConfig
}

func (r *captureRuntime) Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error) {
	r.lastCfg = cfg
	return r.mockRuntime.Spawn(ctx, cfg)
}

func TestAgentPoolFillsModelDefaults(t *testing.T) {
	rt := &captureRuntime{mockRuntime: *newMockRuntime()}
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:       rt,
		MCPEndpoint:   "http://localhost:8080/mcp/",
		ModelProvider: "anthropic",
		ModelName:     "claude-sonnet-4-20250514",
		ModelAPIBase:  "https://api.anthropic.com",
		ModelAPIKey:   "sk-test-key",
	})

	_, err := pool.Acquire(t.Context(), ContainerConfig{
		ConversationID: "conv-model",
		WorkspaceID:    "ws-1",
		UserID:         "user-1",
		AgentToken:     "token-abc",
	})
	require.NoError(t, err)

	// Model config should have been filled from pool defaults.
	assert.Equal(t, "anthropic", rt.lastCfg.ModelProvider)
	assert.Equal(t, "claude-sonnet-4-20250514", rt.lastCfg.ModelName)
	assert.Equal(t, "https://api.anthropic.com", rt.lastCfg.ModelAPIBase)
	assert.Equal(t, "sk-test-key", rt.lastCfg.ModelAPIKey)
	assert.Equal(t, "http://localhost:8080/mcp/", rt.lastCfg.MCPEndpoint)
	assert.Equal(t, "token-abc", rt.lastCfg.AgentToken)
}

func TestAgentPoolPerRequestModelOverridesDefaults(t *testing.T) {
	rt := &captureRuntime{mockRuntime: *newMockRuntime()}
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:       rt,
		ModelProvider: "anthropic",
		ModelName:     "claude-sonnet-4-20250514",
	})

	_, err := pool.Acquire(t.Context(), ContainerConfig{
		ConversationID: "conv-override",
		WorkspaceID:    "ws-1",
		ModelProvider:  "azure-openai",
		ModelName:      "gpt-4o",
	})
	require.NoError(t, err)

	// Per-request values should take precedence.
	assert.Equal(t, "azure-openai", rt.lastCfg.ModelProvider)
	assert.Equal(t, "gpt-4o", rt.lastCfg.ModelName)
}
