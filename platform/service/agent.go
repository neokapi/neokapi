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
// Phase 1 provides the data layer and mock agent responses; ZeroClaw
// container integration is added in Phase 2.
type AgentService struct {
	store    platagent.AgentStore
	eventBus platev.EventBus
}

// NewAgentService creates the agent service.
func NewAgentService(store platagent.AgentStore, eventBus platev.EventBus) *AgentService {
	return &AgentService{
		store:    store,
		eventBus: eventBus,
	}
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
func (s *AgentService) DeleteConversation(ctx context.Context, id string) error {
	return s.store.DeleteConversation(ctx, id)
}

// SSEEvent represents a server-sent event in the agent response stream.
type SSEEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// SendMessage persists a user message and generates an agent response.
// Phase 1: returns a mock response. Phase 2 will integrate ZeroClaw.
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

// CancelConversation marks a conversation as failed.
func (s *AgentService) CancelConversation(ctx context.Context, conversationID string) error {
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("get conversation: %w", err)
	}
	conv.Status = platagent.ConversationFailed
	return s.store.UpdateConversation(ctx, conv)
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

