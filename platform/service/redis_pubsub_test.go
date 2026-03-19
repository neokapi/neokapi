package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelName(t *testing.T) {
	tests := []struct {
		conversationID string
		expected       string
	}{
		{"conv-123", "bravo:sse:conv-123"},
		{"abc", "bravo:sse:abc"},
		{"", "bravo:sse:"},
	}
	for _, tt := range tests {
		t.Run(tt.conversationID, func(t *testing.T) {
			assert.Equal(t, tt.expected, channelName(tt.conversationID))
		})
	}
}

func TestSSEEvent_Serialization(t *testing.T) {
	// Test that SSEEvent round-trips through JSON correctly.
	original := SSEEvent{
		Event: SSEContentDelta,
		Data:  json.RawMessage(`{"delta":"Hello world"}`),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded SSEEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Event, decoded.Event)
	assert.JSONEq(t, string(original.Data), string(decoded.Data))
}

func TestSSEEvent_SerializationAllEventTypes(t *testing.T) {
	tests := []struct {
		name  string
		event SSEEvent
	}{
		{
			name: "message_start",
			event: SSEEvent{
				Event: SSEMessageStart,
				Data:  json.RawMessage(`{"id":"msg-1","role":"assistant"}`),
			},
		},
		{
			name: "content_delta",
			event: SSEEvent{
				Event: SSEContentDelta,
				Data:  json.RawMessage(`{"delta":"some text"}`),
			},
		},
		{
			name: "message_end",
			event: SSEEvent{
				Event: SSEMessageEnd,
				Data:  json.RawMessage(`{"id":"msg-1"}`),
			},
		},
		{
			name: "error",
			event: SSEEvent{
				Event: SSEError,
				Data:  json.RawMessage(`{"error":"something went wrong"}`),
			},
		},
		{
			name: "tool_call_start",
			event: SSEEvent{
				Event: SSEToolCallStart,
				Data:  json.RawMessage(`{"id":"tc-1","tool":"run_flow","input":{}}`),
			},
		},
		{
			name: "tool_call_end",
			event: SSEEvent{
				Event: SSEToolCallEnd,
				Data:  json.RawMessage(`{"id":"tc-1","status":"completed","duration_ms":100}`),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			require.NoError(t, err)

			var decoded SSEEvent
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tt.event.Event, decoded.Event)
			assert.JSONEq(t, string(tt.event.Data), string(decoded.Data))
		})
	}
}

func TestSSEEvent_EmptyData(t *testing.T) {
	evt := SSEEvent{Event: SSEMessageEnd, Data: json.RawMessage(`{}`)}
	data, err := json.Marshal(evt)
	require.NoError(t, err)

	var decoded SSEEvent
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, SSEMessageEnd, decoded.Event)
}

func TestNewAgentPubSub(t *testing.T) {
	// Verify construction does not panic with nil client.
	// Real Redis tests require an integration test with a running Redis instance.
	// Here we just ensure the struct is properly initialized.
	ps := NewAgentPubSub(nil)
	assert.NotNil(t, ps)
}
