package observe

import (
	"context"
	"log/slog"
)

type contextKey struct{}

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
