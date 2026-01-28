package flow

import (
	"context"
	"fmt"

	"github.com/asgeirf/gokapi/core/model"
	"golang.org/x/sync/errgroup"
)

// FlowExecutor orchestrates the execution of a Flow across batch items.
type FlowExecutor interface {
	// Execute runs the Flow over the given batch items.
	Execute(ctx context.Context, f *Flow, items []*FlowItem) error
}

// DefaultFlowExecutor runs tools concurrently using goroutines and channels.
type DefaultFlowExecutor struct {
	channelSize int
}

// NewFlowExecutor creates a new DefaultFlowExecutor.
func NewFlowExecutor() *DefaultFlowExecutor {
	return &DefaultFlowExecutor{
		channelSize: 64,
	}
}

// SetChannelSize configures the buffer size for inter-tool channels.
func (e *DefaultFlowExecutor) SetChannelSize(size int) {
	if size > 0 {
		e.channelSize = size
	}
}

// Execute launches one goroutine per tool, connected by buffered channels.
//
//	[input chan] --> [Tool1] --> [Tool2] --> ... --> [ToolN] --> [output chan]
//	  goroutine     goroutine   goroutine          goroutine
//
// Errors from any stage cancel all others via context.
func (e *DefaultFlowExecutor) Execute(ctx context.Context, f *Flow, items []*FlowItem) error {
	for _, item := range items {
		if err := e.processItem(ctx, f, item); err != nil {
			return fmt.Errorf("processing %s: %w", item.Input.URI, err)
		}
	}
	return nil
}

// processItem processes a single FlowItem through the tool chain.
func (e *DefaultFlowExecutor) processItem(ctx context.Context, f *Flow, item *FlowItem) error {
	if len(f.Tools) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	// Create channels: one input, one between each pair of tools, one output
	channels := make([]chan *model.Part, len(f.Tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.channelSize)
	}

	// Launch a goroutine for each tool
	for i, t := range f.Tools {
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

	return g.Wait()
}

// InputChannel returns the input channel (first channel) after starting
// Execute in a goroutine. This is useful for feeding Parts directly.
// Use ExecuteWithChannels for this pattern.

// ExecuteWithChannels sets up the tool chain and returns the input channel.
// The caller feeds Parts into the input channel and receives them from the output channel.
// The caller must close the input channel when done.
func (e *DefaultFlowExecutor) ExecuteWithChannels(ctx context.Context, f *Flow) (in chan<- *model.Part, out <-chan *model.Part, wait func() error) {
	if len(f.Tools) == 0 {
		ch := make(chan *model.Part, e.channelSize)
		return ch, ch, func() error { return nil }
	}

	ctx, cancel := context.WithCancel(ctx)
	g, ctx := errgroup.WithContext(ctx)

	channels := make([]chan *model.Part, len(f.Tools)+1)
	for i := range channels {
		channels[i] = make(chan *model.Part, e.channelSize)
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
