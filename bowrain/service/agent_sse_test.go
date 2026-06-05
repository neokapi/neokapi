package service

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEWriterWriteEvent(t *testing.T) {
	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	err := sse.WriteEvent("content_delta", ContentDeltaData{Delta: "Hello"})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "event: content_delta\n")
	assert.Contains(t, output, `"delta":"Hello"`)
	assert.NotEmpty(t, output)
}

func TestSSEWriterMultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	require.NoError(t, sse.WriteEvent(SSEMessageStart, MessageStartData{ID: "msg-1", Role: "assistant"}))
	require.NoError(t, sse.WriteEvent(SSEContentDelta, ContentDeltaData{Delta: "Hello "}))
	require.NoError(t, sse.WriteEvent(SSEContentDelta, ContentDeltaData{Delta: "world"}))
	require.NoError(t, sse.WriteEvent(SSEMessageEnd, MessageEndData{ID: "msg-1"}))

	output := buf.String()
	assert.Contains(t, output, "event: message_start")
	assert.Contains(t, output, "event: content_delta")
	assert.Contains(t, output, "event: message_end")
	// Verify proper SSE formatting (double newline between events).
	assert.Contains(t, output, "\n\n")
}

func TestSSEWriterToolCallEvents(t *testing.T) {
	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	require.NoError(t, sse.WriteEvent(SSEToolCallStart, ToolCallStartData{
		ID:    "tc-1",
		Tool:  "run_flow",
		Input: []byte(`{"flow":"pseudo-translate"}`),
	}))
	require.NoError(t, sse.WriteEvent(SSEToolCallEnd, ToolCallEndData{
		ID:         "tc-1",
		Status:     "completed",
		Output:     []byte(`{"blocks_processed":45}`),
		DurationMs: 3200,
	}))

	output := buf.String()
	assert.Contains(t, output, "event: tool_call_start")
	assert.Contains(t, output, "event: tool_call_end")
	assert.Contains(t, output, "run_flow")
}

func TestSSEWriterNeedsApproval(t *testing.T) {
	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	require.NoError(t, sse.WriteEvent(SSENeedsApproval, NeedsApprovalData{
		ID:    "tc-2",
		Tool:  "connector_push",
		Input: []byte(`{"project_id":"proj-1"}`),
	}))

	output := buf.String()
	assert.Contains(t, output, "event: needs_approval")
	assert.Contains(t, output, "connector_push")
}

func TestSSEWriterErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	sse := NewSSEWriter(&buf)

	require.NoError(t, sse.WriteEvent(SSEError, ErrorData{Error: "something went wrong"}))

	output := buf.String()
	assert.Contains(t, output, "event: error")
	assert.Contains(t, output, "something went wrong")
}
