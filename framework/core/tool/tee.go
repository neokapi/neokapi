package tool

import "github.com/neokapi/neokapi/core/model"

// Tee copies each Part from the input channel to all output channels.
// All output channels are closed when the input channel is closed.
// If any output channel blocks, Tee blocks (backpressure propagates).
func Tee(in <-chan *model.Part, outs ...chan<- *model.Part) {
	defer func() {
		for _, out := range outs {
			close(out)
		}
	}()
	for part := range in {
		for _, out := range outs {
			out <- part
		}
	}
}
