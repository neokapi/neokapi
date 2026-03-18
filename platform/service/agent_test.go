package service

import (
	"context"
	"testing"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	platagent "github.com/neokapi/neokapi/platform/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAgentStore(t *testing.T) platagent.AgentStore {
	t.Helper()
	s, err := bragent.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAgentServiceCreateConversation(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "proj1", "Test chat")
	require.NoError(t, err)
	assert.NotEmpty(t, conv.ID)
	assert.Equal(t, "Test chat", conv.Title)
	assert.Equal(t, platagent.ConversationActive, conv.Status)
	assert.Equal(t, "proj1", conv.ProjectID)
}

func TestAgentServiceCreateConversation_DefaultTitle(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "")
	require.NoError(t, err)
	assert.Equal(t, "New conversation", conv.Title)
}

func TestAgentServiceListConversations(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	_, err := svc.CreateConversation(ctx, "ws1", "user1", "", "First")
	require.NoError(t, err)
	_, err = svc.CreateConversation(ctx, "ws1", "user1", "", "Second")
	require.NoError(t, err)
	_, err = svc.CreateConversation(ctx, "ws1", "user2", "", "Other user")
	require.NoError(t, err)

	convs, total, err := svc.ListConversations(ctx, "ws1", "user1", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, convs, 2)
}

func TestAgentServiceDeleteConversation(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "To delete")
	require.NoError(t, err)

	require.NoError(t, svc.DeleteConversation(ctx, conv.ID))
	_, err = svc.GetConversation(ctx, conv.ID)
	assert.Error(t, err)
}

func TestAgentServiceSendMessage(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	userMsg, assistantMsg, err := svc.SendMessage(ctx, conv.ID, "user1", "Hello bravo")
	require.NoError(t, err)
	assert.Equal(t, platagent.RoleUser, userMsg.Role)
	assert.Equal(t, "Hello bravo", userMsg.Content)
	assert.Equal(t, platagent.RoleAssistant, assistantMsg.Role)
	assert.Contains(t, assistantMsg.Content, "Hello bravo")

	// Verify messages are persisted.
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

func TestAgentServiceCancelConversation(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	require.NoError(t, svc.CancelConversation(ctx, conv.ID))

	got, err := svc.GetConversation(ctx, conv.ID)
	require.NoError(t, err)
	assert.Equal(t, platagent.ConversationFailed, got.Status)
}

func TestAgentServiceConfig(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	// Default config.
	cfg, err := svc.GetConfig(ctx, "ws1")
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)

	// Save config.
	cfg.Enabled = true
	cfg.AllowedTools = []string{"list_projects"}
	cfg.MaxConcurrent = 5
	require.NoError(t, svc.SaveConfig(ctx, cfg))

	got, err := svc.GetConfig(ctx, "ws1")
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, []string{"list_projects"}, got.AllowedTools)
	assert.Equal(t, 5, got.MaxConcurrent)
}

func TestAgentServiceListAvailableTools(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	// Agent disabled → no tools.
	tools, err := svc.ListAvailableTools(ctx, "ws1", []string{"list_projects", "run_flow"})
	require.NoError(t, err)
	assert.Empty(t, tools)

	// Enable agent.
	cfg := &platagent.AgentConfig{
		WorkspaceID:     "ws1",
		Enabled:         true,
		RequireApproval: []string{"run_flow"},
	}
	require.NoError(t, svc.SaveConfig(ctx, cfg))

	tools, err = svc.ListAvailableTools(ctx, "ws1", []string{"list_projects", "run_flow"})
	require.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.False(t, tools[0].RequireApproval)
	assert.True(t, tools[1].RequireApproval)
}

func TestAgentServiceApproveAndDenyToolCall(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	// Send a message so we have a message ID for tool calls.
	_, assistantMsg, err := svc.SendMessage(ctx, conv.ID, "user1", "Do something")
	require.NoError(t, err)

	// Add a pending tool call directly via store.
	tc := &platagent.ToolCall{
		MessageID: assistantMsg.ID,
		ToolName:  "connector_push",
		Input:     []byte(`{}`),
		Status:    platagent.ToolCallNeedsApproval,
	}
	require.NoError(t, store.AddToolCall(ctx, tc))

	// Approve it.
	require.NoError(t, svc.ApproveToolCall(ctx, conv.ID, tc.ID, "user1"))

	// Add another tool call to test deny.
	tc2 := &platagent.ToolCall{
		MessageID: assistantMsg.ID,
		ToolName:  "execute_script",
		Input:     []byte(`{}`),
		Status:    platagent.ToolCallNeedsApproval,
	}
	require.NoError(t, store.AddToolCall(ctx, tc2))

	require.NoError(t, svc.DenyToolCall(ctx, conv.ID, tc2.ID, "user1"))
}
