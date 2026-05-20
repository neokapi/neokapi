package flow

import (
	"context"
	"fmt"
	"runtime"

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
// (e.g. NewCacheStore for a project's .kapi/cache/blocks.db) to enable
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
	session, err := e.config.Store.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("open blockstore session: %w", err)
	}

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

	// Collect output parts from the final channel.
	outCh := channels[len(channels)-1]
	var parts []*model.Part
	var collectErr error
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
	if collectErr != nil {
		_ = session.Rollback()
		return nil, fmt.Errorf("collect output: %w", collectErr)
	}
	if err := session.Commit(); err != nil {
		return nil, fmt.Errorf("commit session: %w", err)
	}
	return parts, nil
}

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
	if len(f.ToolFactories) > 0 {
		tools := make([]tool.Tool, len(f.ToolFactories))
		for i, factory := range f.ToolFactories {
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
	if len(f.Tools) == 0 {
		ch := make(chan *model.Part, e.config.ChannelSize)
		return ch, ch, func() error { return nil }
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	session, err := e.config.Store.Begin(ctx)
	if err != nil {
		ch := make(chan *model.Part, e.config.ChannelSize)
		close(ch)
		cancel()
		return ch, ch, func() error { return fmt.Errorf("open blockstore session: %w", err) }
	}

	channels := make([]chan *model.Part, len(f.Tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.config.ChannelSize)
	}

	for i, t := range f.Tools {
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
		cancel()
		if err != nil {
			_ = session.Rollback()
			return err
		}
		if cerr := session.Commit(); cerr != nil {
			return fmt.Errorf("commit session: %w", cerr)
		}
		return nil
	}
}
