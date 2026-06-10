package flow

import (
	"context"
	"fmt"
	"iter"
	"runtime"
	"sync"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"golang.org/x/sync/errgroup"
)

// Executor orchestrates the execution of a Flow across batch items.
type Executor interface {
	// Execute runs the Flow over the given batch items.
	Execute(ctx context.Context, f *Flow, items []*Item) error
}

// FlowExecutor is a deprecated alias for [Executor].
//
// Deprecated: Use [Executor] instead.
type FlowExecutor = Executor

// ExecutorConfig holds configuration for the DefaultExecutor.
type ExecutorConfig struct {
	MaxConcurrency int         // 0 = runtime.NumCPU(); 1 = sequential
	ChannelSize    int         // default 64
	FailFast       bool        // default true; cancel remaining on first error
	Collectors     []Collector // aggregators fed after each document completes
	// Store backs the BlockStore session each item is processed
	// against. Tools that implement tool.SessionTool receive the
	// session; stream-only tools are unaffected. Defaults to
	// blockstore.NewMemoryStore() (ephemeral).
	Store blockstore.Store
}

// ExecutorOption is a functional option for configuring a DefaultExecutor.
type ExecutorOption func(*ExecutorConfig)

// WithMaxConcurrency sets the maximum number of documents processed in parallel.
// 0 means runtime.NumCPU(). 1 means sequential.
func WithMaxConcurrency(n int) ExecutorOption {
	return func(c *ExecutorConfig) {
		c.MaxConcurrency = n
	}
}

// WithChannelSize sets the buffer size for inter-tool channels.
func WithChannelSize(n int) ExecutorOption {
	return func(c *ExecutorConfig) {
		if n > 0 {
			c.ChannelSize = n
		}
	}
}

// WithFailFast controls whether to cancel all remaining documents
// when one fails. Default is true.
func WithFailFast(b bool) ExecutorOption {
	return func(c *ExecutorConfig) {
		c.FailFast = b
	}
}

// WithCollectors registers Collectors that aggregate results across documents.
func WithCollectors(c ...Collector) ExecutorOption {
	return func(cfg *ExecutorConfig) {
		cfg.Collectors = append(cfg.Collectors, c...)
	}
}

// WithBlockStore sets the BlockStore used for SessionTool dispatch.
// Defaults to blockstore.NewMemoryStore(). Pass a persistent store
// (e.g. sqlitestore.New for a project's .kapi/cache/blocks.db) to enable
// incremental work across runs.
func WithBlockStore(store blockstore.Store) ExecutorOption {
	return func(cfg *ExecutorConfig) {
		cfg.Store = store
	}
}

// DefaultExecutor runs tools concurrently using goroutines and channels.
type DefaultExecutor struct {
	config ExecutorConfig
}

// DefaultFlowExecutor is a deprecated alias for [DefaultExecutor].
//
// Deprecated: Use [DefaultExecutor] instead.
type DefaultFlowExecutor = DefaultExecutor

// NewExecutor creates a new DefaultExecutor with the given options.
// With no options, it behaves sequentially with channel size 64 and fail-fast enabled.
func NewExecutor(opts ...ExecutorOption) *DefaultExecutor {
	cfg := ExecutorConfig{
		MaxConcurrency: 1,
		ChannelSize:    64,
		FailFast:       true,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Store == nil {
		cfg.Store = blockstore.NewMemoryStore()
	}
	return &DefaultExecutor{config: cfg}
}

// NewFlowExecutor is a deprecated alias for [NewExecutor].
//
// Deprecated: Use [NewExecutor] instead.
func NewFlowExecutor(opts ...ExecutorOption) *DefaultExecutor {
	return NewExecutor(opts...)
}

// SetChannelSize configures the buffer size for inter-tool channels.
func (e *DefaultExecutor) SetChannelSize(size int) {
	if size > 0 {
		e.config.ChannelSize = size
	}
}

// Execute processes Items through the tool chain.
// When MaxConcurrency > 1 (or 0 for NumCPU), items are processed
// in parallel using a semaphore-bounded fan-out pattern.
func (e *DefaultExecutor) Execute(ctx context.Context, f *Flow, items []*Item) error {
	maxConc := e.config.MaxConcurrency
	if maxConc == 0 {
		maxConc = runtime.NumCPU()
	}

	// Single-item or sequential: use the simple path.
	if maxConc == 1 || len(items) <= 1 {
		for _, item := range items {
			parts, err := e.processItemCollect(ctx, f, item)
			if err != nil {
				return fmt.Errorf("processing %s: %w", item.Input.URI, err)
			}
			if err := e.feedCollectors(ctx, item, parts); err != nil {
				return fmt.Errorf("collector error for %s: %w", item.Input.URI, err)
			}
		}
		return nil
	}

	// Parallel execution with bounded concurrency.
	g, ctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxConc)

	for _, item := range items {
		if e.config.FailFast {
			// Check context before acquiring semaphore to fail fast.
			select {
			case <-ctx.Done():
				return g.Wait()
			default:
			}
		}
		sem <- struct{}{} // acquire slot (blocks if at capacity)
		g.Go(func() error {
			defer func() { <-sem }() // release slot
			parts, err := e.processItemCollect(ctx, f, item)
			if err != nil {
				return fmt.Errorf("processing %s: %w", item.Input.URI, err)
			}
			return e.feedCollectors(ctx, item, parts)
		})
	}
	return g.Wait()
}

// processItemCollect processes a single Item through the tool chain
// and returns the collected output parts.
func (e *DefaultExecutor) processItemCollect(ctx context.Context, f *Flow, item *Item) ([]*model.Part, error) {
	tools, err := e.resolveTools(f)
	if err != nil {
		return nil, fmt.Errorf("resolve tools: %w", err)
	}

	if len(tools) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Open a session for this item. Each item gets its own session so
	// parallel items don't share write state; the default MemoryStore
	// supports concurrent sessions. Tools that implement SessionTool
	// receive the session for random access; stream-only tools are
	// unaffected. Commit on success, Rollback on any tool error.
	rawSession, err := e.config.Store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open blockstore session: %w", err)
	}
	// All tools share this one session and run in their own goroutines, so
	// guard it against concurrent use (see syncSession).
	session := newSyncSession(rawSession)

	g, ctx := errgroup.WithContext(ctx)

	// Create channels: one input, one between each pair of tools, one output
	channels := make([]chan *model.Part, len(tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.config.ChannelSize)
	}

	// Launch a goroutine for each tool
	for i, t := range tools {
		in := channels[i]
		out := channels[i+1]

		g.Go(func() error {
			defer close(out)
			if err := runTool(ctx, t, session, in, out); err != nil {
				return fmt.Errorf("tool %s: %w", t.Name(), err)
			}
			return nil
		})
	}

	// Collect output parts from the final channel. The collector only
	// drains a channel and cannot fail, so it has no error path.
	outCh := channels[len(channels)-1]
	var parts []*model.Part
	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range outCh {
			parts = append(parts, p)
		}
	}()

	// The input channel is the first channel; since processItemCollect
	// is called without feeding input (that's the Execute path's job),
	// we close it immediately to signal no input to process.
	close(channels[0])

	err = g.Wait()
	<-done // wait for collector goroutine

	if err != nil {
		_ = session.Rollback()
		return nil, fmt.Errorf("execute tool chain: %w", err)
	}
	if err := session.Commit(); err != nil {
		return nil, fmt.Errorf("commit session: %w", err)
	}
	return parts, nil
}

// syncSession wraps a blockstore.Session so that concurrent SessionTools
// sharing one session don't race on its underlying transaction.
//
// The executor opens a single session per item (processItemCollect) or per
// channel pipeline (ExecuteWithChannels) and hands it to every tool. Tools
// run in their own goroutines, so when a persistent store is wired (e.g.
// sqlitestore.New via WithBlockStore) two SessionTools can call
// SessionProcess against the same session — and a persistent session is
// backed by a single *sql.Tx, which database/sql does not allow to be used
// concurrently (concurrent statements on one transaction corrupt the
// connection). syncSession serializes every Session method behind a mutex so
// that contract holds regardless of the provider.
//
// Iterator-returning methods (Blocks, ListOverlays) hold the lock for the
// whole iteration, because the underlying provider keeps its *sql.Rows open
// across yields; releasing the lock between rows would let another tool's
// query interleave on the same transaction.
//
// Residual concern: this guard makes concurrent access *safe* (no torn
// transaction state), not concurrent. A persistent SQLite-backed session
// serializes its tools' store access; the throughput win from a persistent
// store is cross-run caching, not intra-run parallelism, so this is the
// intended trade-off. The deeper fix — per-tool transactions, or a session
// type that fans out to independent connections — lives in core/blockstore
// and is out of scope here (tracked by #609).
type syncSession struct {
	mu    *sync.Mutex
	inner blockstore.Session
}

// newSyncSession wraps inner so all access serializes on a fresh mutex.
func newSyncSession(inner blockstore.Session) *syncSession {
	return &syncSession{mu: new(sync.Mutex), inner: inner}
}

func (s *syncSession) Capabilities() blockstore.Capabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Capabilities()
}

func (s *syncSession) Blocks(filter blockstore.BlockFilter) iter.Seq2[*blockstore.Block, error] {
	return func(yield func(*blockstore.Block, error) bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for b, err := range s.inner.Blocks(filter) {
			if !yield(b, err) {
				return
			}
		}
	}
}

func (s *syncSession) GetBlock(hash string) (*blockstore.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.GetBlock(hash)
}

func (s *syncSession) PutBlock(collection string, b *blockstore.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.PutBlock(collection, b)
}

func (s *syncSession) GetOverlay(kind, blockHash string) (blockstore.Overlay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.GetOverlay(kind, blockHash)
}

func (s *syncSession) PutOverlay(o blockstore.Overlay) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.PutOverlay(o)
}

func (s *syncSession) ListOverlays(kind string) iter.Seq2[blockstore.Overlay, error] {
	return func(yield func(blockstore.Overlay, error) bool) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for o, err := range s.inner.ListOverlays(kind) {
			if !yield(o, err) {
				return
			}
		}
	}
}

func (s *syncSession) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Commit()
}

func (s *syncSession) Rollback() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Rollback()
}

func (s *syncSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Close()
}

var _ blockstore.Session = (*syncSession)(nil)

// runTool dispatches a tool invocation through the right contract —
// SessionTool.SessionProcess when the tool opts in AND the store is
// persistent, otherwise the plain streaming Tool.Process.
//
// SessionProcess exists to cache per-block work (e.g. pseudo/AI/MT
// targets) as overlays so a later run against the SAME store can skip
// it. That payoff only materializes when the store survives the run.
// For the default ephemeral in-memory store (one-shot CLI invocations,
// tests), the overlay cache is discarded at process exit, so the
// per-block GetOverlay/PutOverlay round-trips and JSON (un)marshaling
// are pure overhead. Routing those through Tool.Process produces
// identical output with none of that bookkeeping (#608, S5).
func runTool(
	ctx context.Context,
	t tool.Tool,
	session blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	if st, ok := t.(tool.SessionTool); ok && session.Capabilities().Persistent {
		return st.SessionProcess(ctx, session, in, out)
	}
	return t.Process(ctx, in, out)
}

// resolveTools returns the tools to use for processing.
// For parallel execution (multiple documents), it creates fresh instances
// via ToolFactories. For single/sequential, it uses f.Tools directly.
func (e *DefaultExecutor) resolveTools(f *Flow) ([]tool.Tool, error) {
	if factories := f.ToolFactories; len(factories) > 0 {
		tools := make([]tool.Tool, len(factories))
		for i, factory := range factories {
			t, err := factory()
			if err != nil {
				return nil, fmt.Errorf("tool factory %d: %w", i, err)
			}
			tools[i] = t
		}
		return tools, nil
	}
	return f.Tools, nil
}

// feedCollectors passes output parts to all registered collectors.
func (e *DefaultExecutor) feedCollectors(ctx context.Context, item *Item, parts []*model.Part) error {
	for _, c := range e.config.Collectors {
		if err := c.Collect(ctx, item, parts); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteWithChannels sets up the tool chain and returns the input channel.
// The caller feeds Parts into the input channel and receives them from the output channel.
// The caller must close the input channel when done.
func (e *DefaultExecutor) ExecuteWithChannels(ctx context.Context, f *Flow) (in chan<- *model.Part, out <-chan *model.Part, wait func() error) {
	tools := f.Tools
	if len(tools) == 0 {
		ch := make(chan *model.Part, e.config.ChannelSize)
		return ch, ch, func() error { return nil }
	}

	ctx, cancel := context.WithCancel(ctx)

	// Open the session with the parent context, NOT the errgroup's derived
	// context: errgroup cancels its context the moment Wait() returns, and a
	// persistent session backed by a *sql.Tx (sqlitestore.New) is
	// aborted by database/sql when its context is cancelled — which would
	// roll back every overlay the SessionTools wrote, right before Commit.
	// The tool goroutines still observe cancellation through the errgroup
	// context they each receive.
	rawSession, err := e.config.Store.Begin(ctx)
	if err != nil {
		ch := make(chan *model.Part, e.config.ChannelSize)
		close(ch)
		cancel()
		return ch, ch, func() error { return fmt.Errorf("open blockstore session: %w", err) }
	}

	g, ctx := errgroup.WithContext(ctx)
	// All tools share this one session and run concurrently in their own
	// goroutines, so guard it against concurrent use (see syncSession).
	session := newSyncSession(rawSession)

	channels := make([]chan *model.Part, len(tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.config.ChannelSize)
	}

	for i, t := range tools {
		inCh := channels[i]
		outCh := channels[i+1]

		g.Go(func() error {
			defer close(outCh)
			if err := runTool(ctx, t, session, inCh, outCh); err != nil {
				return fmt.Errorf("tool %s: %w", t.Name(), err)
			}
			return nil
		})
	}

	return channels[0], channels[len(channels)-1], func() error {
		err := g.Wait()
		if err != nil {
			cancel()
			_ = session.Rollback()
			return err
		}
		// Commit BEFORE cancelling the pipeline context. A persistent
		// session is backed by a *sql.Tx opened with this context
		// (sqlitestore.New via WithBlockStore); database/sql aborts
		// that tx when its context is cancelled, so cancelling first would
		// roll back every overlay the SessionTools just wrote. The tool
		// goroutines have already finished (g.Wait returned), so deferring
		// the cancel costs nothing.
		cerr := session.Commit()
		cancel()
		if cerr != nil {
			return fmt.Errorf("commit session: %w", cerr)
		}
		return nil
	}
}
