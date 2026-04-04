package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

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
// Non-block parts pass through without retry.
type RetryTool struct {
	inner  Tool
	config RetryConfig
}

// NewRetryTool creates a RetryTool wrapping inner with the given retry config.
func NewRetryTool(inner Tool, config RetryConfig) *RetryTool {
	if config.BackoffFactor < 1 {
		config.BackoffFactor = 1
	}
	return &RetryTool{inner: inner, config: config}
}

func (t *RetryTool) Name() string                 { return t.inner.Name() }
func (t *RetryTool) Description() string          { return t.inner.Description() }
func (t *RetryTool) Config() ToolConfig           { return t.inner.Config() }
func (t *RetryTool) SetConfig(c ToolConfig) error { return t.inner.SetConfig(c) }

// Process wraps the inner tool's Process. If the inner tool is a BaseTool
// with HandleBlockFn, retries are applied per-block. Otherwise, the entire
// Process call is retried on error.
func (t *RetryTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	bt, ok := t.inner.(*BaseTool)
	if !ok || bt.HandleBlockFn == nil {
		// Non-BaseTool or no block handler: retry the entire Process.
		return t.retryProcess(ctx, in, out)
	}

	// Block-level retry: wrap the HandleBlockFn with retry logic.
	originalFn := bt.HandleBlockFn
	bt.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		var lastErr error
		backoff := t.config.InitialBackoff

		for attempt := 0; attempt <= t.config.MaxRetries; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
				backoff = time.Duration(float64(backoff) * t.config.BackoffFactor)
			}

			result, err := originalFn(part)
			if err == nil {
				return result, nil
			}

			if !t.isRetryable(err) {
				return nil, err
			}
			lastErr = err
		}

		return nil, fmt.Errorf("after %d retries: %w", t.config.MaxRetries, lastErr)
	}
	defer func() { bt.HandleBlockFn = originalFn }()

	return t.inner.Process(ctx, in, out)
}

// retryProcess retries the entire Process call on error.
func (t *RetryTool) retryProcess(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// For non-BaseTool, we can only retry the entire stream.
	// This is a best-effort approach — buffering is needed.
	var lastErr error
	backoff := t.config.InitialBackoff

	for attempt := 0; attempt <= t.config.MaxRetries; attempt++ {
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

		err := t.inner.Process(ctx, in, out)
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

var _ Tool = (*RetryTool)(nil)
