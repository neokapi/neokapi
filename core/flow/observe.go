package flow

import (
	"context"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/observe"
	"github.com/neokapi/neokapi/core/tool"
)

// A flow's duration is the sum of its tools, and only the tools say which one
// was slow.
//
// The transaction around a convergence job says it took eight minutes. That
// restates the queue. The question a person actually has — segmentation or the
// model, this format or every format — is answered by the tools inside it, and
// by nothing else the job records.
//
// One span per tool invocation, not per Part: a flow moves thousands of Parts
// through a handful of tools, and a span each would cost more than it measures.

// spanOpFlowTool is shared by every tool span so a backend can aggregate them.
const spanOpFlowTool = "flow.tool"

// WrapWithSpans wraps each tool so its run opens a span. Returns a new slice;
// the original is not modified.
//
// A no-op unless a tracer is registered (core/observe), so wrapping
// unconditionally costs an interface dispatch and no allocation.
func WrapWithSpans(tools []tool.Tool) []tool.Tool {
	wrapped := make([]tool.Tool, len(tools))
	for i, t := range tools {
		wrapped[i] = &ObservedTool{inner: t}
	}
	return wrapped
}

// ObservedTool wraps a tool.Tool and times each run.
type ObservedTool struct{ inner tool.Tool }

// Unwrap exposes the wrapped tool, so a caller that needs to recognise
// something further down the stack can look through this wrapper.
func (o *ObservedTool) Unwrap() tool.Tool { return o.inner }

// Name returns the wrapped tool's name.
func (o *ObservedTool) Name() string { return o.inner.Name() }

// Description returns the wrapped tool's description.
func (o *ObservedTool) Description() string { return o.inner.Description() }

// Config returns the wrapped tool's configuration.
func (o *ObservedTool) Config() tool.ToolConfig { return o.inner.Config() }

// SetConfig applies configuration to the wrapped tool.
func (o *ObservedTool) SetConfig(c tool.ToolConfig) error { return o.inner.SetConfig(c) }

// Process times one run of the wrapped tool.
func (o *ObservedTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	ctx, span := observe.Start(ctx, spanOpFlowTool, o.inner.Name())
	err := o.inner.Process(ctx, in, out)
	span.End(err)
	return err
}

// SessionProcess forwards the session contract to the wrapped tool when it is a
// SessionTool, while still timing the run.
//
// Forwarded rather than collapsed into Process for the reason MetricsTool gives:
// a wrapped SessionTool that loses this silently loses its overlay cache, and
// resumable runs stop resuming with nothing to show that they stopped.
func (o *ObservedTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	st, ok := o.inner.(tool.SessionTool)
	if !ok {
		return o.Process(ctx, in, out)
	}
	ctx, span := observe.Start(ctx, spanOpFlowTool, o.inner.Name())
	err := st.SessionProcess(ctx, sess, in, out)
	span.End(err)
	return err
}
