package event

import (
	"context"
	"sync"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// RunLogger provides structured logging for automation steps.
// Logs are buffered and flushed in batches to avoid per-line DB writes.
type RunLogger struct {
	store  *bstore.AutomationRunStore
	runID  string
	stepID string

	mu  sync.Mutex
	buf []bstore.AutomationLog
}

// NewRunLogger creates a logger for a specific step within a run.
// If store is nil, logs are silently discarded.
func NewRunLogger(store *bstore.AutomationRunStore, runID, stepID string) *RunLogger {
	return &RunLogger{store: store, runID: runID, stepID: stepID}
}

// Info logs an informational message.
func (l *RunLogger) Info(msg string, data map[string]string) {
	l.append("info", msg, data)
}

// Warn logs a warning message.
func (l *RunLogger) Warn(msg string, data map[string]string) {
	l.append("warn", msg, data)
}

// Error logs an error message.
func (l *RunLogger) Error(msg string, data map[string]string) {
	l.append("error", msg, data)
}

func (l *RunLogger) append(level, msg string, data map[string]string) {
	if l.store == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, bstore.AutomationLog{
		StepID:  l.stepID,
		RunID:   l.runID,
		Level:   level,
		Message: msg,
		Data:    data,
	})
	// Auto-flush when buffer gets large.
	if len(l.buf) >= 20 {
		l.flushLocked(context.Background())
	}
}

// Flush writes all buffered logs to the store.
func (l *RunLogger) Flush(ctx context.Context) {
	if l.store == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.flushLocked(ctx)
}

func (l *RunLogger) flushLocked(ctx context.Context) {
	if len(l.buf) == 0 {
		return
	}
	_ = l.store.AppendLogs(ctx, l.buf)
	l.buf = l.buf[:0]
}
