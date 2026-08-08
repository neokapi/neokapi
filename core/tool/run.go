package tool

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

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
