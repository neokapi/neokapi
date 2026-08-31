package observe

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	sentry "github.com/getsentry/sentry-go"
)

// sentryEnabled records whether InitSentry successfully configured a client, so
// capture helpers can cheaply skip work when Sentry is not configured.
var sentryEnabled bool

// SentryConfig configures error reporting. When DSN is empty, InitSentry is a
// no-op and the service runs with error tracking disabled — this is the default
// for local/self-hosted deployments that have not provisioned a Sentry project.
type SentryConfig struct {
	DSN         string
	Environment string // e.g. "production", "staging", "dev"
	Release     string // e.g. version+commit, for release health / regressions
	// SampleRate for error events (0..1); defaults to 1.0 (report everything).
	SampleRate float64
	// TracesSampleRate for performance traces (0..1); 0 disables tracing.
	TracesSampleRate float64
	// ServerName tags events with which service produced them ("server"/"worker").
	ServerName string
}

// InitSentry initializes the global Sentry client. Safe to call with an empty
// DSN (returns false, no-op). PII scrubbing: SendDefaultPII stays false and a
// BeforeSend hook drops request cookies/headers that could carry tokens.
func InitSentry(cfg SentryConfig) bool {
	if cfg.DSN == "" {
		slog.Info("sentry: disabled (no DSN configured)")
		return false
	}
	sampleRate := cfg.SampleRate
	if sampleRate == 0 {
		sampleRate = 1.0
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		ServerName:       cfg.ServerName,
		SampleRate:       sampleRate,
		TracesSampleRate: cfg.TracesSampleRate,
		// Necessary as well as the rate, and easy to miss: sentry-go drops
		// EVERY transaction when this is false, whatever TracesSampleRate says
		// (tracing.go, "Dropping transaction: EnableTracing is set to false").
		// Setting the rate alone produces a deployment that looks instrumented,
		// reports errors normally, and silently discards every transaction —
		// the same shape of fault as having no instrumentation at all, but
		// harder to see because the configuration reads as correct.
		EnableTracing: cfg.TracesSampleRate > 0,
		// SendDefaultPII is deprecated in favor of DataCollection; leaving
		// DataCollection unset keeps its own denylist-based scrubbing (cookies,
		// sensitive headers) as the default, on top of the explicit BeforeSend
		// scrub below — belt and braces, not a relaxation.
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			// Defense in depth: never ship auth material even if some SDK path
			// attaches request data.
			if event.Request != nil {
				event.Request.Cookies = ""
				delete(event.Request.Headers, "Authorization")
				delete(event.Request.Headers, "Cookie")
				delete(event.Request.Headers, "X-Bowrain-Csrf")
			}
			return event
		},
	})
	if err != nil {
		slog.Error("sentry: init failed", "error", err)
		return false
	}
	sentryEnabled = true
	slog.Info("sentry: enabled", "environment", cfg.Environment, "service", cfg.ServerName)
	return true
}

// InitSentryFromEnv configures Sentry from the standard environment variables,
// so the server and worker stay in sync:
//
//	SENTRY_DSN                 error-tracking DSN (empty → disabled)
//	SENTRY_ENVIRONMENT         "production" | "staging" | "dev" (default "production")
//	SENTRY_TRACES_SAMPLE_RATE  0..1 performance tracing (default 0 = off)
//
// serverName distinguishes the emitting service ("server", "worker"); release
// ties events to a build for regression/release-health tracking.
func InitSentryFromEnv(serverName, release string) bool {
	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = "production"
	}
	traces := 0.0
	if v := os.Getenv("SENTRY_TRACES_SAMPLE_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			traces = f
		}
	}
	return InitSentry(SentryConfig{
		DSN:              os.Getenv("SENTRY_DSN"),
		Environment:      env,
		Release:          release,
		TracesSampleRate: traces,
		ServerName:       serverName,
	})
}

// SentryEnabled reports whether Sentry is configured.
func SentryEnabled() bool { return sentryEnabled }

// FlushSentry flushes buffered events; call on shutdown. No-op when disabled.
func FlushSentry(timeout time.Duration) {
	if sentryEnabled {
		sentry.Flush(timeout)
	}
}

// CaptureError reports err to Sentry with the given correlation tags. The
// reference tag is the request/run ID the user sees, so quoting it in Sentry
// search resolves straight to the event. No-op when Sentry is disabled.
//
// tags is a small set of low-cardinality-ish attributes (route, method,
// status); reference is the high-value correlation handle. Returns the Sentry
// event ID (empty when disabled or dropped).
func CaptureError(err error, reference string, tags map[string]string) string {
	if !sentryEnabled || err == nil {
		return ""
	}
	hub := sentry.CurrentHub().Clone()
	hub.ConfigureScope(func(scope *sentry.Scope) {
		if reference != "" {
			scope.SetTag("reference", reference)
			scope.SetTag("request_id", reference)
		}
		for k, v := range tags {
			if v != "" {
				scope.SetTag(k, v)
			}
		}
	})
	id := hub.CaptureException(err)
	if id == nil {
		return ""
	}
	return string(*id)
}
