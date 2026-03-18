// Package agent defines the domain types and store interface for the
// @bravo AI agent system (AD-028).
package agent

import (
	"context"
	"encoding/json"
	"time"
)

// ConversationStatus represents the state of a conversation.
type ConversationStatus string

const (
	ConversationActive    ConversationStatus = "active"
	ConversationCompleted ConversationStatus = "completed"
	ConversationFailed    ConversationStatus = "failed"
)

// MessageRole identifies who sent a message.
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// ToolCallStatus tracks the lifecycle of a tool invocation.
type ToolCallStatus string

const (
	ToolCallPending       ToolCallStatus = "pending"
	ToolCallRunning       ToolCallStatus = "running"
	ToolCallCompleted     ToolCallStatus = "completed"
	ToolCallFailed        ToolCallStatus = "failed"
	ToolCallNeedsApproval ToolCallStatus = "needs_approval"
	ToolCallDenied        ToolCallStatus = "denied"
)

// Conversation is a chat session between a user and @bravo.
type Conversation struct {
	ID          string             `json:"id"`
	WorkspaceID string             `json:"workspace_id"`
	UserID      string             `json:"user_id"`
	ProjectID   string             `json:"project_id,omitempty"`
	Title       string             `json:"title"`
	Status      ConversationStatus `json:"status"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// Message is a single turn in a conversation.
type Message struct {
	ID             string      `json:"id"`
	ConversationID string      `json:"conversation_id"`
	Role           MessageRole `json:"role"`
	Content        string      `json:"content"`
	ToolCalls      []ToolCall  `json:"tool_calls,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
}

// ToolCall is an MCP tool invocation by @bravo.
type ToolCall struct {
	ID        string          `json:"id"`
	MessageID string          `json:"message_id"`
	ToolName  string          `json:"tool_name"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output,omitempty"`
	Status    ToolCallStatus  `json:"status"`
	Duration  time.Duration   `json:"duration"`
	Error     string          `json:"error,omitempty"`
}

// AgentConfig is the per-workspace @bravo configuration.
type AgentConfig struct {
	WorkspaceID     string   `json:"workspace_id"`
	Enabled         bool     `json:"enabled"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	DeniedTools     []string `json:"denied_tools,omitempty"`
	RequireApproval []string `json:"require_approval,omitempty"`
	CodeExecEnabled bool     `json:"code_exec_enabled"`
	MaxConcurrent   int      `json:"max_concurrent"`
}

// AgentStore persists agent conversations, messages, tool calls, and config.
type AgentStore interface {
	// Conversations
	CreateConversation(ctx context.Context, conv *Conversation) error
	GetConversation(ctx context.Context, id string) (*Conversation, error)
	ListConversations(ctx context.Context, workspaceID, userID string, limit, offset int) ([]*Conversation, int, error)
	UpdateConversation(ctx context.Context, conv *Conversation) error
	DeleteConversation(ctx context.Context, id string) error

	// Messages
	AddMessage(ctx context.Context, msg *Message) error
	ListMessages(ctx context.Context, conversationID string, limit, offset int) ([]*Message, error)

	// Tool calls
	AddToolCall(ctx context.Context, tc *ToolCall) error
	UpdateToolCall(ctx context.Context, tc *ToolCall) error

	// Config
	GetAgentConfig(ctx context.Context, workspaceID string) (*AgentConfig, error)
	SaveAgentConfig(ctx context.Context, cfg *AgentConfig) error

	Close() error
}
