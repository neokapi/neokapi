package flow_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionCapturingTool is a SessionTool that records the session it was handed
// and which contract it was dispatched through.
type sessionCapturingTool struct {
	*tool.BaseTool
	sessionCalls atomic.Int32
	streamCalls  atomic.Int32
	gotSession   atomic.Value // blockstore.Session
}

func newSessionCapturingTool() *sessionCapturingTool {
	return &sessionCapturingTool{BaseTool: &tool.BaseTool{ToolName: "session-capture"}}
}

func (t *sessionCapturingTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	t.streamCalls.Add(1)
	return passThrough(ctx, in, out)
}

func (t *sessionCapturingTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	t.sessionCalls.Add(1)
	t.gotSession.Store(sess)
	return passThrough(ctx, in, out)
}

// wrapperCases are the tool.Tool wrappers this package splices around a tool.
// Each must be transparent to the session contract.
var wrapperCases = []struct {
	name string
	wrap func(tool.Tool) tool.Tool
}{
	{
		name: "tapping",
		wrap: func(inner tool.Tool) tool.Tool {
			return flow.NewTappingTool(inner, &testStreamingCollector{})
		},
	},
	{
		name: "metrics",
		wrap: func(inner tool.Tool) tool.Tool {
			return flow.WrapWithMetrics([]tool.Tool{inner}, flow.NewPipelineMetrics([]string{"step"}))[0]
		},
	},
	{
		name: "tracing",
		wrap: func(inner tool.Tool) tool.Tool {
			return flow.NewTracingTool(inner, "node", flow.NewTraceRecorder())
		},
	},
}

// TestToolWrappers_SessionReachesInnerTool verifies that a wrapper hands the
// session it was called with straight to the inner SessionTool.
func TestToolWrappers_SessionReachesInnerTool(t *testing.T) {
	for _, tt := range wrapperCases {
		t.Run(tt.name, func(t *testing.T) {
			inner := newSessionCapturingTool()
			wrapped := tt.wrap(inner)
			st, ok := wrapped.(tool.SessionTool)
			require.True(t, ok, "wrapper must still satisfy tool.SessionTool")

			sess, err := blockstore.NewMemoryStore().Begin(t.Context())
			require.NoError(t, err)
			defer sess.Close()

			in := make(chan *model.Part, 1)
			out := make(chan *model.Part, 1)
			in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "Hello")}
			close(in)

			require.NoError(t, st.SessionProcess(t.Context(), sess, in, out))
			close(out)

			assert.Equal(t, int32(1), inner.sessionCalls.Load(), "inner tool must receive the session contract")
			assert.Equal(t, int32(0), inner.streamCalls.Load())
			assert.Same(t, sess, inner.gotSession.Load(), "the session must be forwarded unchanged")
			assert.Len(t, drainParts(out), 1, "the wrapper must still forward the parts")
		})
	}
}

// TestToolWrappers_ExecutorDispatchesSessionThroughWrapper verifies the reason
// the forwarding matters: the executor's dispatch tests the tool it is given,
// so a wrapper that drops the interface silently routes a persistent run
// through the plain streaming path and disables the overlay cache.
func TestToolWrappers_ExecutorDispatchesSessionThroughWrapper(t *testing.T) {
	for _, tt := range wrapperCases {
		t.Run(tt.name, func(t *testing.T) {
			inner := newSessionCapturingTool()
			exec := flow.NewExecutor(flow.WithBlockStore(persistentStore{Store: blockstore.NewMemoryStore()}))

			f, err := flow.NewFlow("wrapped").AddTool(tt.wrap(inner)).Build()
			require.NoError(t, err)

			in, out, wait := exec.ExecuteWithChannels(t.Context(), f)
			go func() {
				in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "Hello")}
				close(in)
			}()
			parts := drainParts(out)
			require.NoError(t, wait())

			assert.Len(t, parts, 1)
			assert.Equal(t, int32(1), inner.sessionCalls.Load(), "persistent store must reach SessionProcess through the wrapper")
			assert.Equal(t, int32(0), inner.streamCalls.Load())
		})
	}
}

func drainParts(out <-chan *model.Part) []*model.Part {
	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	return parts
}
