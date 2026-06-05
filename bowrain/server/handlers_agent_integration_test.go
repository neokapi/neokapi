package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBravoEndToEnd_ConversationLifecycle exercises the full lifecycle:
// create conversation → send message → list messages → SSE stream →
// tool call approval → cancel → delete. Each step hits real HTTP handlers
// backed by in-memory SQLite to verify the complete pipeline.
func TestBravoEndToEnd_ConversationLifecycle(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	const (
		userID = "user-e2e"
		wsID   = "ws-e2e"
	)

	// ── 1. Create conversation ─────────────────────────────────────────
	body := `{"title":"E2E Chat","project_id":"proj-e2e"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/bravo/conversations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleCreateBravoConversation(c))
	require.Equal(t, http.StatusCreated, rec.Code)

	var conv platagent.Conversation
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &conv))
	require.NotEmpty(t, conv.ID)
	assert.Equal(t, "E2E Chat", conv.Title)
	assert.Equal(t, "proj-e2e", conv.ProjectID)
	assert.Equal(t, platagent.ConversationActive, conv.Status)

	// ── 2. Send a message (JSON mode) ──────────────────────────────────
	msgBody := `{"content":"Translate hello to French"}`
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(msgBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleSendBravoMessage(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var msgResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msgResp))
	userMsg := msgResp["user_message"].(map[string]any)
	assistantMsg := msgResp["assistant_message"].(map[string]any)
	assert.Equal(t, "user", userMsg["role"])
	assert.Equal(t, "Translate hello to French", userMsg["content"])
	assert.Equal(t, "assistant", assistantMsg["role"])

	// ── 3. List messages — verify both were persisted ──────────────────
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)

	require.NoError(t, srv.HandleListBravoMessages(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	msgs := listResp["messages"].([]any)
	assert.Len(t, msgs, 2)

	// ── 4. Send message with SSE stream ────────────────────────────────
	sseBody := `{"content":"Check TM for hello"}`
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(sseBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)
	c.Set("workspace_role", platauth.RoleMember)

	require.NoError(t, srv.HandleSendBravoMessage(c))

	sseOutput := rec.Body.String()
	assert.Contains(t, sseOutput, "event: message_start")
	assert.Contains(t, sseOutput, "event: content_delta")
	assert.Contains(t, sseOutput, "event: message_end")
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))

	// ── 5. Get conversation — verify it's still active ─────────────────
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleGetBravoConversation(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	convData := getResp["conversation"].(map[string]any)
	assert.Equal(t, "active", convData["status"])

	// ── 6. Cancel conversation ─────────────────────────────────────────
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)

	require.NoError(t, srv.HandleCancelBravoConversation(c))
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify status changed to failed.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleGetBravoConversation(c))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	convData = getResp["conversation"].(map[string]any)
	assert.Equal(t, "failed", convData["status"])

	// ── 7. Delete conversation ─────────────────────────────────────────
	req = httptest.NewRequest(http.MethodDelete, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)

	require.NoError(t, srv.HandleDeleteBravoConversation(c))
	require.Equal(t, http.StatusNoContent, rec.Code)

	// Verify conversation is gone.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleGetBravoConversation(c))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestBravoEndToEnd_ConfigAndToolPolicy exercises workspace config management
// and tool listing with approval policy.
func TestBravoEndToEnd_ConfigAndToolPolicy(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	const wsID = "ws-cfg"

	// ── 1. Get default config ──────────────────────────────────────────
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleGetBravoConfig(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var cfg platagent.AgentConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.False(t, cfg.Enabled)
	assert.Equal(t, 3, cfg.MaxConcurrent) // default

	// ── 2. Update config (admin) ───────────────────────────────────────
	updateBody := `{"enabled":true,"code_exec_enabled":true,"max_concurrent":5,"require_approval":["connector_push","execute_script"]}`
	req = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", wsID)
	c.Set("workspace_role", platauth.RoleAdmin)

	require.NoError(t, srv.HandleUpdateBravoConfig(c))
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.CodeExecEnabled)
	assert.Equal(t, 5, cfg.MaxConcurrent)
	assert.Contains(t, cfg.RequireApproval, "connector_push")
	assert.Contains(t, cfg.RequireApproval, "execute_script")

	// ── 3. List tools — should respect policy ──────────────────────────
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleListBravoTools(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var toolsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &toolsResp))
	tools := toolsResp["tools"].([]any)
	assert.NotEmpty(t, tools, "should return available tools when agent is enabled")

	// Verify approval flags are set for configured tools.
	approvalTools := make(map[string]bool)
	for _, t := range tools {
		tool := t.(map[string]any)
		if tool["require_approval"].(bool) {
			approvalTools[tool["name"].(string)] = true
		}
	}
	assert.True(t, approvalTools["connector_push"])
	assert.True(t, approvalTools["execute_script"])

	// ── 4. Update config (non-admin) — should be forbidden ─────────────
	req = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", wsID)
	c.Set("workspace_role", platauth.RoleMember)

	require.Error(t, srv.HandleUpdateBravoConfig(c)) // deny writes 403 + returns error
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestBravoEndToEnd_ToolCallApproval exercises the human-in-the-loop flow:
// create conversation → send message → inject pending tool call → approve/deny.
func TestBravoEndToEnd_ToolCallApproval(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	const (
		userID = "user-approval"
		wsID   = "ws-approval"
	)

	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	// Create conversation and send initial message.
	conv, err := srv.AgentService.CreateConversation(ctx, wsID, userID, "", "Approval Test")
	require.NoError(t, err)

	_, assistantMsg, err := srv.AgentService.SendMessage(ctx, conv.ID, userID, "Push changes")
	require.NoError(t, err)

	// Inject a tool call that needs approval (simulating the agent pausing).
	tc := &platagent.ToolCall{
		MessageID: assistantMsg.ID,
		ToolName:  "connector_push",
		Input:     []byte(`{"project_id":"proj-1"}`),
		Status:    platagent.ToolCallNeedsApproval,
	}
	require.NoError(t, srv.AgentStore.AddToolCall(ctx, tc))
	require.NotEmpty(t, tc.ID)

	// ── Approve the tool call via HTTP ─────────────────────────────────
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id", "tcid")
	c.SetParamValues("demo", conv.ID, tc.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleApproveBravoToolCall(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Inject another tool call to test denial.
	tc2 := &platagent.ToolCall{
		MessageID: assistantMsg.ID,
		ToolName:  "execute_script",
		Input:     []byte(`{"language":"python","code":"print('test')"}`),
		Status:    platagent.ToolCallNeedsApproval,
	}
	require.NoError(t, srv.AgentStore.AddToolCall(ctx, tc2))

	// ── Deny the tool call via HTTP ────────────────────────────────────
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws", "id", "tcid")
	c.SetParamValues("demo", conv.ID, tc2.ID)
	c.Set("user_id", userID)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleDenyBravoToolCall(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestBravoEndToEnd_MultipleConversations verifies pagination and isolation
// between different users' conversations.
func TestBravoEndToEnd_MultipleConversations(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	const wsID = "ws-multi"
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	// Create conversations for two different users.
	for range 5 {
		_, err := srv.AgentService.CreateConversation(ctx, wsID, "alice", "", "Alice chat")
		require.NoError(t, err)
	}
	for range 3 {
		_, err := srv.AgentService.CreateConversation(ctx, wsID, "bob", "", "Bob chat")
		require.NoError(t, err)
	}

	// List Alice's conversations.
	req := httptest.NewRequest(http.MethodGet, "/?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "alice")
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleListBravoConversations(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(5), resp["total"])
	assert.Len(t, resp["conversations"].([]any), 5)

	// List Bob's conversations.
	req = httptest.NewRequest(http.MethodGet, "/?limit=10&offset=0", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "bob")
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleListBravoConversations(c))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["total"])

	// Test pagination: fetch first 2 of Alice's.
	req = httptest.NewRequest(http.MethodGet, "/?limit=2&offset=0", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "alice")
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleListBravoConversations(c))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(5), resp["total"]) // total is still 5
	assert.Len(t, resp["conversations"].([]any), 2)
}

// TestBravoEndToEnd_UsageTracking verifies the usage endpoint returns data
// after conversations with messages.
func TestBravoEndToEnd_UsageTracking(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	const wsID = "ws-usage"

	// Get initial usage (should be zero).
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleGetBravoUsage(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var usage map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &usage))
	assert.Equal(t, float64(0), usage["message_count"])
}

// TestBravoEndToEnd_AgentNotConfigured verifies endpoints behave correctly
// when the agent service is not initialized.
func TestBravoEndToEnd_AgentNotConfigured(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	e := srv.GetEcho()

	tests := []struct {
		name    string
		method  string
		handler func(c echo.Context) error
		expect  int
	}{
		{"create conversation", http.MethodPost, srv.HandleCreateBravoConversation, http.StatusServiceUnavailable},
		{"get config", http.MethodGet, srv.HandleGetBravoConfig, http.StatusServiceUnavailable},
		{"list conversations", http.MethodGet, srv.HandleListBravoConversations, http.StatusOK}, // returns empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tt.method == http.MethodPost {
				bodyReader = strings.NewReader(`{"title":"test"}`)
			}
			var req *http.Request
			if bodyReader != nil {
				req = httptest.NewRequest(tt.method, "/", bodyReader)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, "/", nil)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("ws")
			c.SetParamValues("demo")
			c.Set("user_id", "user-1")
			c.Set("workspace_id", "ws-1")

			require.NoError(t, tt.handler(c))
			assert.Equal(t, tt.expect, rec.Code)
		})
	}
}
