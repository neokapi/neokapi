package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// RetryConfig configures retry behavior for a tool.
type RetryConfig struct {
	MaxRetries      int           // maximum number of retries (0 = no retry)
	InitialBackoff  time.Duration // initial delay between retries
	BackoffFactor   float64       // multiplier for each subsequent retry (e.g., 2.0 for exponential)
	RetryableErrors []string      // if non-empty, only retry errors containing these substrings
}

// DefaultRetryConfig returns a sensible default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		BackoffFactor:  2.0,
	}
}

// RetryTool wraps a tool and retries its block processing on transient errors.
// Non-block parts and tools with custom stream processing run once. Retried
// handlers must be idempotent: retries do not undo writes or external effects.
type RetryTool struct {
	inner  Tool
	config RetryConfig
}

// NewRetryTool creates a RetryTool wrapping inner with the given retry config.
func NewRetryTool(inner Tool, config RetryConfig) *RetryTool {
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.BackoffFactor < 1 {
		config.BackoffFactor = 1
	}
	return &RetryTool{inner: inner, config: config}
}

func (t *RetryTool) Name() string                 { return t.inner.Name() }
func (t *RetryTool) Description() string          { return t.inner.Description() }
func (t *RetryTool) Config() ToolConfig           { return t.inner.Config() }
func (t *RetryTool) SetConfig(c ToolConfig) error { return t.inner.SetConfig(c) }

// Process wraps the inner tool's Process. If the inner tool is a BaseTool with
// a capability-typed block handler, retries are applied per-block by wrapping
// that handler on a private copy. Custom stream processors run once: their
// input channels cannot be rewound and their outputs may already be observed.
func (t *RetryTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	bt, ok := t.inner.(*BaseTool)
	if !ok {
		return t.inner.Process(ctx, in, out)
	}
	local := *bt

	// Wrap whichever block handler the inner tool set with retry logic. The
	// handler's capability type is preserved (so the immutability surface is
	// unchanged); only its error path gains backoff/retry.
	switch {
	case bt.Annotate != nil:
		orig := bt.Annotate
		local.Annotate = func(v BlockView) error {
			return t.retryAttempt(ctx, func() error { return orig(v) })
		}
	case bt.Produce != nil:
		orig := bt.Produce
		local.Produce = func(v VariantView) error {
			return t.retryAttempt(ctx, func() error { return orig(v) })
		}
	case bt.Transform != nil:
		orig := bt.Transform
		local.Transform = func(v BlockView) (EditPlan, error) {
			var plan EditPlan
			err := t.retryAttempt(ctx, func() error {
				var attemptErr error
				plan, attemptErr = orig(v)
				return attemptErr
			})
			return plan, err
		}
	default:
		return t.inner.Process(ctx, in, out)
	}

	return local.Process(ctx, in, out)
}

// SessionProcess preserves the inner tool's session contract. Session tools
// own their replay and transaction semantics, so their stream runs once.
func (t *RetryTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	if inner, ok := t.inner.(SessionTool); ok {
		return inner.SessionProcess(ctx, sess, in, out)
	}
	return t.Process(ctx, in, out)
}

// retryAttempt runs fn under the configured backoff/retry policy: it retries
// retryable errors up to MaxRetries with exponential backoff, honoring ctx.
func (t *RetryTool) retryAttempt(ctx context.Context, fn func() error) error {
	var lastErr error
	backoff := t.config.InitialBackoff
	for attempt := 0; attempt <= t.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			backoff = time.Duration(float64(backoff) * t.config.BackoffFactor)
		}
		err := fn()
		if err == nil {
			return nil
		}
		if !t.isRetryable(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("after %d retries: %w", t.config.MaxRetries, lastErr)
}

func (t *RetryTool) isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if len(t.config.RetryableErrors) == 0 {
		return true // retry all errors when no filter specified
	}
	msg := err.Error()
	for _, pattern := range t.config.RetryableErrors {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

var (
	_ Tool        = (*RetryTool)(nil)
	_ SessionTool = (*RetryTool)(nil)
)
