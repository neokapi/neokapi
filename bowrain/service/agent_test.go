package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockGatewayServer starts a test HTTP server that mimics the ZeroClaw
// gateway /webhook JSON response.
func newMockGatewayServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"model":"gpt-4o","response":"Hello from gateway"}`)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// newPoolWithGateway creates a pool whose mock runtime returns the given
// gateway URL for all spawned containers.
func newPoolWithGateway(t *testing.T, gatewayURL string, maxPerWS int) *AgentPool {
	t.Helper()
	rt := &gatewayMockRuntime{
		mockRuntime: newMockRuntime(),
		gatewayURL:  gatewayURL,
	}
	return NewAgentPool(AgentPoolConfig{
		Runtime:         rt,
		MaxPerWorkspace: maxPerWS,
	})
}

// gatewayMockRuntime wraps mockRuntime to set GatewayURL on spawned containers.
type gatewayMockRuntime struct {
	*mockRuntime
	gatewayURL string
}

func (r *gatewayMockRuntime) Spawn(ctx context.Context, cfg ContainerConfig) (*AgentContainer, error) {
	c, err := r.mockRuntime.Spawn(ctx, cfg)
	if err != nil {
		return nil, err
	}
	c.GatewayURL = r.gatewayURL
	return c, nil
}

func newTestAgentStore(t *testing.T) platagent.AgentStore {
	t.Helper()
	pgdb := pgtest.NewTestDB(t)
	s, err := bragent.NewStore(pgdb)
	require.NoError(t, err)
	return s
}

func TestAgentServiceCreateConversation(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

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
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "")
	require.NoError(t, err)
	assert.Equal(t, "New conversation", conv.Title)
}

func TestAgentServiceListConversations(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

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
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "To delete")
	require.NoError(t, err)

	require.NoError(t, svc.DeleteConversation(ctx, conv.ID))
	_, err = svc.GetConversation(ctx, conv.ID)
	assert.Error(t, err)
}

func TestAgentServiceSendMessage(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	userMsg, assistantMsg, err := svc.SendMessage(ctx, conv.ID, "user1", "Hello bravo")
	require.NoError(t, err)
	assert.Equal(t, platagent.RoleUser, userMsg.Role)
	assert.Equal(t, "Hello bravo", userMsg.Content)
	assert.Equal(t, platagent.RoleAssistant, assistantMsg.Role)
	assert.Contains(t, assistantMsg.Content, "Hello bravo")
	assert.Contains(t, assistantMsg.Content, "SSE streaming")

	// Verify messages are persisted.
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

func TestAgentServiceCancelConversation(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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

func TestAgentServiceSetPoolAndTokenStore(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)

	// TokenStore is initialized by default.
	assert.NotNil(t, svc.TokenStore())

	// Pool is nil by default.
	pool := NewAgentPool(AgentPoolConfig{
		Runtime:         newMockRuntime(),
		MaxPerWorkspace: 3,
	})
	svc.SetPool(pool)
	// Verify pool is set by acquiring a container.
	c, err := pool.Acquire(t.Context(), ContainerConfig{
		ConversationID: "conv-1",
		WorkspaceID:    "ws-1",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, c.ID)
}

func TestAgentServiceSendMessageStream_LocalMode(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	err = svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "Hello local", "ask", nil, sse)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "event: content_delta")
	assert.Contains(t, output, "Hello local")
	assert.Contains(t, output, "event: message_end")

	// Verify messages are persisted (user + assistant).
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
}

func TestAgentServiceSendMessageStream_WithPool(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

	gw := newMockGatewayServer(t)
	pool := newPoolWithGateway(t, gw.URL, 3)
	svc.SetPool(pool)

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	err = svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "Hello pool", "ask", nil, sse)
	require.NoError(t, err)

	// Container should have been acquired.
	_, ok := pool.Get(conv.ID)
	assert.True(t, ok)

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "Hello from gateway")
}

func TestAgentServiceSendMessageStream_PoolLimitError(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

	gw := newMockGatewayServer(t)
	pool := newPoolWithGateway(t, gw.URL, 1)
	svc.SetPool(pool)

	// Fill the pool.
	conv1, _ := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat1")
	var buf1 bytes.Buffer
	require.NoError(t, svc.SendMessageStream(ctx, conv1.ID, "user1", "ws1", "member", "msg1", "ask", nil, NewSSEWriter(&buf1)))

	// Second conversation should hit the limit.
	conv2, _ := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat2")
	var buf2 bytes.Buffer
	err := svc.SendMessageStream(ctx, conv2.ID, "user1", "ws1", "member", "msg2", "ask", nil, NewSSEWriter(&buf2))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent")
}

func TestAgentServiceDeleteConversation_CleansUpPoolAndTokens(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

	gw := newMockGatewayServer(t)
	pool := newPoolWithGateway(t, gw.URL, 3)
	svc.SetPool(pool)

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	// Stream a message to create container + token.
	var buf bytes.Buffer
	require.NoError(t, svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "hi", "ask", nil, NewSSEWriter(&buf)))

	_, ok := pool.Get(conv.ID)
	assert.True(t, ok)

	// Delete should clean up.
	require.NoError(t, svc.DeleteConversation(ctx, conv.ID))
	_, ok = pool.Get(conv.ID)
	assert.False(t, ok)
	assert.Equal(t, 0, pool.ActiveCount("ws1"))
}

func TestAgentServiceCancelConversation_CleansUpPool(t *testing.T) {
	store := newTestAgentStore(t)
	svc := NewAgentService(store, nil)
	ctx := t.Context()

	gw := newMockGatewayServer(t)
	pool := newPoolWithGateway(t, gw.URL, 3)
	svc.SetPool(pool)

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "hi", "ask", nil, NewSSEWriter(&buf)))

	require.NoError(t, svc.CancelConversation(ctx, conv.ID))

	_, ok := pool.Get(conv.ID)
	assert.False(t, ok)

	got, err := svc.GetConversation(ctx, conv.ID)
	require.NoError(t, err)
	assert.Equal(t, platagent.ConversationFailed, got.Status)
}

func TestToolNames(t *testing.T) {
	names := ToolNames()
	assert.Greater(t, len(names), 20)
	assert.Contains(t, names, "list_projects")
	assert.Contains(t, names, "execute_script")
	assert.Contains(t, names, "check_vocabulary")
}
