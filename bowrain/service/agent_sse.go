package service

import (
	"encoding/json"
	"fmt"
	"io"
)

// SSEWriter writes Server-Sent Events to an HTTP response stream.
type SSEWriter struct {
	w       io.Writer
	flusher flusher
}

type flusher interface {
	Flush()
}

// NewSSEWriter creates an SSE writer from an io.Writer.
// If the writer implements http.Flusher, events are flushed immediately.
func NewSSEWriter(w io.Writer) SSEWriter {
	f, _ := w.(flusher)
	return SSEWriter{w: w, flusher: f}
}

// WriteEvent writes a named SSE event with JSON data.
func (s SSEWriter) WriteEvent(event string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal SSE data: %w", err)
	}
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, jsonData)
	if err != nil {
		return fmt.Errorf("write SSE event: %w", err)
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// SSE event types matching the Bowrain AD-016 protocol.
const (
	SSEMessageStart  = "message_start"
	SSEContentDelta  = "content_delta"
	SSEToolCallStart = "tool_call_start"
	SSEToolCallEnd   = "tool_call_end"
	SSENeedsApproval = "needs_approval"
	SSEMessageEnd    = "message_end"
	SSEStepUp        = "step_up"
	SSEError         = "error"
)

// SSE event data structures.

// MessageStartData is emitted at the start of an assistant message.
type MessageStartData struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// ContentDeltaData is emitted for each text chunk.
type ContentDeltaData struct {
	Delta string `json:"delta"`
}

// ToolCallStartData is emitted when the agent invokes a tool.
type ToolCallStartData struct {
	ID    string          `json:"id"`
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// ToolCallEndData is emitted when a tool call completes.
type ToolCallEndData struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output,omitempty"`
	DurationMs int64           `json:"duration_ms"`
}

// NeedsApprovalData is emitted when a tool call requires human approval.
type NeedsApprovalData struct {
	ID    string          `json:"id"`
	Tool  string          `json:"tool"`
	Input json.RawMessage `json:"input"`
}

// MessageEndData is emitted when the assistant message is complete.
type MessageEndData struct {
	ID    string        `json:"id"`
	Usage *MessageUsage `json:"usage,omitempty"`
}

// MessageUsage reports token consumption for a single message turn.
type MessageUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StepUpData is emitted when an action requires a higher permission mode.
// The frontend renders this as an inline card with mode switch buttons.
type StepUpData struct {
	CurrentMode  string   `json:"current_mode"`
	RequiredMode string   `json:"required_mode"`
	Action       string   `json:"action"`      // human-readable description of blocked action
	Permissions  []string `json:"permissions"` // permission names needed
}

// ErrorData is emitted when an error occurs during processing.
type ErrorData struct {
	Error string `json:"error"`
}

// EventSink is the interface for writing SSE events. Implemented by SSEWriter
// (writes to HTTP response) and the Redis sink (publishes to pub/sub).
type EventSink interface {
	WriteEvent(event string, data any) error
}
