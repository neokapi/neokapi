package observe

import (
	"context"
	"log/slog"
)

// ContextHandler wraps a slog.Handler and enriches every record with the
// correlation identifiers carried on the context (request_id, trace_id).
//
// This is what makes drill-down work: any log emitted with a *Context variant
// (slog.InfoContext, ErrorContext, …) whose ctx flowed from RequestIDMiddleware
// — or from a background run seeded via WithRequestID — automatically carries
// the same request_id the client sees as its error "reference". Package-level
// slog.Info calls (no ctx) are unaffected, so this is a safe, additive change.
type ContextHandler struct {
	slog.Handler
}

// NewContextHandler wraps h so that context-borne correlation IDs are appended
// to each record before it reaches h.
func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{Handler: h}
}

// Handle appends request_id / trace_id from ctx (when present) then delegates.
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := RequestIDFromContext(ctx); ok {
		r.AddAttrs(slog.String("request_id", id))
	}
	if id, ok := TraceIDFromContext(ctx); ok {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs preserves the wrapper around the derived handler.
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup preserves the wrapper around the derived handler.
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{Handler: h.Handler.WithGroup(name)}
}
