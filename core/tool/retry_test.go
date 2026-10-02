package tool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryTool_SucceedsWithoutRetry(t *testing.T) {
	inner := &BaseTool{
		ToolName: "test-tool",
		Annotate: func(v BlockView) error {
			return nil
		},
	}

	rt := NewRetryTool(inner, RetryConfig{MaxRetries: 3, InitialBackoff: time.Millisecond})
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(t.Context(), in, out)
	require.NoError(t, err)

	result := <-out
	assert.Equal(t, "b1", result.Resource.ResourceID())
}

func TestRetryTool_RetriesOnTransientError(t *testing.T) {
	var attempts atomic.Int32

	inner := &BaseTool{
		ToolName: "flaky-tool",
		Annotate: func(v BlockView) error {
			n := attempts.Add(1)
			if n < 3 {
				return errors.New("transient error")
			}
			return nil
		},
	}

	rt := NewRetryTool(inner, RetryConfig{
		MaxRetries:     5,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(t.Context(), in, out)
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetryTool_ExhaustsRetries(t *testing.T) {
	inner := &BaseTool{
		ToolName: "always-fails",
		Annotate: func(v BlockView) error {
			return errors.New("permanent error")
		},
	}

	rt := NewRetryTool(inner, RetryConfig{
		MaxRetries:     2,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(t.Context(), in, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after 2 retries")
	assert.Contains(t, err.Error(), "permanent error")
}

func TestRetryTool_RespectsRetryableErrors(t *testing.T) {
	var attempts atomic.Int32

	inner := &BaseTool{
		ToolName: "selective-retry",
		Annotate: func(v BlockView) error {
			attempts.Add(1)
			return errors.New("rate limit exceeded")
		},
	}

	// Only retry "rate limit" errors.
	rt := NewRetryTool(inner, RetryConfig{
		MaxRetries:      2,
		InitialBackoff:  time.Millisecond,
		BackoffFactor:   1.0,
		RetryableErrors: []string{"rate limit"},
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(t.Context(), in, out)
	require.Error(t, err)
	// Should have attempted 1 + 2 retries = 3 attempts.
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetryTool_DoesNotRetryNonMatchingError(t *testing.T) {
	var attempts atomic.Int32

	inner := &BaseTool{
		ToolName: "non-retryable",
		Annotate: func(v BlockView) error {
			attempts.Add(1)
			return errors.New("invalid config")
		},
	}

	rt := NewRetryTool(inner, RetryConfig{
		MaxRetries:      3,
		InitialBackoff:  time.Millisecond,
		BackoffFactor:   1.0,
		RetryableErrors: []string{"rate limit", "timeout"},
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(t.Context(), in, out)
	require.Error(t, err)
	// Should only attempt once — error doesn't match retryable patterns.
	assert.Equal(t, int32(1), attempts.Load())
}

func TestRetryTool_RespectsContextCancellation(t *testing.T) {
	inner := &BaseTool{
		ToolName: "slow-fail",
		Annotate: func(v BlockView) error {
			return errors.New("error")
		},
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately.

	rt := NewRetryTool(inner, RetryConfig{
		MaxRetries:     5,
		InitialBackoff: time.Second, // Would be slow if not cancelled.
		BackoffFactor:  1.0,
	})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(ctx, in, out)
	require.Error(t, err)
}

// consumedStreamFailure consumes the failed part before returning an error.
// Reinvoking it over the same channel skips that part and reports false success.
type consumedStreamFailure struct {
	BaseTool
	calls int
	err   error
}

func (f *consumedStreamFailure) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	f.calls++
	if f.calls == 1 {
		<-in
		return f.err
	}
	return f.BaseTool.Process(ctx, in, out)
}

func TestRetryTool_DoesNotReplayConsumedStream(t *testing.T) {
	failure := errors.New("provider failed after input was consumed")
	inner := &consumedStreamFailure{err: failure}
	inner.ToolName = "stream"
	wrapped := NewRetryTool(inner, RetryConfig{MaxRetries: 2})
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("failed", "content")},
		{Type: model.PartBlock, Resource: model.NewBlock("remaining", "content")},
	}
	_, err := RunOnParts(t.Context(), wrapped, parts)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, inner.calls)
}

func TestRetryTool_LeavesInnerHandlerUnchanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var originalCalls int
		failure := errors.New("original call failed")
		inner := &BaseTool{
			ToolName: "shared-handler",
			Annotate: func(v BlockView) error {
				if v.ID() == "wrapped" {
					<-release
					return nil
				}
				originalCalls++
				return failure
			},
		}
		wrapped := NewRetryTool(inner, RetryConfig{MaxRetries: 2})
		done := make(chan error, 1)
		go func() {
			_, err := RunOnParts(t.Context(), wrapped, []*model.Part{
				{Type: model.PartBlock, Resource: model.NewBlock("wrapped", "content")},
			})
			done <- err
		}()
		synctest.Wait()
		_, err := inner.ApplyContext(t.Context(), &model.Part{
			Type: model.PartBlock, Resource: model.NewBlock("original", "content"),
		})
		close(release)
		require.NoError(t, <-done)
		require.ErrorIs(t, err, failure)
		assert.Equal(t, 1, originalCalls, "using a retry wrapper must not install retries on the original tool")
	})
}

func TestRetryTool_ForwardsSession(t *testing.T) {
	inner := newSessionRecorder()
	wrapped := NewRetryTool(inner, RetryConfig{MaxRetries: 2})
	st, ok := any(wrapped).(SessionTool)
	require.True(t, ok, "wrapping a tool must preserve its session contract")
	require.NoError(t, drainThrough(t, func(in <-chan *model.Part, out chan<- *model.Part) error {
		return st.SessionProcess(t.Context(), nil, in, out)
	}))
	assert.True(t, inner.sawSession)
	assert.False(t, inner.sawPlain)
}

func TestRetryTool_DoesNotRetryCancellation(t *testing.T) {
	var attempts int
	wrapped := NewRetryTool(&BaseTool{}, RetryConfig{MaxRetries: 3})
	err := wrapped.retryAttempt(t.Context(), func() error {
		attempts++
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, attempts)
}
