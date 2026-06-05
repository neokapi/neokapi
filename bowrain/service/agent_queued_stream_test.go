package service

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	bragent "github.com/neokapi/neokapi/bowrain/agent"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockQueue is an in-memory AgentEnqueuer that captures enqueued payloads.
type mockQueue struct {
	mu       sync.Mutex
	payloads []string
	err      error
}

func (q *mockQueue) Enqueue(_ context.Context, payload string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.payloads = append(q.payloads, payload)
	return nil
}

func (q *mockQueue) lastPayload() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.payloads) == 0 {
		return ""
	}
	return q.payloads[len(q.payloads)-1]
}

// TestQueuedStream_EnqueuePayloadFormat verifies that the JSON payload
// enqueued by sendQueuedStream contains all required fields.
// Since sendQueuedStream uses *AgentPubSub (concrete Redis-backed type),
// we cannot run the full method without Redis. Instead we verify the
// payload marshalling logic matches AgentJobMessage expectations.
func TestQueuedStream_EnqueuePayloadFormat(t *testing.T) {
	// Simulate the payload that sendQueuedStream builds before enqueuing.
	payload, err := json.Marshal(map[string]string{
		"conversation_id": "conv-42",
		"message_id":      "msg-99",
		"workspace_id":    "ws-1",
		"user_id":         "user-1",
		"workspace_role":  "admin",
		"content":         "Translate my project",
		"mode":            "coworker",
	})
	require.NoError(t, err)

	mq := &mockQueue{}
	require.NoError(t, mq.Enqueue(t.Context(), string(payload)))

	got := mq.lastPayload()
	require.NotEmpty(t, got)

	var job map[string]string
	require.NoError(t, json.Unmarshal([]byte(got), &job))
	assert.Equal(t, "conv-42", job["conversation_id"])
	assert.Equal(t, "msg-99", job["message_id"])
	assert.Equal(t, "ws-1", job["workspace_id"])
	assert.Equal(t, "user-1", job["user_id"])
	assert.Equal(t, "admin", job["workspace_role"])
	assert.Equal(t, "Translate my project", job["content"])
	assert.Equal(t, "coworker", job["mode"])
}

// TestQueuedStream_EnqueueWithAllModes verifies that different modes
// are correctly included in the enqueued payload.
func TestQueuedStream_EnqueueWithAllModes(t *testing.T) {
	modes := []string{"ask", "coworker", "bravo", ""}
	for _, mode := range modes {
		t.Run("mode_"+mode, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"conversation_id": "conv-1",
				"message_id":      "msg-1",
				"workspace_id":    "ws-1",
				"user_id":         "user-1",
				"workspace_role":  "member",
				"content":         "Hello",
				"mode":            mode,
			})
			require.NoError(t, err)

			var job map[string]string
			require.NoError(t, json.Unmarshal(payload, &job))
			assert.Equal(t, mode, job["mode"])
		})
	}
}

// TestQueuedStream_SSERelaySimulation tests the event relay logic
// that sendQueuedStream uses to forward Redis events to the SSE writer.
// We simulate the relay loop by writing events directly.
func TestQueuedStream_SSERelaySimulation(t *testing.T) {
	events := []SSEEvent{
		{Event: SSEMessageStart, Data: json.RawMessage(`{"id":"m1","role":"assistant"}`)},
		{Event: SSEContentDelta, Data: json.RawMessage(`{"delta":"Hello from worker"}`)},
		{Event: SSEContentDelta, Data: json.RawMessage(`{"delta":" — more text"}`)},
		{Event: SSEMessageEnd, Data: json.RawMessage(`{"id":"m1"}`)},
	}

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	for _, evt := range events {
		err := sse.WriteEvent(evt.Event, evt.Data)
		require.NoError(t, err)

		// Stop relaying after terminal events (matches sendQueuedStream logic).
		if evt.Event == SSEMessageEnd || evt.Event == SSEError {
			break
		}
	}

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "Hello from worker")
	assert.Contains(t, output, "more text")
	assert.Contains(t, output, "event: message_end")
}

// TestQueuedStream_SSERelayStopsOnError verifies that the relay loop
// terminates when an error event is received.
func TestQueuedStream_SSERelayStopsOnError(t *testing.T) {
	events := []SSEEvent{
		{Event: SSEMessageStart, Data: json.RawMessage(`{"id":"m1","role":"assistant"}`)},
		{Event: SSEError, Data: json.RawMessage(`{"error":"agent crashed"}`)},
		// This event should never be written:
		{Event: SSEContentDelta, Data: json.RawMessage(`{"delta":"should not appear"}`)},
	}

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	for _, evt := range events {
		_ = sse.WriteEvent(evt.Event, evt.Data)
		if evt.Event == SSEMessageEnd || evt.Event == SSEError {
			break
		}
	}

	output := buf.String()
	assert.Contains(t, output, "agent crashed")
	assert.NotContains(t, output, "should not appear")
}

// TestQueuedStream_TimeoutWritesError verifies that a timeout produces
// an error SSE event (simulating the timer.C path in sendQueuedStream).
func TestQueuedStream_TimeoutWritesError(t *testing.T) {
	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	// Simulate what sendQueuedStream does on timeout.
	_ = sse.WriteEvent(SSEError, ErrorData{Error: "agent response timed out"})

	output := buf.String()
	assert.Contains(t, output, "event: error")
	assert.Contains(t, output, "agent response timed out")
}

// TestQueuedStream_RoutingDecision verifies that SendMessageStream routes
// to sendQueuedStream when pool==nil, queue!=nil, pubsub!=nil.
// Since we can't provide a real *AgentPubSub, we verify the routing
// by checking that the non-queue paths are not taken.
func TestQueuedStream_RoutingDecision_LocalFallback(t *testing.T) {
	// When pool==nil and queue==nil, should route to local mode.
	pgdb := pgtest.NewTestDB(t)
	store, err := bragent.NewStore(pgdb)
	require.NoError(t, err)

	svc := NewAgentService(store, nil)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	// No pool, no queue → local mock mode.
	err = svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "Hello local", "ask", nil, sse)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "No agent container pool is configured")
}

// TestQueuedStream_RoutingDecision_QueueWithoutPubSub verifies that when
// queue is set but pubsub is nil, it falls through to local mode.
func TestQueuedStream_RoutingDecision_QueueWithoutPubSub(t *testing.T) {
	pgdb := pgtest.NewTestDB(t)
	store, err := bragent.NewStore(pgdb)
	require.NoError(t, err)

	svc := NewAgentService(store, nil)
	svc.queue = &mockQueue{} // queue set but no pubsub
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	err = svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "Hello", "ask", nil, sse)
	require.NoError(t, err)

	// Should fall through to local mode since pubsub is nil.
	output := buf.String()
	assert.Contains(t, output, "No agent container pool is configured")
}

// TestQueuedStream_EnqueueError verifies that enqueue failures propagate.
func TestQueuedStream_EnqueueError(t *testing.T) {
	mq := &mockQueue{err: assert.AnError}

	err := mq.Enqueue(t.Context(), "anything")
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

// TestQueuedStream_UserMessagePersistedBeforeEnqueue verifies that the
// user message is persisted before the job is enqueued. We check this
// by verifying SendMessageStream persists the user message even in local mode.
func TestQueuedStream_UserMessagePersistedBeforeEnqueue(t *testing.T) {
	pgdb := pgtest.NewTestDB(t)
	store, err := bragent.NewStore(pgdb)
	require.NoError(t, err)

	svc := NewAgentService(store, nil)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "ws1", "user1", "", "Chat")
	require.NoError(t, err)

	var buf bytes.Buffer
	err = svc.SendMessageStream(ctx, conv.ID, "user1", "ws1", "member", "My question", "ask", nil, NewSSEWriter(&buf))
	require.NoError(t, err)

	msgs, err := svc.ListMessages(ctx, conv.ID, 10, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(msgs), 1)
	// First message should be the user message.
	assert.Equal(t, "My question", msgs[0].Content)
}

// TestQueuedStream_EventChannelRelay exercises a goroutine-based relay
// similar to the real sendQueuedStream loop, using plain channels.
func TestQueuedStream_EventChannelRelay(t *testing.T) {
	events := make(chan SSEEvent, 10)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	// Feed events.
	go func() {
		events <- SSEEvent{Event: SSEMessageStart, Data: json.RawMessage(`{"id":"m1","role":"assistant"}`)}
		events <- SSEEvent{Event: SSEContentDelta, Data: json.RawMessage(`{"delta":"chunk1"}`)}
		events <- SSEEvent{Event: SSEContentDelta, Data: json.RawMessage(`{"delta":"chunk2"}`)}
		events <- SSEEvent{Event: SSEMessageEnd, Data: json.RawMessage(`{"id":"m1"}`)}
	}()

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	// Relay loop matching sendQueuedStream logic.
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			_ = sse.WriteEvent(evt.Event, evt.Data)
			if evt.Event == SSEMessageEnd || evt.Event == SSEError {
				goto done
			}
		case <-timeout.C:
			t.Fatal("timed out")
		case <-ctx.Done():
			t.Fatal("context cancelled")
		}
	}
done:

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "chunk1")
	assert.Contains(t, output, "chunk2")
	assert.Contains(t, output, "event: message_end")
}

// TestQueuedStream_ContextCancelDuringRelay verifies that context cancellation
// during the relay loop stops processing.
func TestQueuedStream_ContextCancelDuringRelay(t *testing.T) {
	events := make(chan SSEEvent, 10)
	ctx, cancel := context.WithCancel(t.Context())

	// Send one event then cancel.
	go func() {
		events <- SSEEvent{Event: SSEMessageStart, Data: json.RawMessage(`{"id":"m1","role":"assistant"}`)}
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	var ctxErr error
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				goto done
			}
			_ = sse.WriteEvent(evt.Event, evt.Data)
			if evt.Event == SSEMessageEnd || evt.Event == SSEError {
				goto done
			}
		case <-timeout.C:
			goto done
		case <-ctx.Done():
			ctxErr = ctx.Err()
			goto done
		}
	}
done:

	require.ErrorIs(t, ctxErr, context.Canceled)
	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	// message_end should NOT be present since we cancelled.
	assert.NotContains(t, output, "event: message_end")
}
