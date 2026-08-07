package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentPoolCleanupIdle_MeasuresFromLastUse: the sweep measured from
// CreatedAt, which makes "idle timeout" a maximum lifetime — a conversation
// still being used had its container stopped out from under it.
func TestAgentPoolCleanupIdle_MeasuresFromLastUse(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 10,
		IdleTimeout:     40 * time.Millisecond,
	})
	ctx := t.Context()

	_, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "c1", WorkspaceID: "ws1", UserID: "u1"})
	require.NoError(t, err)

	// Keep the conversation alive across more than one timeout's worth of time.
	for range 4 {
		time.Sleep(15 * time.Millisecond)
		_, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "c1", WorkspaceID: "ws1", UserID: "u1"})
		require.NoError(t, err)
		assert.Zero(t, pool.CleanupIdle(ctx), "a container in use is not idle")
	}

	_, ok := pool.Get("c1")
	assert.True(t, ok, "the conversation still has its container")

	// Stop using it, and it goes.
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, 1, pool.CleanupIdle(ctx))
	_, ok = pool.Get("c1")
	assert.False(t, ok)
}
