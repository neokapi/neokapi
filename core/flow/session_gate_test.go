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

// dispatchRecordingTool is a SessionTool that records which contract the
// executor dispatched it through. Both paths are pass-throughs producing
// identical output, so the only observable is which counter ticked.
// It embeds *tool.BaseTool to satisfy the full Tool interface
// (Config/SetConfig) while overriding Process to record the stream path.
type dispatchRecordingTool struct {
	*tool.BaseTool
	sessionCalls atomic.Int32
	streamCalls  atomic.Int32
}

func newDispatchRecordingTool() *dispatchRecordingTool {
	return &dispatchRecordingTool{BaseTool: &tool.BaseTool{ToolName: "dispatch-recorder"}}
}

func (t *dispatchRecordingTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	t.streamCalls.Add(1)
	return passThrough(ctx, in, out)
}

func (t *dispatchRecordingTool) SessionProcess(ctx context.Context, _ blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	t.sessionCalls.Add(1)
	return passThrough(ctx, in, out)
}

func passThrough(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// persistentStore wraps a Store and forces Persistent=true on the
// capabilities it (and its sessions) advertise, so the executor takes
// the SessionProcess path. Used to assert the gate flips correctly.
type persistentStore struct{ blockstore.Store }

func (p persistentStore) Capabilities() blockstore.Capabilities {
	c := p.Store.Capabilities()
	c.Persistent = true
	return c
}

func (p persistentStore) Begin(ctx context.Context) (blockstore.Session, error) {
	sess, err := p.Store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return persistentSession{Session: sess}, nil
}

type persistentSession struct{ blockstore.Session }

func (p persistentSession) Capabilities() blockstore.Capabilities {
	c := p.Session.Capabilities()
	c.Persistent = true
	return c
}

// runRecorder pushes a single block through the executor's channel API
// with the given recorder as the only tool, and drains the output.
func runRecorder(t *testing.T, exec *flow.DefaultExecutor, rec *dispatchRecordingTool) {
	t.Helper()
	f, err := flow.NewFlow("gate").AddTool(rec).Build()
	require.NoError(t, err)

	in, out, wait := exec.ExecuteWithChannels(context.Background(), f)
	go func() {
		blk := model.NewRunsBlock("h1", []model.Run{{Text: &model.TextRun{Text: "Hello"}}})
		blk.Translatable = true
		in <- &model.Part{Type: model.PartBlock, Resource: blk}
		close(in)
	}()
	var n int
	for range out {
		n++
	}
	require.NoError(t, wait())
	require.Equal(t, 1, n, "the one block must flow through")
}

// TestSessionGate_OneShotMemoryStoreUsesStreaming verifies that with the
// default ephemeral in-memory store, a SessionTool is dispatched through
// the plain streaming Process path — not SessionProcess (#608, S5).
func TestSessionGate_OneShotMemoryStoreUsesStreaming(t *testing.T) {
	rec := newDispatchRecordingTool()
	exec := flow.NewExecutor() // default: NewMemoryStore (not persistent)
	runRecorder(t, exec, rec)

	assert.Equal(t, int32(1), rec.streamCalls.Load(), "one-shot run must use Tool.Process")
	assert.Equal(t, int32(0), rec.sessionCalls.Load(), "one-shot run must NOT use SessionProcess")
}

// TestSessionGate_PersistentStoreUsesSession verifies that when the store
// advertises Persistent=true, the executor dispatches the SessionTool
// through SessionProcess so its overlay cache is exercised.
func TestSessionGate_PersistentStoreUsesSession(t *testing.T) {
	rec := newDispatchRecordingTool()
	store := persistentStore{Store: blockstore.NewMemoryStore()}
	exec := flow.NewExecutor(flow.WithBlockStore(store))
	runRecorder(t, exec, rec)

	assert.Equal(t, int32(1), rec.sessionCalls.Load(), "persistent store must use SessionProcess")
	assert.Equal(t, int32(0), rec.streamCalls.Load(), "persistent store must NOT use Tool.Process")
}

// TestSessionGate_MemoryStoreNotPersistent pins the capability flag the
// gate keys off, so a future change to memory store capabilities trips
// this test rather than silently changing dispatch behavior.
func TestSessionGate_MemoryStoreNotPersistent(t *testing.T) {
	assert.False(t, blockstore.NewMemoryStore().Capabilities().Persistent,
		"the default in-memory store must remain non-persistent")
}
