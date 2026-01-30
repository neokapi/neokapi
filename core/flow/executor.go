package flow

import (
	"context"
	"fmt"
	"runtime"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
	"golang.org/x/sync/errgroup"
)

// FlowExecutor orchestrates the execution of a Flow across batch items.
type FlowExecutor interface {
	// Execute runs the Flow over the given batch items.
	Execute(ctx context.Context, f *Flow, items []*FlowItem) error
}

// ExecutorConfig holds configuration for the DefaultFlowExecutor.
type ExecutorConfig struct {
	MaxConcurrency int         // 0 = runtime.NumCPU(); 1 = sequential
	ChannelSize    int         // default 64
	FailFast       bool        // default true; cancel remaining on first error
	Collectors     []Collector // aggregators fed after each document completes
}

// ExecutorOption is a functional option for configuring a DefaultFlowExecutor.
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

// DefaultFlowExecutor runs tools concurrently using goroutines and channels.
type DefaultFlowExecutor struct {
	config ExecutorConfig
}

// NewFlowExecutor creates a new DefaultFlowExecutor with the given options.
// With no options, it behaves sequentially with channel size 64 and fail-fast enabled.
func NewFlowExecutor(opts ...ExecutorOption) *DefaultFlowExecutor {
	cfg := ExecutorConfig{
		MaxConcurrency: 1,
		ChannelSize:    64,
		FailFast:       true,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &DefaultFlowExecutor{config: cfg}
}

// SetChannelSize configures the buffer size for inter-tool channels.
// Kept for backward compatibility.
func (e *DefaultFlowExecutor) SetChannelSize(size int) {
	if size > 0 {
		e.config.ChannelSize = size
	}
}

// Execute processes FlowItems through the tool chain.
// When MaxConcurrency > 1 (or 0 for NumCPU), items are processed
// in parallel using a semaphore-bounded fan-out pattern.
func (e *DefaultFlowExecutor) Execute(ctx context.Context, f *Flow, items []*FlowItem) error {
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
		item := item
		if e.config.FailFast {
			// Check context before acquiring semaphore to fail fast.
			select {
			case <-ctx.Done():
				break
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

// processItemCollect processes a single FlowItem through the tool chain
// and returns the collected output parts.
func (e *DefaultFlowExecutor) processItemCollect(ctx context.Context, f *Flow, item *FlowItem) ([]*model.Part, error) {
	tools, err := e.resolveTools(f)
	if err != nil {
		return nil, err
	}

	if len(tools) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	// Create channels: one input, one between each pair of tools, one output
	channels := make([]chan *model.Part, len(tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.config.ChannelSize)
	}

	// Launch a goroutine for each tool
	for i, t := range tools {
		i, t := i, t
		in := channels[i]
		out := channels[i+1]

		g.Go(func() error {
			defer close(out)
			if err := t.Process(ctx, in, out); err != nil {
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
		return nil, err
	}
	if collectErr != nil {
		return nil, collectErr
	}
	return parts, nil
}

// resolveTools returns the tools to use for processing.
// For parallel execution (multiple documents), it creates fresh instances
// via ToolFactories. For single/sequential, it uses f.Tools directly.
func (e *DefaultFlowExecutor) resolveTools(f *Flow) ([]tool.Tool, error) {
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
func (e *DefaultFlowExecutor) feedCollectors(ctx context.Context, item *FlowItem, parts []*model.Part) error {
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
func (e *DefaultFlowExecutor) ExecuteWithChannels(ctx context.Context, f *Flow) (in chan<- *model.Part, out <-chan *model.Part, wait func() error) {
	if len(f.Tools) == 0 {
		ch := make(chan *model.Part, e.config.ChannelSize)
		return ch, ch, func() error { return nil }
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	channels := make([]chan *model.Part, len(f.Tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.config.ChannelSize)
	}

	for i, t := range f.Tools {
		i, t := i, t
		inCh := channels[i]
		outCh := channels[i+1]

		g.Go(func() error {
			defer close(outCh)
			if err := t.Process(ctx, inCh, outCh); err != nil {
				return fmt.Errorf("tool %s: %w", t.Name(), err)
			}
			return nil
		})
	}

	return channels[0], channels[len(channels)-1], func() error {
		err := g.Wait()
		cancel()
		return err
	}
}
