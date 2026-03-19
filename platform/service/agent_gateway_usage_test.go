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

	// Mock gateway that returns JSON webhook response.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"model":"gpt-4o","response":"Done."}`)
	}))
	defer ts.Close()

	container := &AgentContainer{
		ID:             "test-container",
		GatewayURL:     ts.URL,
		ConversationID: conv.ID,
	}

	var buf bytes.Buffer
	result, err := svc.streamFromGateway(ctx, container, conv.ID, "user1", "Hello", "ask", NewSSEWriter(&buf))
	require.NoError(t, err)

	// JSON webhook response does not include usage data.
	assert.Equal(t, 0, result.InputTokens)
	assert.Equal(t, 0, result.OutputTokens)
	assert.NotEmpty(t, result.MessageID)

	// Verify persisted message.
	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Done.", msgs[0].Content)

	// Verify SSE output includes the response content.
	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "Done.")
}
