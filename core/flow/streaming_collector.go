package flow

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// StreamingCollector extends Collector with inline observation of Parts as they
// flow through the pipeline. Observe is called synchronously for each Part, so
// implementations must be fast. Heavy aggregation should use internal buffering.
type StreamingCollector interface {
	Collector
	// Observe is called inline as each Part flows through the pipeline.
	// Must be fast — called synchronously in the pipeline path.
	Observe(part *model.Part)
}

// TappingTool wraps a tool.Tool and calls a StreamingCollector's Observe method
// on each Part as it passes through, without modifying the Part stream.
type TappingTool struct {
	inner     tool.Tool
	collector StreamingCollector
}

// NewTappingTool creates a TappingTool that observes Parts via the collector
// while delegating processing to the inner tool.
func NewTappingTool(inner tool.Tool, collector StreamingCollector) *TappingTool {
	return &TappingTool{inner: inner, collector: collector}
}

// Name returns the wrapped tool's name.
func (t *TappingTool) Name() string { return t.inner.Name() }

// Description returns the wrapped tool's description.
func (t *TappingTool) Description() string { return t.inner.Description() }

// Config returns the wrapped tool's configuration.
func (t *TappingTool) Config() tool.ToolConfig { return t.inner.Config() }

// SetConfig applies configuration to the wrapped tool.
func (t *TappingTool) SetConfig(c tool.ToolConfig) error { return t.inner.SetConfig(c) }

// Process intercepts the output of the inner tool, calling Observe on each Part
// before forwarding it to the output channel.
func (t *TappingTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	tappedOut := make(chan *model.Part, cap(out))

	// Run the inner tool writing to our interceptor channel.
	errCh := make(chan error, 1)
	go func() {
		errCh <- t.inner.Process(ctx, in, tappedOut)
		close(tappedOut)
	}()

	// Observe and forward each Part.
	for part := range tappedOut {
		t.collector.Observe(part)
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return <-errCh
}

// Verify TappingTool implements tool.Tool at compile time.
var _ tool.Tool = (*TappingTool)(nil)
