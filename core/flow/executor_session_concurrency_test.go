package flow_test

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/require"
)

// unsafePersistentStore advertises Persistent=true (so the executor takes the
// SessionProcess path) and hands out a session backed by a plain, unguarded
// map. It deliberately does NO internal locking — it stands in for a real
// persistent provider whose single *sql.Tx is not safe for concurrent use.
// The executor's syncSession wrapper is what must serialize access; without
// it, concurrent SessionTools race on this map and the race detector fires.
type unsafePersistentStore struct{ sess *unsafeSession }

func newUnsafePersistentStore() *unsafePersistentStore {
	return &unsafePersistentStore{sess: &unsafeSession{
		blocks:   map[string]*blockstore.Block{},
		overlays: map[string]blockstore.Overlay{},
	}}
}

func (s *unsafePersistentStore) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{RandomAccess: true, Writable: true, Persistent: true}
}

func (s *unsafePersistentStore) Begin(context.Context) (blockstore.Session, error) {
	return s.sess, nil
}

func (s *unsafePersistentStore) Close() error { return nil }

// unsafeSession is intentionally not concurrency-safe.
type unsafeSession struct {
	blocks   map[string]*blockstore.Block
	overlays map[string]blockstore.Overlay
}

func (s *unsafeSession) Capabilities() blockstore.Capabilities {
	return blockstore.Capabilities{RandomAccess: true, Writable: true, Persistent: true}
}

func (s *unsafeSession) Blocks(blockstore.BlockFilter) iter.Seq2[*blockstore.Block, error] {
	return func(yield func(*blockstore.Block, error) bool) {
		for _, b := range s.blocks { // unguarded map read
			if !yield(b, nil) {
				return
			}
		}
	}
}

func (s *unsafeSession) GetBlock(hash string) (*blockstore.Block, error) {
	if b, ok := s.blocks[hash]; ok { // unguarded map read
		return b, nil
	}
	return nil, blockstore.ErrNotFound
}

func (s *unsafeSession) PutBlock(_ string, b *blockstore.Block) error {
	s.blocks[b.Hash] = b // unguarded map write
	return nil
}

func (s *unsafeSession) GetOverlay(kind, blockHash string) (blockstore.Overlay, error) {
	if o, ok := s.overlays[kind+"\x00"+blockHash]; ok { // unguarded map read
		return o, nil
	}
	return blockstore.Overlay{}, blockstore.ErrNotFound
}

func (s *unsafeSession) PutOverlay(o blockstore.Overlay) error {
	s.overlays[o.Kind+"\x00"+o.BlockHash] = o // unguarded map write
	return nil
}

func (s *unsafeSession) ListOverlays(string) iter.Seq2[blockstore.Overlay, error] {
	return func(yield func(blockstore.Overlay, error) bool) {
		for _, o := range s.overlays { // unguarded map read
			if !yield(o, nil) {
				return
			}
		}
	}
}

func (s *unsafeSession) Commit() error   { return nil }
func (s *unsafeSession) Rollback() error { return nil }
func (s *unsafeSession) Close() error    { return nil }

// sessionHammerTool is a SessionTool that, while passing parts through,
// performs many random-access reads/writes and full iterations against its
// session. When several run concurrently against one shared session, any
// missing serialization corrupts shared state.
type sessionHammerTool struct {
	*tool.BaseTool
	ops int
}

func newSessionHammerTool(name string, ops int) *sessionHammerTool {
	return &sessionHammerTool{BaseTool: &tool.BaseTool{ToolName: name}, ops: ops}
}

// Process is required by the Tool interface; the persistent-store gate routes
// to SessionProcess, but keep a real pass-through so the tool is valid.
func (t *sessionHammerTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	return passThrough(ctx, in, out)
}

func (t *sessionHammerTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	for i := range t.ops {
		key := fmt.Sprintf("%s-%d", t.Name(), i)
		if err := sess.PutBlock("c", &blockstore.Block{Hash: key, Translatable: true}); err != nil {
			return err
		}
		if err := sess.PutOverlay(blockstore.Overlay{
			Kind: "annotations/" + t.Name(), BlockHash: key, Payload: []byte(`{"v":1}`),
		}); err != nil {
			return err
		}
		_, _ = sess.GetBlock(key)
		_, _ = sess.GetOverlay("annotations/"+t.Name(), key)
		for _, err := range sess.Blocks(blockstore.BlockFilter{}) {
			if err != nil {
				return err
			}
		}
		for _, err := range sess.ListOverlays("annotations/" + t.Name()) {
			if err != nil {
				return err
			}
		}
	}
	return passThrough(ctx, in, out)
}

var _ tool.SessionTool = (*sessionHammerTool)(nil)

// TestExecutor_ConcurrentSessionToolsShareGuardedSession runs several
// SessionTools concurrently against a single shared persistent session via
// ExecuteWithChannels. The backing session is deliberately not
// concurrency-safe; the executor's syncSession wrapper is the only thing
// serializing access. Run under -race: without the guard the shared maps are
// accessed concurrently and the detector fires; with it the run is clean.
// Guards audit #31 / issue #609.
func TestExecutor_ConcurrentSessionToolsShareGuardedSession(t *testing.T) {
	store := newUnsafePersistentStore()
	require.True(t, store.Capabilities().Persistent,
		"store must be persistent so the SessionProcess path is taken")

	const ops = 50
	fb := flow.NewFlow("hammer")
	for i := range 4 {
		fb = fb.AddTool(newSessionHammerTool(fmt.Sprintf("hammer%d", i), ops))
	}
	f, err := fb.Build()
	require.NoError(t, err)

	exec := flow.NewExecutor(flow.WithBlockStore(store))

	in, out, wait := exec.ExecuteWithChannels(context.Background(), f)
	go func() {
		for i := range 8 {
			blk := model.NewRunsBlock(fmt.Sprintf("p%d", i),
				[]model.Run{{Text: &model.TextRun{Text: "hello"}}})
			blk.Translatable = true
			in <- &model.Part{Type: model.PartBlock, Resource: blk}
		}
		close(in)
	}()

	var n int
	for range out {
		n++
	}
	require.NoError(t, wait(), "guarded concurrent session run must commit cleanly")
	require.Equal(t, 8, n, "all parts must flow through the chain")
}
