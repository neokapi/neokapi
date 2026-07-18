package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// newTestLogger returns a logger writing JSON to buf, wrapped in ContextHandler
// the same way SetupLogger wires it in production.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	sink := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewContextHandler(sink))
}

func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("decode log line %q: %v", buf.String(), err)
	}
	return m
}

func TestContextHandler_StampsRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx := WithRequestID(context.Background(), "req-123")
	logger.InfoContext(ctx, "hello")

	m := decode(t, &buf)
	if m["request_id"] != "req-123" {
		t.Fatalf("request_id = %v, want req-123", m["request_id"])
	}
}

func TestContextHandler_StampsTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx := WithTraceID(WithRequestID(context.Background(), "req-1"), "trace-abc")
	logger.ErrorContext(ctx, "boom")

	m := decode(t, &buf)
	if m["request_id"] != "req-1" {
		t.Fatalf("request_id = %v, want req-1", m["request_id"])
	}
	if m["trace_id"] != "trace-abc" {
		t.Fatalf("trace_id = %v, want trace-abc", m["trace_id"])
	}
}

func TestContextHandler_NoIDWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.InfoContext(context.Background(), "plain")

	m := decode(t, &buf)
	if _, ok := m["request_id"]; ok {
		t.Fatalf("request_id should be absent, got %v", m["request_id"])
	}
}

func TestContextHandler_PreservesWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf).With("component", "worker")

	ctx := WithRequestID(context.Background(), "req-xyz")
	logger.InfoContext(ctx, "annotated")

	m := decode(t, &buf)
	if m["component"] != "worker" {
		t.Fatalf("component attr lost: %v", m["component"])
	}
	if m["request_id"] != "req-xyz" {
		t.Fatalf("request_id = %v, want req-xyz (WithAttrs must keep the wrapper)", m["request_id"])
	}
}
