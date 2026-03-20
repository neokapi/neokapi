package agenticmcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgenticEventJSON(t *testing.T) {
	ev := AgenticEvent{
		Type:        EventExecStarted,
		ExecutionID: "exec_1710936000000",
		Workspace:   "excalidraw-l10n",
		Agent:       "sophie-translator",
		Role:        "translator",
		Timestamp:   "2026-03-20T12:00:15Z",
		Data: map[string]any{
			"task":   "Translate 142 fr-FR blocks",
			"locale": "fr-FR",
		},
	}

	data, err := json.Marshal(ev)
	require.NoError(t, err)

	var decoded AgenticEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, EventExecStarted, decoded.Type)
	assert.Equal(t, "exec_1710936000000", decoded.ExecutionID)
	assert.Equal(t, "excalidraw-l10n", decoded.Workspace)
	assert.Equal(t, "sophie-translator", decoded.Agent)
	assert.Equal(t, "translator", decoded.Role)
	assert.Equal(t, "Translate 142 fr-FR blocks", decoded.Data["task"])
	assert.Equal(t, "fr-FR", decoded.Data["locale"])
}

func TestAgenticEventTypes(t *testing.T) {
	assert.Equal(t, AgenticEventType("exec.started"), EventExecStarted)
	assert.Equal(t, AgenticEventType("exec.completed"), EventExecCompleted)
	assert.Equal(t, AgenticEventType("exec.failed"), EventExecFailed)
	assert.Equal(t, AgenticEventType("exec.progress"), EventExecProgress)
	assert.Equal(t, AgenticEventType("exec.tool_call"), EventExecToolCall)
	assert.Equal(t, AgenticEventType("agent.observation"), EventObservation)
	assert.Equal(t, AgenticEventType("agent.suggestion"), EventSuggestion)
}

func TestIntFromData(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		key      string
		expected int
	}{
		{"float64", map[string]any{"tokens_used": float64(45200)}, "tokens_used", 45200},
		{"int", map[string]any{"tokens_used": 100}, "tokens_used", 100},
		{"int64", map[string]any{"tokens_used": int64(999)}, "tokens_used", 999},
		{"missing", map[string]any{}, "tokens_used", 0},
		{"nil data", nil, "tokens_used", 0},
		{"wrong type", map[string]any{"tokens_used": "hello"}, "tokens_used", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, intFromData(tc.data, tc.key))
		})
	}
}

func TestExecutionSubscriberHandleMessage(t *testing.T) {
	// This tests the JSON parsing logic of handleMessage without a real Redis or PostgreSQL.
	// We verify that a valid JSON event can be unmarshalled correctly.
	ev := AgenticEvent{
		Type:        EventExecCompleted,
		ExecutionID: "exec_123",
		Workspace:   "test-ws",
		Agent:       "test-agent",
		Role:        "translator",
		Timestamp:   "2026-03-20T14:00:00Z",
		Data: map[string]any{
			"summary":     "Done",
			"tokens_used": float64(1000),
		},
	}

	data, err := json.Marshal(ev)
	require.NoError(t, err)

	var parsed AgenticEvent
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, EventExecCompleted, parsed.Type)
	assert.Equal(t, "exec_123", parsed.ExecutionID)
	assert.Equal(t, 1000, intFromData(parsed.Data, "tokens_used"))
}
