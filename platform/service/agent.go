package service

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/id"
	platagent "github.com/neokapi/neokapi/platform/agent"
	platev "github.com/neokapi/neokapi/platform/event"
)

// AgentService orchestrates agent conversations, messages, and tool policy.
type AgentService struct {
	store      platagent.AgentStore
	eventBus   platev.EventBus
	pool       *AgentPool       // manages ZeroClaw containers (nil = mock mode)
	tokenStore *AgentTokenStore // scoped agent tokens for MCP delegation
}

// NewAgentService creates the agent service.
func NewAgentService(store platagent.AgentStore, eventBus platev.EventBus) *AgentService {
	return &AgentService{
		store:      store,
		eventBus:   eventBus,
		tokenStore: NewAgentTokenStore(),
	}
}

// SetPool attaches a container pool for ZeroClaw integration.
// When set, SendMessageStream routes to real agent containers.
func (s *AgentService) SetPool(pool *AgentPool) {
	s.pool = pool
}

// TokenStore returns the agent token store for middleware integration.
func (s *AgentService) TokenStore() *AgentTokenStore {
	return s.tokenStore
}

// CreateConversation creates a new conversation.
func (s *AgentService) CreateConversation(ctx context.Context, workspaceID, userID, projectID, title string) (*platagent.Conversation, error) {
	if title == "" {
		title = "New conversation"
	}
	conv := &platagent.Conversation{
		WorkspaceID: workspaceID,
		UserID:      userID,
		ProjectID:   projectID,
		Title:       title,
	}
	if err := s.store.CreateConversation(ctx, conv); err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(platev.Event{
			ID:     id.New(),
			Type:   platev.EventAgentConversationCreated,
			Source: "agent",
			Actor:  "bravo:" + userID,
			Data: map[string]string{
				"conversation_id": conv.ID,
				"workspace_id":    workspaceID,
			},
			Timestamp: time.Now(),
		})
	}

	return conv, nil
}

// GetConversation retrieves a conversation by ID.
func (s *AgentService) GetConversation(ctx context.Context, id string) (*platagent.Conversation, error) {
	return s.store.GetConversation(ctx, id)
}

// ListConversations returns paginated conversations for a user in a workspace.
func (s *AgentService) ListConversations(ctx context.Context, workspaceID, userID string, limit, offset int) ([]*platagent.Conversation, int, error) {
	return s.store.ListConversations(ctx, workspaceID, userID, limit, offset)
}

// DeleteConversation removes a conversation and all its messages.
// Also releases any associated container and revokes tokens.
func (s *AgentService) DeleteConversation(ctx context.Context, convID string) error {
	s.cleanupConversation(ctx, convID)
	return s.store.DeleteConversation(ctx, convID)
}

// SendMessage persists a user message and generates an agent response.
// This is the synchronous variant used by the Phase 1 JSON API.
func (s *AgentService) SendMessage(ctx context.Context, conversationID, userID, content string) (*platagent.Message, *platagent.Message, error) {
	// Persist user message.
	userMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleUser,
		Content:        content,
	}
	if err := s.store.AddMessage(ctx, userMsg); err != nil {
		return nil, nil, fmt.Errorf("add user message: %w", err)
	}

	// Phase 1: mock assistant response.
	// Phase 2 will POST to ZeroClaw gateway and stream real responses.
	assistantMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleAssistant,
		Content:        fmt.Sprintf("I received your message: %q. Agent integration is coming in Phase 2.", content),
	}
	if err := s.store.AddMessage(ctx, assistantMsg); err != nil {
		return nil, nil, fmt.Errorf("add assistant message: %w", err)
	}

	// Update conversation timestamp.
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err == nil {
		_ = s.store.UpdateConversation(ctx, conv)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(platev.Event{
			ID:     id.New(),
			Type:   platev.EventAgentMessageSent,
			Source: "agent",
			Actor:  "bravo:" + userID,
			Data: map[string]string{
				"conversation_id": conversationID,
				"message_id":      assistantMsg.ID,
			},
			Timestamp: time.Now(),
		})
	}

	return userMsg, assistantMsg, nil
}

// ListMessages returns paginated messages for a conversation.
func (s *AgentService) ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]*platagent.Message, error) {
	return s.store.ListMessages(ctx, conversationID, limit, offset)
}

// ApproveToolCall approves a gated tool call.
func (s *AgentService) ApproveToolCall(ctx context.Context, conversationID, toolCallID, userID string) error {
	tc := &platagent.ToolCall{
		ID:     toolCallID,
		Status: platagent.ToolCallRunning,
	}
	if err := s.store.UpdateToolCall(ctx, tc); err != nil {
		return fmt.Errorf("approve tool call: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(platev.Event{
			ID:     id.New(),
			Type:   platev.EventAgentToolApproved,
			Source: "agent",
			Actor:  "bravo:" + userID,
			Data: map[string]string{
				"conversation_id": conversationID,
				"tool_call_id":    toolCallID,
			},
			Timestamp: time.Now(),
		})
	}
	return nil
}

// DenyToolCall denies a gated tool call.
func (s *AgentService) DenyToolCall(ctx context.Context, conversationID, toolCallID, userID string) error {
	tc := &platagent.ToolCall{
		ID:     toolCallID,
		Status: platagent.ToolCallDenied,
	}
	if err := s.store.UpdateToolCall(ctx, tc); err != nil {
		return fmt.Errorf("deny tool call: %w", err)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(platev.Event{
			ID:     id.New(),
			Type:   platev.EventAgentToolDenied,
			Source: "agent",
			Actor:  "bravo:" + userID,
			Data: map[string]string{
				"conversation_id": conversationID,
				"tool_call_id":    toolCallID,
			},
			Timestamp: time.Now(),
		})
	}
	return nil
}

// CancelConversation marks a conversation as failed and releases resources.
func (s *AgentService) CancelConversation(ctx context.Context, conversationID string) error {
	s.cleanupConversation(ctx, conversationID)

	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	conv.Status = platagent.ConversationFailed
	return s.store.UpdateConversation(ctx, conv)
}

// SendMessageStream sends a user message and streams the agent response via SSE.
// When a pool is configured, it routes to a real ZeroClaw container.
// Otherwise, it falls back to the synchronous mock response.
func (s *AgentService) SendMessageStream(ctx context.Context, conversationID, userID, workspaceID, workspaceRole, content string, sse SSEWriter) error {
	// Persist user message.
	userMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleUser,
		Content:        content,
	}
	if err := s.store.AddMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("add user message: %w", err)
	}

	// If no pool is configured, fall back to mock.
	if s.pool == nil {
		return s.sendMockStream(ctx, conversationID, userID, content, sse)
	}

	// Create a scoped agent token for MCP delegation.
	tokenTTL := 30 * time.Minute
	token, err := s.tokenStore.Create(userID, workspaceID, conversationID, workspaceRole, tokenTTL)
	if err != nil {
		return fmt.Errorf("create agent token: %w", err)
	}

	// Acquire a container from the pool.
	container, err := s.pool.Acquire(ctx, ContainerConfig{
		ConversationID: conversationID,
		WorkspaceID:    workspaceID,
		UserID:         userID,
		AgentToken:     token.Token,
	})
	if err != nil {
		s.tokenStore.Revoke(token.Token)
		return fmt.Errorf("acquire container: %w", err)
	}

	// POST message to ZeroClaw gateway and stream response.
	// Phase 2 implementation: the gateway integration will pipe response
	// chunks from ZeroClaw → SSE events to the client.
	_ = container // Will be used in the HTTP POST to container.GatewayURL

	// For now, emit mock SSE events since we don't have a running ZeroClaw yet.
	return s.sendMockStream(ctx, conversationID, userID, content, sse)
}

// sendMockStream emits mock SSE events for testing without ZeroClaw.
func (s *AgentService) sendMockStream(ctx context.Context, conversationID, userID, content string, sse SSEWriter) error {
	assistantMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleAssistant,
		Content:        fmt.Sprintf("I received your message: %q. Agent integration is coming in Phase 2.", content),
	}
	if err := s.store.AddMessage(ctx, assistantMsg); err != nil {
		return fmt.Errorf("add assistant message: %w", err)
	}

	// Emit SSE events.
	_ = sse.WriteEvent(SSEMessageStart, MessageStartData{ID: assistantMsg.ID, Role: "assistant"})
	_ = sse.WriteEvent(SSEContentDelta, ContentDeltaData{Delta: assistantMsg.Content})
	_ = sse.WriteEvent(SSEMessageEnd, MessageEndData{ID: assistantMsg.ID})

	// Update conversation timestamp.
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err == nil {
		_ = s.store.UpdateConversation(ctx, conv)
	}

	if s.eventBus != nil {
		s.eventBus.Publish(platev.Event{
			ID:     id.New(),
			Type:   platev.EventAgentMessageSent,
			Source: "agent",
			Actor:  "bravo:" + userID,
			Data: map[string]string{
				"conversation_id": conversationID,
				"message_id":      assistantMsg.ID,
			},
			Timestamp: time.Now(),
		})
	}

	return nil
}

// cleanupConversation releases pool containers and revokes tokens for a conversation.
func (s *AgentService) cleanupConversation(ctx context.Context, conversationID string) {
	if s.pool != nil {
		_ = s.pool.Release(ctx, conversationID)
	}
	if s.tokenStore != nil {
		s.tokenStore.RevokeForConversation(conversationID)
	}
}

// GetConfig returns the agent config for a workspace.
func (s *AgentService) GetConfig(ctx context.Context, workspaceID string) (*platagent.AgentConfig, error) {
	return s.store.GetAgentConfig(ctx, workspaceID)
}

// SaveConfig saves the agent config for a workspace.
func (s *AgentService) SaveConfig(ctx context.Context, cfg *platagent.AgentConfig) error {
	return s.store.SaveAgentConfig(ctx, cfg)
}

// ListAvailableTools returns all MCP tools available to the agent after
// applying workspace policy. toolNames is the full set of registered tools.
func (s *AgentService) ListAvailableTools(ctx context.Context, workspaceID string, toolNames []string) ([]ToolInfo, error) {
	cfg, err := s.store.GetAgentConfig(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	var tools []ToolInfo
	for _, name := range toolNames {
		decision := checkToolPolicy(cfg, name)
		if decision != "deny" {
			tools = append(tools, ToolInfo{
				Name:            name,
				RequireApproval: decision == "approve",
			})
		}
	}
	return tools, nil
}

// ToolInfo describes a tool available to the agent.
type ToolInfo struct {
	Name            string `json:"name"`
	RequireApproval bool   `json:"require_approval"`
}

func checkToolPolicy(cfg *platagent.AgentConfig, toolName string) string {
	if cfg == nil || !cfg.Enabled {
		return "deny"
	}
	for _, t := range cfg.DeniedTools {
		if t == toolName {
			return "deny"
		}
	}
	for _, t := range cfg.RequireApproval {
		if t == toolName {
			return "approve"
		}
	}
	if len(cfg.AllowedTools) > 0 {
		for _, t := range cfg.AllowedTools {
			if t == toolName {
				return "allow"
			}
		}
		return "deny"
	}
	return "allow"
}

// ToolNames returns the names of all registered MCP agent tools.
func ToolNames() []string {
	return []string{
		"list_projects", "get_project", "create_project", "update_project",
		"list_blocks", "get_block", "update_block",
		"create_version", "list_streams", "diff_streams", "merge_stream",
		"list_flows", "run_flow", "get_flow_status",
		"tm_search", "tm_import",
		"term_search", "term_add",
		"connector_pull", "connector_push", "connector_status",
		"execute_script",
		"check_vocabulary", "list_profiles", "get_voice_guide",
	}
}

