package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	platagent "github.com/neokapi/neokapi/bowrain/core/agent"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAgentTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	pgdb := pgtest.NewTestDB(t)
	agentStore, err := bragent.NewStore(pgdb)
	require.NoError(t, err)
	srv.AgentStore = agentStore
	srv.AgentService = service.NewAgentService(agentStore, nil)

	return srv
}

func TestHandleCreateBravoConversation(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	body := `{"title":"My chat","project_id":"proj-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/bravo/conversations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "user-1")
	c.Set("workspace_id", "ws-1")

	err := srv.HandleCreateBravoConversation(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var conv platagent.Conversation
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &conv))
	assert.NotEmpty(t, conv.ID)
	assert.Equal(t, "My chat", conv.Title)
	assert.Equal(t, "proj-1", conv.ProjectID)
}

func TestHandleListBravoConversations_Empty(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/bravo/conversations", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "user-1")
	c.Set("workspace_id", "ws-1")

	err := srv.HandleListBravoConversations(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["total"])
}

func TestHandleGetBravoConversation(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()
	ctx := e.NewContext(nil, nil).Request()
	_ = ctx

	// Create a conversation first.
	conv, err := srv.AgentService.CreateConversation(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"ws-1", "user-1", "", "Test")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/bravo/conversations/"+conv.ID, nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", "user-1")
	c.Set("workspace_id", "ws-1")

	err = srv.HandleGetBravoConversation(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	convData := resp["conversation"].(map[string]any)
	assert.Equal(t, conv.ID, convData["id"])
}

func TestHandleSendBravoMessage(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	conv, err := srv.AgentService.CreateConversation(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"ws-1", "user-1", "", "Chat")
	require.NoError(t, err)

	body := `{"content":"Hello bravo!"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", "user-1")
	c.Set("workspace_id", "ws-1")

	err = srv.HandleSendBravoMessage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	userMsg := resp["user_message"].(map[string]any)
	assistantMsg := resp["assistant_message"].(map[string]any)
	assert.Equal(t, "user", userMsg["role"])
	assert.Equal(t, "Hello bravo!", userMsg["content"])
	assert.Equal(t, "assistant", assistantMsg["role"])
}

func TestHandleSendBravoMessage_EmptyContent(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	conv, err := srv.AgentService.CreateConversation(
		httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		"ws-1", "user-1", "", "Chat")
	require.NoError(t, err)

	body := `{"content":""}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", "user-1")
	c.Set("workspace_id", "ws-1")

	err = srv.HandleSendBravoMessage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleListBravoMessages(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()
	reqCtx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	conv, err := srv.AgentService.CreateConversation(reqCtx, "ws-1", "user-1", "", "Chat")
	require.NoError(t, err)
	_, _, err = srv.AgentService.SendMessage(reqCtx, conv.ID, "user-1", "Msg 1")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)

	err = srv.HandleListBravoMessages(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	msgs := resp["messages"].([]any)
	assert.Len(t, msgs, 2) // user + assistant
}

func TestHandleDeleteBravoConversation(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()
	reqCtx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	conv, err := srv.AgentService.CreateConversation(reqCtx, "ws-1", "user-1", "", "Chat")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)

	err = srv.HandleDeleteBravoConversation(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleCancelBravoConversation(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()
	reqCtx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	conv, err := srv.AgentService.CreateConversation(reqCtx, "ws-1", "user-1", "", "Chat")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)

	err = srv.HandleCancelBravoConversation(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleGetBravoConfig(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", "ws-1")

	err := srv.HandleGetBravoConfig(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var cfg platagent.AgentConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.False(t, cfg.Enabled)
	assert.Equal(t, 3, cfg.MaxConcurrent)
}

func TestHandleUpdateBravoConfig(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	body := `{"enabled":true,"max_concurrent":10}`
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", "ws-1")
	c.Set("workspace_role", platauth.RoleAdmin)

	err := srv.HandleUpdateBravoConfig(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var cfg platagent.AgentConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 10, cfg.MaxConcurrent)
}

func TestHandleUpdateBravoConfig_RequiresAdmin(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	body := `{"enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", "ws-1")
	c.Set("workspace_role", platauth.RoleMember)

	// Deny writes 403 and returns a non-nil error (so the handler aborts).
	require.Error(t, srv.HandleUpdateBravoConfig(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleListBravoTools(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", "ws-1")

	err := srv.HandleListBravoTools(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleSendBravoMessage_SSEStream(t *testing.T) {
	srv := setupAgentTestServer(t)
	e := srv.GetEcho()
	reqCtx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	conv, err := srv.AgentService.CreateConversation(reqCtx, "ws-1", "user-1", "", "Chat")
	require.NoError(t, err)

	body := `{"content":"Hello SSE!"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "id")
	c.SetParamValues("demo", conv.ID)
	c.Set("user_id", "user-1")
	c.Set("workspace_id", "ws-1")
	c.Set("workspace_role", platauth.RoleMember)

	err = srv.HandleSendBravoMessage(c)
	require.NoError(t, err)

	output := rec.Body.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "event: content_delta")
	assert.Contains(t, output, "Hello SSE!")
	assert.Contains(t, output, "event: message_end")
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
}

func TestHandleAgentNotConfigured(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("workspace_id", "ws-1")

	err := srv.HandleGetBravoConfig(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
