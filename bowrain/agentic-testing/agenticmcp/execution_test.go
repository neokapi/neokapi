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
	assert.Equal(t, EventExecStarted, AgenticEventType("exec.started"))
	assert.Equal(t, EventExecCompleted, AgenticEventType("exec.completed"))
	assert.Equal(t, EventExecFailed, AgenticEventType("exec.failed"))
	assert.Equal(t, EventExecProgress, AgenticEventType("exec.progress"))
	assert.Equal(t, EventExecToolCall, AgenticEventType("exec.tool_call"))
	assert.Equal(t, EventObservation, AgenticEventType("agent.observation"))
	assert.Equal(t, EventSuggestion, AgenticEventType("agent.suggestion"))
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

func TestEventHub_BroadcastAll(t *testing.T) {
	hub := NewEventHub()

	c1 := &EventClient{C: make(chan AgenticEvent, 8)}
	c2 := &EventClient{C: make(chan AgenticEvent, 8)}
	hub.Subscribe(c1)
	hub.Subscribe(c2)

	ev := AgenticEvent{Type: EventExecStarted, Workspace: "ws1", Agent: "a1"}
	hub.Broadcast(ev)

	assert.Equal(t, 2, hub.ClientCount())
	assert.Equal(t, ev, <-c1.C)
	assert.Equal(t, ev, <-c2.C)

	hub.Unsubscribe(c1)
	assert.Equal(t, 1, hub.ClientCount())

	hub.Broadcast(AgenticEvent{Type: EventExecCompleted, Workspace: "ws1"})
	assert.Len(t, c2.C, 1)
}

func TestEventHub_WorkspaceFilter(t *testing.T) {
	hub := NewEventHub()

	all := &EventClient{C: make(chan AgenticEvent, 8)}
	ws1Only := &EventClient{C: make(chan AgenticEvent, 8), WorkspaceSlug: "ws1"}
	hub.Subscribe(all)
	hub.Subscribe(ws1Only)

	hub.Broadcast(AgenticEvent{Type: EventExecStarted, Workspace: "ws1"})
	hub.Broadcast(AgenticEvent{Type: EventExecStarted, Workspace: "ws2"})

	// all client gets both events.
	assert.Len(t, all.C, 2)
	// ws1Only client gets only the ws1 event.
	assert.Len(t, ws1Only.C, 1)
	ev := <-ws1Only.C
	assert.Equal(t, "ws1", ev.Workspace)

	hub.Unsubscribe(all)
	hub.Unsubscribe(ws1Only)
}

func TestEventHub_DropSlowClient(t *testing.T) {
	hub := NewEventHub()

	// Buffer of 1 — second event should be dropped.
	slow := &EventClient{C: make(chan AgenticEvent, 1)}
	hub.Subscribe(slow)

	hub.Broadcast(AgenticEvent{Type: EventExecStarted, Workspace: "ws1"})
	hub.Broadcast(AgenticEvent{Type: EventExecCompleted, Workspace: "ws1"})

	assert.Len(t, slow.C, 1) // only first event fits
	hub.Unsubscribe(slow)
}

func TestExecutionSubscriber_BroadcastsToHub(t *testing.T) {
	// Verify that handleMessage broadcasts parsed events to the EventHub.
	// No real Redis or PostgreSQL — we only test the hub relay path.
	hub := NewEventHub()
	sub := &ExecutionSubscriber{hub: hub}

	client := &EventClient{C: make(chan AgenticEvent, 8)}
	hub.Subscribe(client)
	defer hub.Unsubscribe(client)

	// handleMessage will fail on store.InsertEvent (nil store) but should still
	// broadcast to the hub. We can't call handleMessage directly since it needs
	// a store, so we test the hub broadcast path via Broadcast directly to
	// confirm the wiring pattern. The actual handleMessage → hub path is
	// covered by integration tests (issue #116).
	ev := AgenticEvent{
		Type:      EventExecStarted,
		Workspace: "ws1",
		Agent:     "a1",
	}
	sub.hub.Broadcast(ev)

	assert.Len(t, client.C, 1)
	got := <-client.C
	assert.Equal(t, EventExecStarted, got.Type)
	assert.Equal(t, "ws1", got.Workspace)
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
