package tool

import (
	"container/heap"
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
)

// ParallelBlockTool wraps an inner tool and fans out Block processing across
// N goroutines while preserving Part ordering. Non-Block Parts pass through
// the inner tool sequentially. This is useful for IO-bound tools (AI translate,
// MT) where each block is an independent API call.
type ParallelBlockTool struct {
	inner       Tool
	concurrency int
}

// NewParallelBlockTool creates a ParallelBlockTool that processes blocks in
// parallel using the inner tool's block handler. concurrency controls the
// maximum number of blocks processed simultaneously.
func NewParallelBlockTool(inner Tool, concurrency int) *ParallelBlockTool {
	if concurrency < 1 {
		concurrency = 1
	}
	return &ParallelBlockTool{
		inner:       inner,
		concurrency: concurrency,
	}
}

// Name returns the wrapped tool's name.
func (p *ParallelBlockTool) Name() string { return p.inner.Name() }

// Description returns the wrapped tool's description.
func (p *ParallelBlockTool) Description() string { return p.inner.Description() }

// Config returns the wrapped tool's configuration.
func (p *ParallelBlockTool) Config() ToolConfig { return p.inner.Config() }

// SetConfig applies configuration to the wrapped tool.
func (p *ParallelBlockTool) SetConfig(c ToolConfig) error { return p.inner.SetConfig(c) }

// recoverHandle invokes fn, converting a panic into an error. The dispatcher
// and its workers run tool handlers on their own goroutines, where an
// unrecovered panic would crash the whole process instead of failing the
// tool; this mirrors the executor's per-tool recovery (flow.runTool).
func recoverHandle(name string, fn func() (*model.Part, error)) (result *model.Part, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %s panicked: %v\n%s", name, r, debug.Stack())
		}
	}()
	return fn()
}

// sequencedPart pairs a Part with a monotonic sequence number for ordering.
type sequencedPart struct {
	seq  uint64
	part *model.Part
	err  error
}

// orderedBuffer is a min-heap that emits parts in sequence order.
type orderedBuffer []sequencedPart

func (h orderedBuffer) Len() int           { return len(h) }
func (h orderedBuffer) Less(i, j int) bool { return h[i].seq < h[j].seq }
func (h orderedBuffer) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *orderedBuffer) Push(x any)        { *h = append(*h, x.(sequencedPart)) }
func (h *orderedBuffer) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Process fans out Block parts to worker goroutines while preserving order.
// Non-Block parts are processed by the inner tool's Process method directly.
//
// The algorithm:
//  1. Assign monotonic sequence numbers to all incoming Parts.
//  2. Block Parts go to a worker pool; non-Block Parts go directly to a results channel.
//  3. A reassembly goroutine collects results and emits them in sequence order
//     using a min-heap buffer.
func (p *ParallelBlockTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// If concurrency is 1 or the inner tool isn't a BaseTool with a per-block
	// handler, fall back to the inner tool's sequential processing.
	baseTool, isBase := p.inner.(*BaseTool)
	if p.concurrency <= 1 || !isBase || !baseTool.hasBlockHandler() {
		return p.inner.Process(ctx, in, out)
	}
	// The fan-out path routes blocks through handleBlock directly (bypassing
	// the inner tool's Process), so enforce the exactly-one-handler contract
	// here the same way Process does.
	if err := baseTool.ValidateHandlers(); err != nil {
		return err
	}

	// Own a cancellable child context so that any early return (worker error
	// or downstream cancel) unblocks the dispatcher and in-flight workers
	// parked on the results channel, instead of leaking them until some
	// external cancel arrives.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// results channel carries sequenced parts from workers and passthrough.
	results := make(chan sequencedPart, p.concurrency*2)

	// Worker semaphore for block processing.
	sem := make(chan struct{}, p.concurrency)

	var wg sync.WaitGroup
	var seq uint64
	var dispatchErr error

	// Dispatcher goroutine: reads input, dispatches blocks to workers.
	go func() {
		defer func() {
			// Wait for all workers to complete before closing results.
			wg.Wait()
			close(results)
		}()

		for {
			select {
			case <-ctx.Done():
				dispatchErr = ctx.Err()
				return
			case part, ok := <-in:
				if !ok {
					return
				}
				currentSeq := seq
				seq++

				if part.Type == model.PartBlock {
					// Acquire semaphore slot (backpressure).
					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						dispatchErr = ctx.Err()
						return
					}

					wg.Go(func() {
						defer func() { <-sem }() // release slot
						// Route through the dispatcher so the tool's typed
						// handler (and the immutability backstop) applies; each
						// worker handles a distinct block, so no shared state.
						result, err := recoverHandle(p.inner.Name(), func() (*model.Part, error) {
							return baseTool.handleBlock(ctx, part)
						})
						select {
						case results <- sequencedPart{seq: currentSeq, part: result, err: err}:
						case <-ctx.Done():
						}
					})
				} else {
					// Non-Block: dispatch to inner tool's handler, then send result.
					result, err := recoverHandle(p.inner.Name(), func() (*model.Part, error) {
						return baseTool.dispatch(ctx, part)
					})
					select {
					case results <- sequencedPart{seq: currentSeq, part: result, err: err}:
					case <-ctx.Done():
						dispatchErr = ctx.Err()
						return
					}
				}
			}
		}
	}()

	// Reassembly: collect results and emit in order.
	var buf orderedBuffer
	heap.Init(&buf)
	var nextSeq uint64

	for sp := range results {
		if sp.err != nil {
			return sp.err
		}

		heap.Push(&buf, sp)

		// Emit all consecutive parts starting from nextSeq.
		for buf.Len() > 0 && buf[0].seq == nextSeq {
			item, ok := heap.Pop(&buf).(sequencedPart)
			if !ok {
				continue
			}
			select {
			case out <- item.part:
			case <-ctx.Done():
				return ctx.Err()
			}
			nextSeq++
		}
	}

	// Drain any remaining items (shouldn't happen if everything is correct).
	for buf.Len() > 0 {
		item, ok := heap.Pop(&buf).(sequencedPart)
		if !ok {
			continue
		}
		if item.err != nil {
			return item.err
		}
		select {
		case out <- item.part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return dispatchErr
}

// SessionProcess hands the session to the inner tool.
//
// The wrapper has to declare the method for the executor's type assertion to
// see a SessionTool at all: a wrapper that only implements Tool silently
// downgrades whatever it wraps to the sessionless path, and a tool whose
// persistent overlay cache lives in the session then rebuilds it on every run
// with nothing logged. The fan-out below applies to a *BaseTool inner, which is
// not a SessionTool, so no parallelism is given up here.
func (p *ParallelBlockTool) SessionProcess(ctx context.Context, sess blockstore.Session, in <-chan *model.Part, out chan<- *model.Part) error {
	st, ok := p.inner.(SessionTool)
	if !ok {
		return p.Process(ctx, in, out)
	}
	return st.SessionProcess(ctx, sess, in, out)
}

// Verify ParallelBlockTool implements Tool and SessionTool at compile time.
var (
	_ Tool        = (*ParallelBlockTool)(nil)
	_ SessionTool = (*ParallelBlockTool)(nil)
)
