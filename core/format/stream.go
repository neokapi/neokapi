package format

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/neokapi/neokapi/core/model"
)

// partStreamBuffer is the send buffer every reader's Part channel uses. Sized
// so a reader stays ahead of a tool pipeline without holding a whole document.
const partStreamBuffer = 64

// StreamParts runs read on its own goroutine and returns the channel it fills,
// closed when read returns. A reader's Read method is the one place in a format
// where work happens on a goroutine the caller does not own, so it is also the
// one place where a panic has nowhere to surface: the flow runner recovers on
// its feeder goroutine, and this one is not it. An emphasis inside an excluded
// inline HTML span took the whole process down that way (#2444).
//
// A panic therefore arrives on the channel as a PartResult error, the same way
// a returned error does, and the caller's range loop ends normally. The stack
// travels with it, because a recovered panic is a reader bug and the report has
// to name the line.
func StreamParts(ctx context.Context, read func(context.Context, chan<- model.PartResult) error) <-chan model.PartResult {
	ch := make(chan model.PartResult, partStreamBuffer)
	go func() {
		defer close(ch)
		defer func() {
			if rec := recover(); rec != nil {
				sendResult(ctx, ch, model.PartResult{
					Error: fmt.Errorf("reader panicked: %v\n%s", rec, debug.Stack()),
				})
			}
		}()
		if err := read(ctx, ch); err != nil {
			sendResult(ctx, ch, model.PartResult{Error: err})
		}
	}()
	return ch
}

// sendResult delivers one result unless the context is already done, so a
// caller that walked away from the channel cannot strand the goroutine.
func sendResult(ctx context.Context, ch chan<- model.PartResult, res model.PartResult) {
	select {
	case ch <- res:
	case <-ctx.Done():
	}
}
