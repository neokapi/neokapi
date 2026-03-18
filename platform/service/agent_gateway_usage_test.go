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

func TestStreamFromGateway_CapturesUsage(t *testing.T) {
	store, err := bragent.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	svc := NewAgentService(store, nil)
	ctx := context.Background()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	// Mock gateway that sends usage in message_end.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "event: message_start\ndata: {\"id\":\"msg-u1\",\"role\":\"assistant\"}\n\n")
		fmt.Fprint(w, "event: content_delta\ndata: {\"delta\":\"Done.\"}\n\n")
		fmt.Fprint(w, "event: message_end\ndata: {\"id\":\"msg-u1\",\"usage\":{\"input_tokens\":1234,\"output_tokens\":567}}\n\n")
	}))
	defer ts.Close()

	container := &AgentContainer{
		ID:             "test-container",
		GatewayURL:     ts.URL,
		ConversationID: conv.ID,
	}

	var buf bytes.Buffer
	result, err := svc.streamFromGateway(ctx, container, conv.ID, "user1", "Hello", NewSSEWriter(&buf))
	require.NoError(t, err)

	// Verify usage was captured.
	assert.Equal(t, 1234, result.InputTokens)
	assert.Equal(t, 567, result.OutputTokens)
	assert.Equal(t, "msg-u1", result.MessageID)

	// Verify persisted message has token counts.
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, 1234, msgs[0].InputTokens)
	assert.Equal(t, 567, msgs[0].OutputTokens)

	// Verify SSE output includes usage.
	output := buf.String()
	assert.Contains(t, output, "input_tokens")
	assert.Contains(t, output, "1234")
}
