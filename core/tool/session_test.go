package tool_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// annotatingTool is a minimal example of a SessionTool that reads
// each incoming Part from the stream but also consults the session
// for a pre-existing termbase annotation, and writes a QA sidecar
// per block. Purpose: assert the SessionTool contract composes with
// the existing streaming contract end-to-end.
type annotatingTool struct {
	cfg tool.ToolConfig
}

func (t *annotatingTool) Name() string                        { return "annotating" }
func (t *annotatingTool) Description() string                 { return "test SessionTool" }
func (t *annotatingTool) Config() tool.ToolConfig             { return t.cfg }
func (t *annotatingTool) SetConfig(cfg tool.ToolConfig) error { t.cfg = cfg; return nil }

func (t *annotatingTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (t *annotatingTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	// For each streamed Part, write a "annotations/qa" sidecar keyed
	// on its block hash. Then pass through. Real tools would also
	// read prior sidecars (termbase matches, TM) via sess.GetSidecar.
	for p := range in {
		if p != nil {
			if blk, ok := p.Resource.(*model.Block); ok && blk != nil {
				if err := sess.PutSidecar(blockstore.Sidecar{
					Kind:      "annotations/qa",
					BlockHash: blk.ID,
					Payload:   []byte(`{"ok":true}`),
				}); err != nil {
					return err
				}
			}
		}
		select {
		case out <- p:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

var _ tool.SessionTool = (*annotatingTool)(nil)

// Verify SessionTool is assignable to Tool.
var _ tool.Tool = (tool.SessionTool)(nil)

func TestSessionTool_assignableAsTool(t *testing.T) {
	var st tool.SessionTool = &annotatingTool{}
	var base tool.Tool = st
	assert.Equal(t, "annotating", base.Name())
}

func TestSessionTool_writesSidecarsViaSession(t *testing.T) {
	store := blockstore.NewMemoryStore()
	defer store.Close()

	sess, err := store.Begin(context.Background())
	require.NoError(t, err)

	// Seed two Parts carrying Blocks.
	in := make(chan *model.Part, 2)
	out := make(chan *model.Part, 2)
	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{ID: "h1"}}
	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{ID: "h2"}}
	close(in)

	tl := &annotatingTool{}
	require.NoError(t, tl.SessionProcess(context.Background(), sess, in, out))
	close(out)

	// Both Parts flowed through.
	count := 0
	for range out {
		count++
	}
	assert.Equal(t, 2, count)

	// Session has the sidecars the tool wrote.
	sc, err := sess.GetSidecar("annotations/qa", "h1")
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(sc.Payload))
	sc, err = sess.GetSidecar("annotations/qa", "h2")
	require.NoError(t, err)
	assert.Equal(t, `{"ok":true}`, string(sc.Payload))

	require.NoError(t, sess.Commit())
}

// RunWithSession is a tiny executor-level helper that demonstrates
// how a real flow wires a session to whichever contract a tool
// implements. The production flow executor can grow a similar
// dispatch once every built-in tool has been evaluated for
// SessionTool adoption (follow-up per AD-046 rollout).
func TestRunWithSession_dispatchesSessionToolWhenAvailable(t *testing.T) {
	store := blockstore.NewMemoryStore()
	defer store.Close()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	tl := &annotatingTool{}
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{ID: "hX"}}
	close(in)
	require.NoError(t, runWithSession(context.Background(), tl, sess, in, out))
	close(out)
	<-out // drain

	sc, err := sess.GetSidecar("annotations/qa", "hX")
	require.NoError(t, err)
	assert.NotEmpty(t, sc.Payload)

	// Klz just to touch the klf Block type so goimports stays sensible.
	assert.NotNil(t, klf.Block{})
}

// runWithSession is what the flow executor will call; if the tool
// implements SessionTool, route with session context, else fall
// back to the streaming Process. Inlined here for the test — the
// production equivalent lives in core/flow.
func runWithSession(
	ctx context.Context,
	t tool.Tool,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	if st, ok := t.(tool.SessionTool); ok {
		return st.SessionProcess(ctx, sess, in, out)
	}
	return t.Process(ctx, in, out)
}
