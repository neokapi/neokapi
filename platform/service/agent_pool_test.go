package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRuntime is an in-memory ContainerRuntime for testing.
type mockRuntime struct {
	mu         sync.Mutex
	containers map[string]*AgentContainer
	nextID     int
	spawnErr   error
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{containers: make(map[string]*AgentContainer)}
}

func (r *mockRuntime) Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error) {
	if r.spawnErr != nil {
		return nil, r.spawnErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	c := &AgentContainer{
		ID:             fmt.Sprintf("container-%d", r.nextID),
		GatewayURL:     fmt.Sprintf("http://localhost:%d", 42617+r.nextID),
		ConversationID: cfg.ConversationID,
		WorkspaceID:    cfg.WorkspaceID,
		UserID:         cfg.UserID,
		CreatedAt:      time.Now(),
	}
	r.containers[c.ID] = c
	return c, nil
}

func (r *mockRuntime) Stop(ctx context.Context, containerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.containers, containerID)
	return nil
}

func (r *mockRuntime) Health(ctx context.Context, containerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.containers[containerID]
	return ok, nil
}

func TestAgentPoolAcquireSpawnsContainer(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MCPEndpoint:     "http://localhost:8080/mcp/",
		MaxPerWorkspace: 3,
	})

	c, err := pool.Acquire(context.Background(), ContainerConfig{
		ConversationID: "conv-1",
		WorkspaceID:    "ws-1",
		UserID:         "user-1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, c.ID)
	assert.Equal(t, "conv-1", c.ConversationID)
	assert.Equal(t, 1, pool.ActiveCount("ws-1"))
}

func TestAgentPoolAcquireReusesContainer(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 3,
	})
	ctx := context.Background()

	c1, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)

	c2, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)

	assert.Equal(t, c1.ID, c2.ID) // Same container reused.
	assert.Equal(t, 1, pool.ActiveCount("ws-1"))
}

func TestAgentPoolAcquireEnforcesLimit(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 2,
	})
	ctx := context.Background()

	_, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)
	_, err = pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-2", WorkspaceID: "ws-1"})
	require.NoError(t, err)

	_, err = pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-3", WorkspaceID: "ws-1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent")
}

func TestAgentPoolRelease(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 3,
	})
	ctx := context.Background()

	_, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.Equal(t, 1, pool.ActiveCount("ws-1"))

	released, err := pool.Release(ctx, "conv-1")
	require.NoError(t, err)
	assert.NotNil(t, released)
	assert.Equal(t, 0, pool.ActiveCount("ws-1"))
}

func TestAgentPoolStopAll(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 10,
	})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := pool.Acquire(ctx, ContainerConfig{
			ConversationID: fmt.Sprintf("conv-%d", i),
			WorkspaceID:    "ws-1",
		})
		require.NoError(t, err)
	}
	assert.Equal(t, 5, pool.ActiveCount("ws-1"))

	pool.StopAll(ctx)
	assert.Equal(t, 0, pool.ActiveCount("ws-1"))
}

func TestAgentPoolGet(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 3,
	})
	ctx := context.Background()

	// Not found.
	_, ok := pool.Get("conv-1")
	assert.False(t, ok)

	// Found after acquire.
	c, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)

	got, ok := pool.Get("conv-1")
	assert.True(t, ok)
	assert.Equal(t, c.ID, got.ID)
}

func TestAgentPoolRespawnsUnhealthy(t *testing.T) {
	rt := newMockRuntime()
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: 3,
	})
	ctx := context.Background()

	c1, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)

	// Simulate container crash by removing it from the runtime.
	rt.mu.Lock()
	delete(rt.containers, c1.ID)
	rt.mu.Unlock()

	c2, err := pool.Acquire(ctx, ContainerConfig{ConversationID: "conv-1", WorkspaceID: "ws-1"})
	require.NoError(t, err)
	assert.NotEqual(t, c1.ID, c2.ID) // New container spawned.
}
