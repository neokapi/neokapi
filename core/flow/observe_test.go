package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/observe"
	"github.com/neokapi/neokapi/core/tool"
)

type recordingTracer struct{ spans []*recordedSpan }

func (r *recordingTracer) Start(ctx context.Context, op, name string) (context.Context, observe.Span) {
	s := &recordedSpan{op: op, name: name}
	r.spans = append(r.spans, s)
	return ctx, s
}

type recordedSpan struct {
	op, name string
	ended    bool
	err      error
}

func (s *recordedSpan) SetAttr(string, string) {}
func (s *recordedSpan) End(err error)          { s.ended, s.err = true, err }

// traced scopes a tracer to the call, so these tests leave no process state.
func traced(t *testing.T) (context.Context, *recordingTracer) {
	t.Helper()
	tr := &recordingTracer{}
	return observe.WithTracer(context.Background(), tr), tr
}

// drain runs a tool to completion over a closed input.
func drain(ctx context.Context, tl tool.Tool) error {
	in := make(chan *model.Part)
	out := make(chan *model.Part, 8)
	close(in)
	err := tl.Process(ctx, in, out)
	close(out)
	return err
}

// A tool's run is timed, and named for the tool so it aggregates.
//
// Naming it for the flow position instead would mint a row per pipeline shape
// and answer nothing about which tool is slow across them.
func TestWrapWithSpans_NamesTheTool(t *testing.T) {
	ctx, tr := traced(t)

	tools := WrapWithSpans([]tool.Tool{newPassThroughTool("segment")})
	require.NoError(t, drain(ctx, tools[0]))

	require.Len(t, tr.spans, 1)
	assert.Equal(t, "flow.tool", tr.spans[0].op)
	assert.Equal(t, "segment", tr.spans[0].name)
	assert.True(t, tr.spans[0].ended)
}

// A failing tool still ends its span, carrying the error.
func TestWrapWithSpans_FailureEndsTheSpan(t *testing.T) {
	ctx, tr := traced(t)

	boom := errors.New("tool failed")
	tools := WrapWithSpans([]tool.Tool{&failingTool{err: boom}})
	require.ErrorIs(t, drain(ctx, tools[0]), boom)

	require.Len(t, tr.spans, 1)
	assert.True(t, tr.spans[0].ended)
	assert.Equal(t, boom, tr.spans[0].err)
}

// The wrapper is transparent, so the executor still sees the tool it wrapped.
func TestWrapWithSpans_PassesThroughIdentity(t *testing.T) {
	inner := newPassThroughTool("segment")
	tools := WrapWithSpans([]tool.Tool{inner})

	assert.Equal(t, "segment", tools[0].Name())
	assert.Equal(t, inner, tools[0].(*ObservedTool).Unwrap())
	assert.Empty(t, WrapWithSpans(nil))
}

// A SessionTool keeps its session through the wrapper.
//
// This is the failure MetricsTool documents: a wrapper that collapses
// SessionProcess into Process silently drops the overlay cache, and resumable
// runs stop resuming with nothing to show that they stopped. Asserted on the
// session reaching the inner tool rather than on the span, because the span
// would look correct either way.
func TestWrapWithSpans_SessionSurvivesTheWrapper(t *testing.T) {
	ctx, tr := traced(t)

	inner := &sessionRecordingTool{}
	inner.BaseTool.ToolName = "translate"
	wrapped := WrapWithSpans([]tool.Tool{inner})[0]

	st, ok := wrapped.(tool.SessionTool)
	require.True(t, ok, "the wrapper must still satisfy SessionTool")

	in := make(chan *model.Part)
	out := make(chan *model.Part, 4)
	close(in)
	require.NoError(t, st.SessionProcess(ctx, nil, in, out))

	assert.True(t, inner.sessionSeen, "SessionProcess reached the inner tool")
	require.Len(t, tr.spans, 1)
	assert.Equal(t, "translate", tr.spans[0].name)
}

// Tracing off is the default for every embedded run, so it has to be free and
// impossible to get wrong.
func TestWrapWithSpans_WithoutATracer(t *testing.T) {
	tools := WrapWithSpans([]tool.Tool{newPassThroughTool("segment")})
	assert.NotPanics(t, func() { _ = drain(context.Background(), tools[0]) })
}

type failingTool struct {
	tool.BaseTool
	err error
}

func (f *failingTool) Process(context.Context, <-chan *model.Part, chan<- *model.Part) error {
	return f.err
}

type sessionRecordingTool struct {
	tool.BaseTool
	sessionSeen bool
}

func (s *sessionRecordingTool) Process(context.Context, <-chan *model.Part, chan<- *model.Part) error {
	return nil
}

func (s *sessionRecordingTool) SessionProcess(context.Context, blockstore.Session, <-chan *model.Part, chan<- *model.Part) error {
	s.sessionSeen = true
	return nil
}
