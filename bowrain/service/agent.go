package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/id"
)

// AgentEnqueuer enqueues agent job messages. Matches the jobs.Queue.Enqueue signature.
type AgentEnqueuer interface {
	Enqueue(ctx context.Context, payload string) error
}

// AgentService orchestrates agent conversations, messages, and tool policy.
type AgentService struct {
	store        platagent.AgentStore
	eventBus     platev.EventBus
	pool         *AgentPool          // manages ZeroClaw containers (nil when using queue mode)
	tokenStore   *AgentTokenStore    // scoped agent tokens for MCP delegation
	queue        AgentEnqueuer       // Service Bus queue for bravo-jobs (nil = direct/mock mode)
	pubsub       *AgentPubSub        // Redis pub/sub for SSE relay (nil = direct/mock mode)
	billingHooks *billing.UsageHooks // billing credit deduction (nil = disabled)
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
// When set, SendMessageStream routes to real agent containers directly.
func (s *AgentService) SetPool(pool *AgentPool) {
	s.pool = pool
}

// SetQueue configures queue-based agent orchestration.
// When set (and pool is nil), SendMessageStream enqueues jobs to Service Bus
// and subscribes to Redis pub/sub for SSE relay.
func (s *AgentService) SetQueue(queue AgentEnqueuer, pubsub *AgentPubSub) {
	s.queue = queue
	s.pubsub = pubsub
}

// SetBillingHooks configures billing credit deduction for agent usage.
func (s *AgentService) SetBillingHooks(hooks *billing.UsageHooks) {
	s.billingHooks = hooks
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

// SendMessage persists a user message and generates a synchronous agent response.
// Used by JSON API clients. For SSE streaming, use SendMessageStream.
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

	// Synchronous response (no container pool).
	// For real agent responses, use SendMessageStream with a pool.
	assistantMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleAssistant,
		Content:        fmt.Sprintf("I received your message: %q. Use SSE streaming for full agent responses.", content),
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
func (s *AgentService) SendMessageStream(ctx context.Context, conversationID, userID, workspaceID, workspaceRole, content, mode string, bravoCtx map[string]string, sse SSEWriter) error {
	// Persist user message.
	userMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleUser,
		Content:        content,
	}
	if err := s.store.AddMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("add user message: %w", err)
	}

	// Queue mode: enqueue to Service Bus, subscribe to Redis for SSE relay.
	if s.pool == nil && s.queue != nil && s.pubsub != nil {
		return s.sendQueuedStream(ctx, conversationID, userID, workspaceID, workspaceRole, content, mode, bravoCtx, userMsg.ID, sse)
	}

	// No pool and no queue: local mock response.
	if s.pool == nil {
		return s.sendLocalStream(ctx, conversationID, userID, content, sse)
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

	// Stream response from ZeroClaw gateway → SSE to client.
	result, err := s.streamFromGateway(ctx, container, conversationID, userID, content, mode, bravoCtx, sse)
	if err != nil {
		return fmt.Errorf("gateway stream: %w", err)
	}

	// Record token usage.
	if result != nil && (result.InputTokens > 0 || result.OutputTokens > 0) {
		_ = s.store.RecordUsage(ctx, &platagent.UsageRecord{
			WorkspaceID:    workspaceID,
			UserID:         userID,
			ConversationID: conversationID,
			MessageID:      result.MessageID,
			Kind:           "tokens",
			InputTokens:    result.InputTokens,
			OutputTokens:   result.OutputTokens,
		})

		// Deduct billing credits and report to Stripe.
		if s.billingHooks != nil {
			totalTokens := result.InputTokens + result.OutputTokens
			s.billingHooks.DeductTokens(ctx, workspaceID, totalTokens, "bravo_message", conversationID)
		}
	}

	// Update conversation timestamp.
	conv, _ := s.store.GetConversation(ctx, conversationID)
	if conv != nil {
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
			},
			Timestamp: time.Now(),
		})
	}

	return nil
}

// sendLocalStream emits SSE events from a local (non-container) agent response.
// Used when no container pool is configured (e.g., development/testing).
func (s *AgentService) sendLocalStream(ctx context.Context, conversationID, userID, content string, sse SSEWriter) error {
	assistantMsg := &platagent.Message{
		ConversationID: conversationID,
		Role:           platagent.RoleAssistant,
		Content:        fmt.Sprintf("I received your message: %q. No agent container pool is configured.", content),
	}
	if err := s.store.AddMessage(ctx, assistantMsg); err != nil {
		return fmt.Errorf("add assistant message: %w", err)
	}

	_ = sse.WriteEvent(SSEMessageStart, MessageStartData{ID: assistantMsg.ID, Role: "assistant"})
	_ = sse.WriteEvent(SSEContentDelta, ContentDeltaData{Delta: assistantMsg.Content})
	_ = sse.WriteEvent(SSEMessageEnd, MessageEndData{ID: assistantMsg.ID})

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

// sendQueuedStream enqueues an agent job to Service Bus and subscribes to
// Redis pub/sub to relay SSE events back to the client.
func (s *AgentService) sendQueuedStream(ctx context.Context, conversationID, userID, workspaceID, workspaceRole, content, mode string, bravoCtx map[string]string, messageID string, sse SSEWriter) error {
	// Encode the job message.
	jobData := map[string]any{
		"conversation_id": conversationID,
		"message_id":      messageID,
		"workspace_id":    workspaceID,
		"user_id":         userID,
		"workspace_role":  workspaceRole,
		"content":         content,
		"mode":            mode,
	}
	if len(bravoCtx) > 0 {
		jobData["context"] = bravoCtx
	}
	payload, err := json.Marshal(jobData)
	if err != nil {
		return fmt.Errorf("marshal agent job: %w", err)
	}

	// Subscribe to Redis BEFORE enqueuing so we don't miss events.
	events, cancel := s.pubsub.Subscribe(ctx, conversationID)
	defer cancel()

	// Enqueue the job.
	if err := s.queue.Enqueue(ctx, string(payload)); err != nil {
		return fmt.Errorf("enqueue agent job: %w", err)
	}

	slog.InfoContext(ctx, "agent job enqueued, waiting for events", "conversation_id", conversationID)

	// Relay Redis events → SSE until message_end, error, or timeout.
	timeout := 5 * time.Minute
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				slog.DebugContext(ctx, "agent SSE relay: channel closed", "conversation_id", conversationID)
				return nil // channel closed
			}
			slog.DebugContext(ctx, "agent SSE relay event", "event", evt.Event, "conversation_id", conversationID)
			_ = sse.WriteEvent(evt.Event, evt.Data)

			// Terminal events: stop relaying.
			if evt.Event == SSEMessageEnd || evt.Event == SSEError {
				return nil
			}

		case <-timer.C:
			slog.WarnContext(ctx, "agent SSE relay: timeout", "conversation_id", conversationID)
			_ = sse.WriteEvent(SSEError, ErrorData{Error: "agent response timed out"})
			return nil

		case <-ctx.Done():
			slog.DebugContext(ctx, "agent SSE relay: context cancelled", "conversation_id", conversationID, "error", ctx.Err())
			return ctx.Err()
		}
	}
}

// cleanupConversation releases pool containers and revokes tokens for a conversation.
// It also records container time usage if a container was running.
func (s *AgentService) cleanupConversation(ctx context.Context, conversationID string) {
	if s.pool != nil {
		container, _ := s.pool.Release(ctx, conversationID)
		if container != nil && !container.CreatedAt.IsZero() {
			duration := time.Since(container.CreatedAt)
			_ = s.store.RecordUsage(ctx, &platagent.UsageRecord{
				WorkspaceID:    container.WorkspaceID,
				UserID:         container.UserID,
				ConversationID: conversationID,
				Kind:           "container_time",
				DurationSec:    duration.Seconds(),
			})

			// Deduct billing credits for container time.
			if s.billingHooks != nil {
				s.billingHooks.DeductContainerTime(ctx, container.WorkspaceID, duration, conversationID)
			}
		}
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

// GetUsageSummary returns aggregated usage for a workspace over a time range.
func (s *AgentService) GetUsageSummary(ctx context.Context, workspaceID string, from, to time.Time) (*platagent.UsageSummary, error) {
	return s.store.GetUsageSummary(ctx, workspaceID, from, to)
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
	if slices.Contains(cfg.DeniedTools, toolName) {
		return "deny"
	}
	if slices.Contains(cfg.RequireApproval, toolName) {
		return "approve"
	}
	if len(cfg.AllowedTools) > 0 {
		if slices.Contains(cfg.AllowedTools, toolName) {
			return "allow"
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
