package flow

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// StepSnapshot is a point-in-time read of a single step's counters.
type StepSnapshot struct {
	Name     string `json:"name"`
	PartsIn  int64  `json:"parts_in"`
	PartsOut int64  `json:"parts_out"`
}

// StepMetrics holds atomic counters for a single pipeline step.
type StepMetrics struct {
	Name     string
	PartsIn  atomic.Int64
	PartsOut atomic.Int64
}

// PipelineMetrics collects lightweight, lock-free per-step counters
// for a streaming pipeline. Safe for concurrent use.
type PipelineMetrics struct {
	Steps []*StepMetrics
}

// NewPipelineMetrics creates a PipelineMetrics with one StepMetrics per name.
func NewPipelineMetrics(stepNames []string) *PipelineMetrics {
	steps := make([]*StepMetrics, len(stepNames))
	for i, name := range stepNames {
		steps[i] = &StepMetrics{Name: name}
	}
	return &PipelineMetrics{Steps: steps}
}

// Snapshot reads all counters and returns a value-type slice.
func (pm *PipelineMetrics) Snapshot() []StepSnapshot {
	snaps := make([]StepSnapshot, len(pm.Steps))
	for i, s := range pm.Steps {
		snaps[i] = StepSnapshot{
			Name:     s.Name,
			PartsIn:  s.PartsIn.Load(),
			PartsOut: s.PartsOut.Load(),
		}
	}
	return snaps
}

// Reset zeros all counters. Call between files to restart step progress.
func (pm *PipelineMetrics) Reset() {
	for _, s := range pm.Steps {
		s.PartsIn.Store(0)
		s.PartsOut.Store(0)
	}
}

// WrapWithMetrics wraps each tool with a MetricsTool at the corresponding
// PipelineMetrics index. Returns a new slice; the original is not modified.
// Panics if len(tools) != len(pm.Steps).
func WrapWithMetrics(tools []tool.Tool, pm *PipelineMetrics) []tool.Tool {
	if len(tools) != len(pm.Steps) {
		panic("WrapWithMetrics: tool count does not match step count")
	}
	wrapped := make([]tool.Tool, len(tools))
	for i, t := range tools {
		wrapped[i] = &MetricsTool{inner: t, metrics: pm.Steps[i]}
	}
	return wrapped
}

// ---------------------------------------------------------------------------
// MetricsTool
// ---------------------------------------------------------------------------

// MetricsTool wraps a tool.Tool and increments atomic counters for each
// Part that enters and exits the tool. No locks, no allocations per part.
type MetricsTool struct {
	inner   tool.Tool
	metrics *StepMetrics
}

// Name returns the wrapped tool's name.
func (m *MetricsTool) Name() string { return m.inner.Name() }

// Description returns the wrapped tool's description.
func (m *MetricsTool) Description() string { return m.inner.Description() }

// Config returns the wrapped tool's configuration.
func (m *MetricsTool) Config() tool.ToolConfig { return m.inner.Config() }

// SetConfig applies configuration to the wrapped tool.
func (m *MetricsTool) SetConfig(c tool.ToolConfig) error { return m.inner.SetConfig(c) }

// Process intercepts parts flowing through the inner tool, incrementing
// PartsIn on input and PartsOut on output.
//
// Channel ownership follows the same pattern as TracingTool.Process:
// create intermediate channels, close after inner.Process returns,
// WaitGroup to wait for both interceptors to drain.
//
// The input interceptor must not block forever if the inner tool stops
// reading early (mid-stream error or context cancellation): its forwarding
// send selects on ctx.Done() and a stop channel that is closed once
// inner.Process returns. Both interceptor goroutines are joined before
// Process returns so neither leaks on the happy path or the cancel/error path.
func (m *MetricsTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	return m.intercept(ctx, in, out, func(innerIn <-chan *model.Part, innerOut chan<- *model.Part) error {
		return m.inner.Process(ctx, innerIn, innerOut)
	})
}

// SessionProcess forwards the session contract to the wrapped tool when it
// is a SessionTool, while still counting parts — so persistent overlay
// caching survives the metrics wrapper (without this, a wrapped SessionTool
// silently loses its cache, defeating resumable runs).
func (m *MetricsTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	st, ok := m.inner.(tool.SessionTool)
	if !ok {
		return m.Process(ctx, in, out)
	}
	return m.intercept(ctx, in, out, func(innerIn <-chan *model.Part, innerOut chan<- *model.Part) error {
		return st.SessionProcess(ctx, sess, innerIn, innerOut)
	})
}

// intercept runs run() with metrics-counting channels spliced around the
// inner tool's in/out.
func (m *MetricsTool) intercept(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part, run func(<-chan *model.Part, chan<- *model.Part) error) error {
	metricsIn := make(chan *model.Part, cap(in))
	metricsOut := make(chan *model.Part, cap(out))

	// stop is closed once the inner tool returns, signalling the input
	// interceptor to abandon any pending forward and exit.
	stop := make(chan struct{})

	var interceptors sync.WaitGroup
	interceptors.Add(2)

	// Input interceptor: count parts entering, forward to inner.
	go func() {
		defer interceptors.Done()
		defer close(metricsIn)
		for part := range in {
			m.metrics.PartsIn.Add(1)
			select {
			case metricsIn <- part:
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()

	// Output interceptor: count parts exiting, forward to caller.
	go func() {
		defer interceptors.Done()
		for part := range metricsOut {
			m.metrics.PartsOut.Add(1)
			out <- part
		}
	}()

	// Run the inner tool.
	err := run(metricsIn, metricsOut)

	// Signal the input interceptor to stop forwarding (the inner tool is no
	// longer reading metricsIn) and close metricsOut so the output
	// interceptor terminates.
	close(stop)
	close(metricsOut)

	// Join both interceptors so neither goroutine outlives Process.
	interceptors.Wait()

	return err
}

// Verify MetricsTool implements tool.Tool and tool.SessionTool at compile time.
var (
	_ tool.Tool        = (*MetricsTool)(nil)
	_ tool.SessionTool = (*MetricsTool)(nil)
)
