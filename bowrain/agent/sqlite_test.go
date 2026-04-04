package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
)

func TestSQLiteStoreConversations(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	// Create conversation.
	conv := &platagent.Conversation{
		WorkspaceID: "ws1",
		UserID:      "user1",
		Title:       "Test conversation",
	}
	require.NoError(t, store.CreateConversation(ctx, conv))
	assert.NotEmpty(t, conv.ID)
	assert.Equal(t, platagent.ConversationActive, conv.Status)

	// Get conversation.
	got, err := store.GetConversation(ctx, conv.ID)
	require.NoError(t, err)
	assert.Equal(t, conv.ID, got.ID)
	assert.Equal(t, "Test conversation", got.Title)

	// List conversations.
	convs, total, err := store.ListConversations(ctx, "ws1", "user1", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, convs, 1)

	// Update conversation.
	conv.Title = "Updated title"
	conv.Status = platagent.ConversationCompleted
	require.NoError(t, store.UpdateConversation(ctx, conv))

	got, err = store.GetConversation(ctx, conv.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated title", got.Title)
	assert.Equal(t, platagent.ConversationCompleted, got.Status)

	// Delete conversation.
	require.NoError(t, store.DeleteConversation(ctx, conv.ID))
	_, err = store.GetConversation(ctx, conv.ID)
	assert.Error(t, err)
}

func TestSQLiteStoreMessages(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	conv := &platagent.Conversation{
		WorkspaceID: "ws1",
		UserID:      "user1",
	}
	require.NoError(t, store.CreateConversation(ctx, conv))

	// Add messages.
	msg1 := &platagent.Message{
		ConversationID: conv.ID,
		Role:           platagent.RoleUser,
		Content:        "Hello",
	}
	require.NoError(t, store.AddMessage(ctx, msg1))
	assert.NotEmpty(t, msg1.ID)

	msg2 := &platagent.Message{
		ConversationID: conv.ID,
		Role:           platagent.RoleAssistant,
		Content:        "Hi there!",
	}
	require.NoError(t, store.AddMessage(ctx, msg2))

	// List messages.
	msgs, err := store.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "Hello", msgs[0].Content)
	assert.Equal(t, "Hi there!", msgs[1].Content)
}

func TestSQLiteStoreToolCalls(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	conv := &platagent.Conversation{WorkspaceID: "ws1", UserID: "user1"}
	require.NoError(t, store.CreateConversation(ctx, conv))

	msg := &platagent.Message{ConversationID: conv.ID, Role: platagent.RoleAssistant, Content: "Running flow..."}
	require.NoError(t, store.AddMessage(ctx, msg))

	tc := &platagent.ToolCall{
		MessageID: msg.ID,
		ToolName:  "run_flow",
		Input:     []byte(`{"flow":"pseudo-translate"}`),
		Status:    platagent.ToolCallRunning,
	}
	require.NoError(t, store.AddToolCall(ctx, tc))
	assert.NotEmpty(t, tc.ID)

	// Update tool call.
	tc.Status = platagent.ToolCallCompleted
	tc.Output = []byte(`{"blocks_processed":45}`)
	require.NoError(t, store.UpdateToolCall(ctx, tc))

	// Verify through message listing.
	msgs, err := store.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].ToolCalls, 1)
	assert.Equal(t, "run_flow", msgs[0].ToolCalls[0].ToolName)
	assert.Equal(t, platagent.ToolCallCompleted, msgs[0].ToolCalls[0].Status)
}

func TestSQLiteStoreAgentConfig(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := t.Context()

	// Get default config (no row exists).
	cfg, err := store.GetAgentConfig(ctx, "ws1")
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, 3, cfg.MaxConcurrent)

	// Save config.
	cfg.Enabled = true
	cfg.AllowedTools = []string{"list_projects", "run_flow"}
	cfg.RequireApproval = []string{"connector_push"}
	cfg.MaxConcurrent = 5
	require.NoError(t, store.SaveAgentConfig(ctx, cfg))

	// Read back.
	got, err := store.GetAgentConfig(ctx, "ws1")
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, []string{"list_projects", "run_flow"}, got.AllowedTools)
	assert.Equal(t, []string{"connector_push"}, got.RequireApproval)
	assert.Equal(t, 5, got.MaxConcurrent)

	// Update config (upsert).
	cfg.Enabled = false
	require.NoError(t, store.SaveAgentConfig(ctx, cfg))
	got, err = store.GetAgentConfig(ctx, "ws1")
	require.NoError(t, err)
	assert.False(t, got.Enabled)
}
