package tool

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// Tee copies each Part from the input channel to all output channels.
// All output channels are closed when the input channel is closed or the
// context is cancelled. If any output channel blocks, Tee blocks
// (backpressure propagates) until that send completes or ctx is cancelled.
func Tee(ctx context.Context, in <-chan *model.Part, outs ...chan<- *model.Part) {
	defer func() {
		for _, out := range outs {
			close(out)
		}
	}()
	for {
		var part *model.Part
		select {
		case <-ctx.Done():
			return
		case next, ok := <-in:
			if !ok {
				return
			}
			part = next
		}
		for _, out := range outs {
			select {
			case out <- part:
			case <-ctx.Done():
				return
			}
		}
	}
}
