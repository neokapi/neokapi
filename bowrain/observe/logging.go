// Package observe provides observability infrastructure for the Bowrain server:
// structured logging, Prometheus metrics, request correlation, and profiling.
package observe

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	slogformatter "github.com/samber/slog-formatter"
	slogmulti "github.com/samber/slog-multi"
)

// SetupLogger creates and installs a structured slog.Logger as the default.
//
// format controls the output handler:
//   - "json" (default) — slog.JSONHandler for production
//   - "text" — colorized tint handler for local development
//
// level controls the minimum log level:
//   - "debug", "info" (default), "warn", "error"
//
// The logger includes PII redaction middleware that masks sensitive fields
// (email addresses, connection strings) before they reach any handler.
//
// Calling slog.SetDefault bridges the stdlib log package so existing
// log.Printf calls automatically flow through this handler (Go 1.22+).
func SetupLogger(format, level string) *slog.Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{Level: lvl}

	var sink slog.Handler
	switch strings.ToLower(format) {
	case "text", "dev":
		sink = tint.NewHandler(os.Stdout, &tint.Options{
			Level:      lvl,
			TimeFormat: time.Kitchen,
		})
	default:
		sink = slog.NewJSONHandler(os.Stdout, opts)
	}

	handler := slogmulti.
		Pipe(slogformatter.NewFormatterMiddleware(
			slogformatter.PIIFormatter("email"),
			slogformatter.PIIFormatter("user_email"),
			slogformatter.PIIFormatter("owner_email"),
			slogformatter.PIIFormatter("name"),
			slogformatter.PIIFormatter("user_name"),
			maskURLFormatter("redis_url"),
			maskURLFormatter("database_url"),
		)).
		Handler(sink)

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// parseLevel converts a string to slog.Level.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// maskURLFormatter redacts passwords and credentials from connection string URLs.
// "redis://user:secret@host:6379" → "redis://***@host:6379"
func maskURLFormatter(key string) slogformatter.Formatter {
	return slogformatter.FormatByKey(key, func(v slog.Value) slog.Value {
		s := v.String()
		// Mask anything between :// and @ (user:password portion).
		if start := strings.Index(s, "://"); start >= 0 {
			if at := strings.Index(s[start+3:], "@"); at >= 0 {
				return slog.StringValue(s[:start+3] + "***" + s[start+3+at:])
			}
		}
		return v
	})
}
