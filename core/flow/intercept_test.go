package flow

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unguardedSendTool forwards every part with a bare channel send — no select on
// ctx.Done(). It models a tool the wrappers do not control (a plugin tool
// dispatched as a subprocess): if a wrapper abandons the inner output channel on
// cancellation, this tool parks on its send and never returns.
type unguardedSendTool struct {
	tool.BaseTool
	returned chan struct{}
}

func newUnguardedSendTool(name string) *unguardedSendTool {
	t := &unguardedSendTool{returned: make(chan struct{})}
	t.BaseTool.ToolName = name
	return t
}

func (t *unguardedSendTool) Process(_ context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	defer close(t.returned)
	for part := range in {
		out <- part
	}
	return nil
}

// observedCollector counts the parts a StreamingCollector was shown.
type observedCollector struct{ observed atomic.Int32 }

func (c *observedCollector) Observe(*model.Part) { c.observed.Add(1) }

func (c *observedCollector) Collect(context.Context, *Item, []*model.Part) error { return nil }

func (c *observedCollector) Result() (CollectorResult, error) {
	return CollectorResult{Name: "observed", Data: int(c.observed.Load())}, nil
}

// TestToolWrappers_InnerToolReturnsAfterCancel verifies that cancelling a run
// whose output is not being drained still lets the inner tool finish: the
// wrapper keeps consuming the inner tool's output (discarding it) instead of
// walking away from a tool that is mid-send, and reports the cancellation.
func TestToolWrappers_InnerToolReturnsAfterCancel(t *testing.T) {
	tests := []struct {
		name string
		wrap func(tool.Tool) tool.Tool
	}{
		{
			name: "tapping",
			wrap: func(inner tool.Tool) tool.Tool { return NewTappingTool(inner, &observedCollector{}) },
		},
		{
			name: "metrics",
			wrap: func(inner tool.Tool) tool.Tool {
				return WrapWithMetrics([]tool.Tool{inner}, NewPipelineMetrics([]string{"step"}))[0]
			},
		},
		{
			name: "tracing",
			wrap: func(inner tool.Tool) tool.Tool { return NewTracingTool(inner, "node", NewTraceRecorder()) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := newUnguardedSendTool(tt.name)
			wrapped := tt.wrap(inner)

			// out has no reader, so the wrapper's forwarding send blocks and the
			// inner tool ends up blocked behind it.
			in := make(chan *model.Part)
			out := make(chan *model.Part)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			feedStop := make(chan struct{})
			feedDone := make(chan struct{})
			go func() {
				defer close(feedDone)
				defer close(in)
				for range 100 {
					select {
					case in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1"}}:
					case <-feedStop:
						return
					}
				}
			}()

			done := make(chan error, 1)
			go func() { done <- wrapped.Process(ctx, in, out) }()

			// Let the parts start flowing, then cancel with the output stalled.
			time.Sleep(20 * time.Millisecond)
			cancel()

			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(2 * time.Second):
				t.Fatal("Process did not return after cancel")
			}

			select {
			case <-inner.returned:
			case <-time.After(2 * time.Second):
				t.Fatal("inner tool never returned (its goroutine leaked)")
			}

			close(feedStop)
			<-feedDone
		})
	}
}

// TestInterceptTool_ForwardsEveryPartAndInnerError covers the uncancelled path:
// both hooks see every part, the parts reach out in order, and the inner tool's
// error is what the caller gets.
func TestInterceptTool_ForwardsEveryPartAndInnerError(t *testing.T) {
	in := make(chan *model.Part, 3)
	out := make(chan *model.Part, 3)
	for _, id := range []string{"a", "b", "c"} {
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: id}}
	}
	close(in)

	var seenIn, seenOut []string
	innerErr := context.DeadlineExceeded
	err := interceptTool(context.Background(), in, out,
		func(p *model.Part) { seenIn = append(seenIn, p.Resource.ResourceID()) },
		func(p *model.Part) { seenOut = append(seenOut, p.Resource.ResourceID()) },
		func(innerIn <-chan *model.Part, innerOut chan<- *model.Part) error {
			for p := range innerIn {
				innerOut <- p
			}
			return innerErr
		})
	require.ErrorIs(t, err, innerErr)
	close(out)

	var forwarded []string
	for p := range out {
		forwarded = append(forwarded, p.Resource.ResourceID())
	}
	assert.Equal(t, []string{"a", "b", "c"}, seenIn)
	assert.Equal(t, []string{"a", "b", "c"}, seenOut)
	assert.Equal(t, []string{"a", "b", "c"}, forwarded)
}
