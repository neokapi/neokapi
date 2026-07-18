package observe

import (
	"context"
	"log/slog"
)

type contextKey struct{}

type requestIDKey struct{}

type traceIDKey struct{}

// WithLogger stores a logger in the context. Used by RequestIDMiddleware
// to attach a per-request child logger with the request ID pre-set.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// Logger returns the logger stored in ctx, or slog.Default() if none.
func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithRequestID stores the per-request correlation ID on the context so that
// ContextHandler can stamp it onto every log record produced with this ctx
// (via slog's *Context variants), without threading a child logger by hand.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext returns the correlation ID stored on ctx, if any.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok && id != "" {
		return id, true
	}
	return "", false
}

// WithTraceID stores a distributed-trace ID (e.g. the Sentry trace ID) on the
// context so logs and the trace backend share one correlation handle.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceIDFromContext returns the trace ID stored on ctx, if any.
func TraceIDFromContext(ctx context.Context) (string, bool) {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok && id != "" {
		return id, true
	}
	return "", false
}
