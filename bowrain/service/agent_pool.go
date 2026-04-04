package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ContainerConfig holds the configuration for spawning a ZeroClaw agent container.
type ContainerConfig struct {
	Image          string            // e.g. "ghcr.io/neokapi/bravo-agent:latest"
	ConversationID string            // unique conversation this container serves
	WorkspaceID    string            // workspace scope
	UserID         string            // user the agent acts on behalf of
	MCPEndpoint    string            // bowrain MCP server URL
	AgentToken     string            // scoped bearer token for MCP auth
	ModelProvider  string            // "azure-openai", "anthropic", "ollama"
	ModelName      string            // e.g. "gpt-4o", "claude-sonnet-4-20250514"
	ModelAPIBase   string            // provider API base URL
	ModelAPIKey    string            // provider API key
	SystemPrompt   string            // agent system prompt
	GatewayPort    int               // port for ZeroClaw gateway (default 42617)
	Env            map[string]string // additional environment variables

	// Registry credentials for pulling private container images.
	RegistryServer   string // e.g. "ghcr.io"
	RegistryUsername string // e.g. "neokapi"
	RegistryPassword string // PAT or token
}

// AgentContainer represents a running ZeroClaw agent container.
type AgentContainer struct {
	ID             string // container ID (from runtime)
	GatewayURL     string // e.g. "http://10.0.1.42:42617"
	ConversationID string
	WorkspaceID    string
	UserID         string
	CreatedAt      time.Time
}

// ContainerRuntime abstracts the container orchestration backend
// (Docker, containerd, Azure Container Apps, etc.).
type ContainerRuntime interface {
	Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error)
	Stop(ctx context.Context, containerID string) error
	Health(ctx context.Context, containerID string) (bool, error)
}

// AgentPool manages the lifecycle of ZeroClaw agent containers.
// It maintains a mapping from conversation IDs to running containers,
// handles spawning, health-checking, and recycling.
type AgentPool struct {
	runtime         ContainerRuntime
	mcpEndpoint     string
	bravoImage      string
	maxPerWorkspace int
	idleTimeout     time.Duration

	// Model defaults — injected into containers when not overridden per-request.
	modelProvider string
	modelName     string
	modelAPIBase  string
	modelAPIKey   string

	// Registry credentials for private images.
	registryServer   string
	registryUsername string
	registryPassword string

	mu         sync.Mutex
	containers map[string]*AgentContainer // conversationID → container
	pending    map[string]int             // workspaceID → count of spawns in progress
}

// AgentPoolConfig holds configuration for the agent pool.
type AgentPoolConfig struct {
	Runtime         ContainerRuntime
	MCPEndpoint     string        // bowrain MCP server URL
	BravoImage      string        // container image for @bravo
	MaxPerWorkspace int           // max concurrent containers per workspace
	IdleTimeout     time.Duration // idle timeout before recycling

	// Model defaults for agent containers.
	ModelProvider string // e.g. "azure-openai", "anthropic"
	ModelName     string // e.g. "gpt-4o"
	ModelAPIBase  string
	ModelAPIKey   string

	// Registry credentials for pulling private container images.
	RegistryServer   string
	RegistryUsername string
	RegistryPassword string
}

// NewAgentPool creates a new agent container pool.
func NewAgentPool(cfg AgentPoolConfig) *AgentPool {
	if cfg.MaxPerWorkspace <= 0 {
		cfg.MaxPerWorkspace = 3
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.BravoImage == "" {
		cfg.BravoImage = "ghcr.io/neokapi/bravo-agent:latest"
	}
	return &AgentPool{
		runtime:          cfg.Runtime,
		mcpEndpoint:      cfg.MCPEndpoint,
		bravoImage:       cfg.BravoImage,
		maxPerWorkspace:  cfg.MaxPerWorkspace,
		idleTimeout:      cfg.IdleTimeout,
		modelProvider:    cfg.ModelProvider,
		modelName:        cfg.ModelName,
		modelAPIBase:     cfg.ModelAPIBase,
		modelAPIKey:      cfg.ModelAPIKey,
		registryServer:   cfg.RegistryServer,
		registryUsername: cfg.RegistryUsername,
		registryPassword: cfg.RegistryPassword,
		containers:       make(map[string]*AgentContainer),
		pending:          make(map[string]int),
	}
}

// Acquire returns an existing container for the conversation, or spawns a new one.
func (p *AgentPool) Acquire(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error) {
	p.mu.Lock()
	if c, ok := p.containers[cfg.ConversationID]; ok {
		p.mu.Unlock()
		// Health-check the existing container.
		healthy, err := p.runtime.Health(ctx, c.ID)
		if err == nil && healthy {
			return c, nil
		}
		// Unhealthy — remove and respawn.
		p.mu.Lock()
		delete(p.containers, cfg.ConversationID)
		p.mu.Unlock()
		_ = p.runtime.Stop(ctx, c.ID)
	} else {
		p.mu.Unlock()
	}

	// Check workspace concurrency limit and reserve a pending slot atomically.
	p.mu.Lock()
	count := 0
	for _, c := range p.containers {
		if c.WorkspaceID == cfg.WorkspaceID {
			count++
		}
	}
	count += p.pending[cfg.WorkspaceID]
	if count >= p.maxPerWorkspace {
		p.mu.Unlock()
		return nil, fmt.Errorf("workspace %s has reached max concurrent agents (%d)", cfg.WorkspaceID, p.maxPerWorkspace)
	}
	p.pending[cfg.WorkspaceID]++
	p.mu.Unlock()

	// Fill in pool-level defaults.
	if cfg.Image == "" {
		cfg.Image = p.bravoImage
	}
	if cfg.MCPEndpoint == "" {
		cfg.MCPEndpoint = p.mcpEndpoint
	}
	if cfg.GatewayPort == 0 {
		cfg.GatewayPort = 42617
	}
	if cfg.ModelProvider == "" {
		cfg.ModelProvider = p.modelProvider
	}
	if cfg.ModelName == "" {
		cfg.ModelName = p.modelName
	}
	if cfg.ModelAPIBase == "" {
		cfg.ModelAPIBase = p.modelAPIBase
	}
	if cfg.ModelAPIKey == "" {
		cfg.ModelAPIKey = p.modelAPIKey
	}
	if cfg.RegistryServer == "" {
		cfg.RegistryServer = p.registryServer
	}
	if cfg.RegistryUsername == "" {
		cfg.RegistryUsername = p.registryUsername
	}
	if cfg.RegistryPassword == "" {
		cfg.RegistryPassword = p.registryPassword
	}

	container, err := p.runtime.Spawn(ctx, cfg)

	p.mu.Lock()
	p.pending[cfg.WorkspaceID]--
	if err != nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("spawn agent container: %w", err)
	}
	p.containers[cfg.ConversationID] = container
	p.mu.Unlock()

	return container, nil
}

// Release stops and removes a container for a conversation.
// Returns the container metadata (for metering) or nil if not found.
func (p *AgentPool) Release(ctx context.Context, conversationID string) (*AgentContainer, error) {
	p.mu.Lock()
	c, ok := p.containers[conversationID]
	if ok {
		delete(p.containers, conversationID)
	}
	p.mu.Unlock()

	if !ok {
		return nil, nil
	}
	return c, p.runtime.Stop(ctx, c.ID)
}

// Get returns the container for a conversation, if any.
func (p *AgentPool) Get(conversationID string) (*AgentContainer, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.containers[conversationID]
	return c, ok
}

// ActiveCount returns the number of active containers for a workspace.
func (p *AgentPool) ActiveCount(workspaceID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, c := range p.containers {
		if c.WorkspaceID == workspaceID {
			count++
		}
	}
	return count
}

// CleanupIdle removes containers that have been idle longer than the configured timeout.
// Returns the number of containers stopped.
func (p *AgentPool) CleanupIdle(ctx context.Context) int {
	p.mu.Lock()
	var idle []*AgentContainer
	now := time.Now()
	for convID, c := range p.containers {
		if now.Sub(c.CreatedAt) > p.idleTimeout {
			idle = append(idle, c)
			delete(p.containers, convID)
		}
	}
	p.mu.Unlock()

	for _, c := range idle {
		_ = p.runtime.Stop(ctx, c.ID)
	}
	return len(idle)
}

// RunCleanupLoop periodically cleans up idle containers until the context is cancelled.
func (p *AgentPool) RunCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(p.idleTimeout / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := p.CleanupIdle(ctx); n > 0 {
				slog.Info("agent pool: cleaned up idle containers", "count", n)
			}
		}
	}
}

// StopAll stops all running containers. Used during server shutdown.
func (p *AgentPool) StopAll(ctx context.Context) {
	p.mu.Lock()
	all := make([]*AgentContainer, 0, len(p.containers))
	for _, c := range p.containers {
		all = append(all, c)
	}
	p.containers = make(map[string]*AgentContainer)
	p.mu.Unlock()

	for _, c := range all {
		_ = p.runtime.Stop(ctx, c.ID)
	}
}
