package tool

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// ReadAll receives Parts from in until the channel closes and returns them in
// the order received. It returns ctx's error when ctx ends first, so a tool
// that buffers its whole input before working on it still stops on
// cancellation while upstream is idle.
func ReadAll(ctx context.Context, in <-chan *model.Part) ([]*model.Part, error) {
	var parts []*model.Part
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case part, ok := <-in:
			if !ok {
				return parts, nil
			}
			parts = append(parts, part)
		}
	}
}

// RunOnParts drives a tool synchronously over an in-memory slice of parts and
// collects the results. It is the buffered, single-shot counterpart to
// Tool.Process for callers that already hold every part in memory (editor
// actions, job workers) rather than a stream.
//
// Process runs in its own goroutine while the caller drains the output channel
// concurrently, so a fan-out tool that emits more parts than it consumes cannot
// deadlock on the bounded buffer.
func RunOnParts(ctx context.Context, t Tool, parts []*model.Part) ([]*model.Part, error) {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	errCh := make(chan error, 1)
	go func() {
		err := t.Process(ctx, in, out)
		close(out)
		errCh <- err
	}()

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
	}
	if err := <-errCh; err != nil {
		return nil, err
	}
	return result, nil
}
