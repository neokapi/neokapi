package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamFromGateway_Success(t *testing.T) {
	store, err := bragent.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	// Mock gateway server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "text/event-stream", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "event: message_start\ndata: {\"id\":\"msg-1\",\"role\":\"assistant\"}\n\n")
		fmt.Fprint(w, "event: content_delta\ndata: {\"delta\":\"Hello \"}\n\n")
		fmt.Fprint(w, "event: content_delta\ndata: {\"delta\":\"world!\"}\n\n")
		fmt.Fprint(w, "event: tool_call_start\ndata: {\"id\":\"tc-1\",\"tool\":\"list_projects\"}\n\n")
		fmt.Fprint(w, "event: tool_call_end\ndata: {\"id\":\"tc-1\",\"status\":\"completed\"}\n\n")
		fmt.Fprint(w, "event: message_end\ndata: {\"id\":\"msg-1\"}\n\n")
	}))
	defer ts.Close()

	container := &AgentContainer{
		ID:             "test-container",
		GatewayURL:     ts.URL,
		ConversationID: conv.ID,
	}

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	err = svc.streamFromGateway(ctx, container, conv.ID, "user1", "Hello", sse)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "Hello ")
	assert.Contains(t, output, "world!")
	assert.Contains(t, output, "event: tool_call_start")
	assert.Contains(t, output, "event: tool_call_end")
	assert.Contains(t, output, "event: message_end")

	// Assistant message should be persisted.
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 1) // assistant message
	assert.Equal(t, "Hello world!", msgs[0].Content)
}

func TestStreamFromGateway_ServerError(t *testing.T) {
	store, err := bragent.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	svc := NewAgentService(store, nil)
	ctx := context.Background()

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
	err = svc.streamFromGateway(ctx, container, conv.ID, "user1", "Hello", NewSSEWriter(&buf))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
