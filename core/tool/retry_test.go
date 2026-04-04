package tool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryTool_SucceedsWithoutRetry(t *testing.T) {
	inner := &BaseTool{
		ToolName: "test-tool",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return part, nil
		},
	}

	rt := NewRetryTool(inner, RetryConfig{MaxRetries: 3, InitialBackoff: time.Millisecond})
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	block := model.NewBlock("b1", "hello")
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)

	err := rt.Process(context.Background(), in, out)
	require.NoError(t, err)

	result := <-out
	assert.Equal(t, "b1", result.Resource.ResourceID())
}

func TestRetryTool_RetriesOnTransientError(t *testing.T) {
	var attempts atomic.Int32

	inner := &BaseTool{
		ToolName: "flaky-tool",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			n := attempts.Add(1)
			if n < 3 {
				return nil, errors.New("transient error")
			}
			return part, nil
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

	err := rt.Process(context.Background(), in, out)
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetryTool_ExhaustsRetries(t *testing.T) {
	inner := &BaseTool{
		ToolName: "always-fails",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return nil, errors.New("permanent error")
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

	err := rt.Process(context.Background(), in, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after 2 retries")
	assert.Contains(t, err.Error(), "permanent error")
}

func TestRetryTool_RespectsRetryableErrors(t *testing.T) {
	var attempts atomic.Int32

	inner := &BaseTool{
		ToolName: "selective-retry",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			attempts.Add(1)
			return nil, errors.New("rate limit exceeded")
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

	err := rt.Process(context.Background(), in, out)
	require.Error(t, err)
	// Should have attempted 1 + 2 retries = 3 attempts.
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetryTool_DoesNotRetryNonMatchingError(t *testing.T) {
	var attempts atomic.Int32

	inner := &BaseTool{
		ToolName: "non-retryable",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			attempts.Add(1)
			return nil, errors.New("invalid config")
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

	err := rt.Process(context.Background(), in, out)
	require.Error(t, err)
	// Should only attempt once — error doesn't match retryable patterns.
	assert.Equal(t, int32(1), attempts.Load())
}

func TestRetryTool_RespectsContextCancellation(t *testing.T) {
	inner := &BaseTool{
		ToolName: "slow-fail",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return nil, errors.New("error")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
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
