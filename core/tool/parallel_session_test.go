package tool

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// sessionRecorder is a SessionTool that records which entry point ran.
type sessionRecorder struct {
	*BaseTool
	sawSession bool
	sawPlain   bool
}

func newSessionRecorder() *sessionRecorder {
	r := &sessionRecorder{
		BaseTool: &BaseTool{
			ToolName:        "session-recorder",
			ToolDescription: "records the entry point",
			Annotate:        func(BlockView) error { return nil },
		},
	}
	return r
}

func (r *sessionRecorder) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	r.sawPlain = true
	return r.BaseTool.Process(ctx, in, out)
}

func (r *sessionRecorder) SessionProcess(ctx context.Context, _ blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	r.sawSession = true
	return r.BaseTool.Process(ctx, in, out)
}

var _ SessionTool = (*sessionRecorder)(nil)

// TestParallelBlockToolForwardsSession asserts the wrapper both presents itself
// as a SessionTool and routes the session to the inner tool.
//
// A wrapper that implements only Tool sends the executor down the sessionless
// path, which turns a tool's persistent overlay cache into a per-run rebuild
// with nothing logged.
func TestParallelBlockToolForwardsSession(t *testing.T) {
	inner := newSessionRecorder()
	wrapped := NewParallelBlockTool(inner, 4)

	var asSession *SessionTool
	require.Implements(t, asSession, wrapped,
		"the wrapper must be visible as a SessionTool")

	require.NoError(t, drainThrough(t, func(in <-chan *model.Part, out chan<- *model.Part) error {
		return wrapped.SessionProcess(context.Background(), nil, in, out)
	}))
	require.True(t, inner.sawSession, "the session must reach the inner tool")
	require.False(t, inner.sawPlain, "the sessionless path must not be taken")
}

// TestParallelBlockToolSessionFallsBackForPlainTool covers the other side: a
// wrapped tool that is not a SessionTool still runs, through Process.
func TestParallelBlockToolSessionFallsBackForPlainTool(t *testing.T) {
	inner := &BaseTool{
		ToolName:        "plain",
		ToolDescription: "no session",
		Annotate:        func(BlockView) error { return nil },
	}
	wrapped := NewParallelBlockTool(inner, 4)

	require.NoError(t, drainThrough(t, func(in <-chan *model.Part, out chan<- *model.Part) error {
		return wrapped.SessionProcess(context.Background(), nil, in, out)
	}))
}

// drainThrough feeds one part through run and collects the output, so a tool
// that never emits cannot pass by deadlocking.
func drainThrough(t *testing.T, run func(<-chan *model.Part, chan<- *model.Part) error) error {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 4)
	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{ID: "b1"}}
	close(in)
	err := run(in, out)
	close(out)
	count := 0
	for range out {
		count++
	}
	if err == nil {
		require.Equal(t, 1, count, "the part must reach the output")
	}
	return err
}
