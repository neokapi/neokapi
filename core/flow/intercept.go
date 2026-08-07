package flow

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// partHook observes one Part crossing an intercepted tool's boundary. Hooks run
// inline on the interceptor goroutines, so they must be fast.
type partHook func(*model.Part)

// interceptTool runs a single tool invocation with observation channels spliced
// around it: run receives the channels the inner tool reads from and writes to,
// onIn sees every Part entering the tool and onOut every Part leaving it. Either
// hook may be nil. It is the one implementation of the wrapper contract the
// tool.Tool wrappers in this package share (metrics, tracing, tapping).
//
// Channel ownership: the executor creates in/out and closes out once the wrapper
// returns, and a tool never closes its own output channel — so the inner output
// channel is closed here, after run returns, to terminate the output
// interceptor. out is left untouched.
//
// Neither interceptor outlives the call, whatever the inner tool does:
//
//   - The input interceptor stops forwarding on context cancellation or once run
//     returns (stop), since a tool that has returned no longer reads its input.
//   - The output interceptor keeps consuming the inner tool's output after
//     cancellation — discarding rather than forwarding — so an inner tool whose
//     sends are not guarded by ctx still completes its send and returns instead
//     of parking on a channel nobody reads. Wrappers accept arbitrary tools,
//     including plugin tools whose Process this package does not control.
//
// Both are joined before returning. Cancellation that stopped forwarding
// surfaces as the context error when the inner tool reports none itself.
func interceptTool(
	ctx context.Context,
	in <-chan *model.Part,
	out chan<- *model.Part,
	onIn, onOut partHook,
	run func(in <-chan *model.Part, out chan<- *model.Part) error,
) error {
	innerIn := make(chan *model.Part, cap(in))
	innerOut := make(chan *model.Part, cap(out))
	stop := make(chan struct{})

	// forwardErr is written by the output interceptor and read after the join.
	var forwardErr error

	var interceptors sync.WaitGroup
	interceptors.Add(2)

	go func() {
		defer interceptors.Done()
		defer close(innerIn)
		for part := range in {
			if onIn != nil {
				onIn(part)
			}
			select {
			case innerIn <- part:
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()

	go func() {
		defer interceptors.Done()
		forwarding := true
		for part := range innerOut {
			if onOut != nil {
				onOut(part)
			}
			if !forwarding {
				continue
			}
			select {
			case out <- part:
			case <-ctx.Done():
				forwarding = false
				forwardErr = ctx.Err()
			}
		}
	}()

	err := run(innerIn, innerOut)

	close(stop)
	close(innerOut)
	interceptors.Wait()

	if err != nil {
		return err
	}
	return forwardErr
}
