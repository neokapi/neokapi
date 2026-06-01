package flow

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passThroughTool is a minimal tool that forwards all parts unchanged.
type passThroughTool struct{ tool.BaseTool }

func newPassThroughTool(name string) *passThroughTool {
	t := &passThroughTool{}
	t.BaseTool.ToolName = name
	return t
}

// earlyStopTool reads exactly one part from its input then returns, stopping
// mid-stream while parts remain queued. It models a tool that errors or
// cancels before draining its input — the case that previously leaked the
// wrapper's input-interceptor goroutine.
type earlyStopTool struct {
	tool.BaseTool
	err error
}

func newEarlyStopTool(name string, err error) *earlyStopTool {
	t := &earlyStopTool{err: err}
	t.BaseTool.ToolName = name
	return t
}

func (t *earlyStopTool) Process(_ context.Context, in <-chan *model.Part, _ chan<- *model.Part) error {
	// Read a single part, then return without draining the rest of `in`.
	<-in
	return t.err
}

// waitGoroutinesSettle polls until the live goroutine count drops to at most
// baseline, or fails the test. It gives leaked goroutines a chance to be
// observed as not exiting.
func waitGoroutinesSettle(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= baseline {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.LessOrEqualf(t, runtime.NumGoroutine(), baseline,
		"goroutine count did not return to baseline %d (leak)", baseline)
}

func TestPipelineMetrics_Snapshot(t *testing.T) {
	pm := NewPipelineMetrics([]string{"step-a", "step-b"})
	require.Len(t, pm.Steps, 2)

	snap := pm.Snapshot()
	assert.Equal(t, "step-a", snap[0].Name)
	assert.Equal(t, "step-b", snap[1].Name)
	assert.Equal(t, int64(0), snap[0].PartsIn)
	assert.Equal(t, int64(0), snap[0].PartsOut)
}

func TestPipelineMetrics_Reset(t *testing.T) {
	pm := NewPipelineMetrics([]string{"x"})
	pm.Steps[0].PartsIn.Store(10)
	pm.Steps[0].PartsOut.Store(5)
	pm.Reset()

	snap := pm.Snapshot()
	assert.Equal(t, int64(0), snap[0].PartsIn)
	assert.Equal(t, int64(0), snap[0].PartsOut)
}

func TestMetricsTool_CountsParts(t *testing.T) {
	pm := NewPipelineMetrics([]string{"echo"})
	inner := newPassThroughTool("echo")
	wrapped := WrapWithMetrics([]tool.Tool{inner}, pm)

	in := make(chan *model.Part, 3)
	out := make(chan *model.Part, 3)

	// Feed 3 parts.
	for i := range 3 {
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: string(rune('a' + i))}}
	}
	close(in)

	err := wrapped[0].Process(context.Background(), in, out)
	require.NoError(t, err)
	close(out)

	// Drain output.
	var received int
	for range out {
		received++
	}
	assert.Equal(t, 3, received)

	snap := pm.Snapshot()
	assert.Equal(t, int64(3), snap[0].PartsIn)
	assert.Equal(t, int64(3), snap[0].PartsOut)
}

func TestMetricsTool_MultiToolChain(t *testing.T) {
	pm := NewPipelineMetrics([]string{"first", "second"})
	tools := WrapWithMetrics([]tool.Tool{
		newPassThroughTool("first"),
		newPassThroughTool("second"),
	}, pm)

	// Wire up: in → tool0 → mid → tool1 → out
	in := make(chan *model.Part, 5)
	mid := make(chan *model.Part, 5)
	out := make(chan *model.Part, 5)

	for i := range 5 {
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: string(rune('a' + i))}}
	}
	close(in)

	// Run first tool.
	err := tools[0].Process(context.Background(), in, mid)
	require.NoError(t, err)
	close(mid)

	// Run second tool.
	err = tools[1].Process(context.Background(), mid, out)
	require.NoError(t, err)
	close(out)

	// Drain.
	var count int
	for range out {
		count++
	}
	assert.Equal(t, 5, count)

	snap := pm.Snapshot()
	assert.Equal(t, int64(5), snap[0].PartsIn)
	assert.Equal(t, int64(5), snap[0].PartsOut)
	assert.Equal(t, int64(5), snap[1].PartsIn)
	assert.Equal(t, int64(5), snap[1].PartsOut)
}

func TestMetricsTool_DelegatesName(t *testing.T) {
	pm := NewPipelineMetrics([]string{"myTool"})
	inner := newPassThroughTool("myTool")
	wrapped := WrapWithMetrics([]tool.Tool{inner}, pm)

	assert.Equal(t, "myTool", wrapped[0].Name())
}

func TestWrapWithMetrics_PanicsOnMismatch(t *testing.T) {
	pm := NewPipelineMetrics([]string{"a"})
	assert.Panics(t, func() {
		WrapWithMetrics([]tool.Tool{newPassThroughTool("a"), newPassThroughTool("b")}, pm)
	})
}

// TestMetricsTool_NoInterceptorLeakOnEarlyInnerError verifies that when the
// inner tool stops reading mid-stream (returning an error before draining its
// input) the MetricsTool's input-interceptor goroutine still exits and Process
// returns instead of blocking forever on a send into the inner channel.
func TestMetricsTool_NoInterceptorLeakOnEarlyInnerError(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	pm := NewPipelineMetrics([]string{"early"})
	sentinel := errors.New("inner stopped early")
	wrapped := &MetricsTool{inner: newEarlyStopTool("early", sentinel), metrics: pm.Steps[0]}

	// Unbuffered channels so the interceptor must block on its forwarding send
	// once the inner tool stops reading — the leak condition.
	in := make(chan *model.Part)
	out := make(chan *model.Part)

	// Feed more parts than the inner tool will read; never close `in`, mirroring
	// an upstream stage that is still producing when the pipeline aborts. The
	// feeder is itself cancellable so this test goroutine doesn't masquerade as
	// the leak we're hunting for.
	feedStop := make(chan struct{})
	var feeders sync.WaitGroup
	feeders.Add(2)
	go func() {
		defer feeders.Done()
		for i := range 100 {
			select {
			case in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: string(rune('a' + i%26))}}:
			case <-feedStop:
				return
			}
		}
	}()

	// Drain any output the inner tool happens to emit so the output interceptor
	// never blocks (this inner tool emits none, but be defensive).
	go func() {
		defer feeders.Done()
		for range out {
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- wrapped.Process(context.Background(), in, out)
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, sentinel)
	case <-time.After(2 * time.Second):
		t.Fatal("MetricsTool.Process did not return after inner tool stopped early (interceptor leaked)")
	}

	// Tear down the test's own helper goroutines so only a genuine interceptor
	// leak can keep the goroutine count above baseline.
	close(feedStop)
	close(out)
	feeders.Wait()

	waitGoroutinesSettle(t, baseline)
}

// TestMetricsTool_NoInterceptorLeakOnCancel verifies that cancelling the
// context mid-stream unblocks the input interceptor's forwarding send so the
// goroutine exits and Process returns.
func TestMetricsTool_NoInterceptorLeakOnCancel(t *testing.T) {
	runtime.GC()
	baseline := runtime.NumGoroutine()

	pm := NewPipelineMetrics([]string{"cancel"})
	// blockTool reads one part then blocks on ctx so the wrapper's input
	// interceptor stalls on its next forwarding send.
	inner := newBlockUntilCancelTool("cancel")
	wrapped := &MetricsTool{inner: inner, metrics: pm.Steps[0]}

	in := make(chan *model.Part)
	out := make(chan *model.Part)
	feedStop := make(chan struct{})
	var feeders sync.WaitGroup
	feeders.Add(2)
	go func() {
		defer feeders.Done()
		for range 100 {
			select {
			case in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "x"}}:
			case <-feedStop:
				return
			}
		}
	}()
	go func() {
		defer feeders.Done()
		for range out {
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- wrapped.Process(ctx, in, out)
	}()

	// Let the pipeline get going, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("MetricsTool.Process did not return after cancel (interceptor leaked)")
	}

	close(feedStop)
	close(out)
	feeders.Wait()

	waitGoroutinesSettle(t, baseline)
}

// blockUntilCancelTool reads one part then blocks until the context is done,
// modelling a long-running inner tool that is aborted by cancellation.
type blockUntilCancelTool struct{ tool.BaseTool }

func newBlockUntilCancelTool(name string) *blockUntilCancelTool {
	t := &blockUntilCancelTool{}
	t.BaseTool.ToolName = name
	return t
}

func (t *blockUntilCancelTool) Process(ctx context.Context, in <-chan *model.Part, _ chan<- *model.Part) error {
	<-in
	<-ctx.Done()
	return ctx.Err()
}
