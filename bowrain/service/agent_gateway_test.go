package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamFromGateway_Success(t *testing.T) {
	pgdb := pgtest.NewTestDB(t)
	store, err := bragent.NewStore(pgdb)
	require.NoError(t, err)

	svc := NewAgentService(store, nil)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	// Mock gateway server (JSON webhook response).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"model":"gpt-4o","response":"Hello world!"}`)
	}))
	defer ts.Close()

	container := &AgentContainer{
		ID:             "test-container",
		GatewayURL:     ts.URL,
		ConversationID: conv.ID,
	}

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	result, err := svc.streamFromGateway(ctx, container, conv.ID, "user1", "Hello", "ask", nil, sse)
	require.NoError(t, err)
	assert.NotEmpty(t, result.MessageID)

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "Hello world!")
	assert.Contains(t, output, "event: message_end")

	// Assistant message should be persisted.
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, "Hello world!", msgs[0].Content)
}

func TestModePrefix(t *testing.T) {
	tests := []struct {
		mode     string
		contains string
		empty    bool
	}{
		{mode: "ask", contains: "Mode: Ask"},
		{mode: "coworker", contains: "Mode: Co-worker"},
		{mode: "bravo", contains: "Mode: Brand Voice"},
		{mode: "", empty: true},
		{mode: "unknown", empty: true},
	}
	for _, tt := range tests {
		t.Run("mode_"+tt.mode, func(t *testing.T) {
			got := modePrefix(tt.mode)
			if tt.empty {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.contains)
				assert.Greater(t, len(got), 20, "prefix should be a substantial instruction")
			}
		})
	}
}

func TestModePrefix_AskReadOnly(t *testing.T) {
	prefix := modePrefix("ask")
	assert.Contains(t, prefix, "do not execute any changes")
}

func TestModePrefix_CoworkerFullAccess(t *testing.T) {
	prefix := modePrefix("coworker")
	assert.Contains(t, prefix, "manage projects")
	assert.Contains(t, prefix, "Confirm before any destructive")
}

func TestModePrefix_BravoBrandVoice(t *testing.T) {
	prefix := modePrefix("bravo")
	assert.Contains(t, prefix, "brand voice")
	assert.Contains(t, prefix, "check_vocabulary")
}

func TestStreamFromGateway_ServerError(t *testing.T) {
	pgdb := pgtest.NewTestDB(t)
	store, err := bragent.NewStore(pgdb)
	require.NoError(t, err)

	svc := NewAgentService(store, nil)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal error")
	}))
	defer ts.Close()

	container := &AgentContainer{
		ID:             "test-container",
		GatewayURL:     ts.URL,
		ConversationID: conv.ID,
	}

	var buf bytes.Buffer
	_, err = svc.streamFromGateway(ctx, container, conv.ID, "user1", "Hello", "ask", nil, NewSSEWriter(&buf))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
