package tool

import (
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

// orderedPart is one slot in the bounded reassembly window. A nil part with
// ready set represents a dropped part and still advances the sequence.
type orderedPart struct {
	part  *model.Part
	ready bool
}

// Process runs independent block handlers on a fixed worker pool. The number
// of parts admitted but not yet emitted is bounded by concurrency, including
// completed results waiting behind a slower block. Downstream backpressure
// therefore reaches the input even when workers finish out of order.
//
// Non-block handlers run between groups of completed block handlers, so layer
// and group state can be updated safely. Block handlers must be safe to call
// concurrently for distinct blocks and must honor their view's context.
// Process joins every worker before returning, including on error.
func (p *ParallelBlockTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	baseTool, isBase := p.inner.(*BaseTool)
	if p.concurrency <= 1 || !isBase || !baseTool.hasBlockHandler() {
		return p.inner.Process(ctx, in, out)
	}
	if err := baseTool.ValidateHandlers(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	jobs := make(chan sequencedPart)
	results := make(chan sequencedPart, p.concurrency)
	window := make(chan struct{}, p.concurrency)
	done := make(chan struct{})
	defer func() {
		cancel()
		<-done
	}()

	var workers sync.WaitGroup
	var active sync.WaitGroup
	for range p.concurrency {
		workers.Go(func() {
			for job := range jobs {
				if ctx.Err() == nil {
					job.part, job.err = recoverHandle(p.inner.Name(), func() (*model.Part, error) {
						return baseTool.handleBlock(ctx, job.part)
					})
					select {
					case results <- job:
					case <-ctx.Done():
					}
				}
				active.Done()
			}
		})
	}

	go func() {
		defer func() {
			close(jobs)
			workers.Wait()
			close(results)
			close(done)
		}()

		for seq := uint64(0); ; seq++ {
			// Keep this permit until ordered emission, rather than releasing it
			// when a worker finishes. Acquire before reading to bound lookahead.
			select {
			case window <- struct{}{}:
			case <-ctx.Done():
				return
			}
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

			if part.Type == model.PartBlock {
				active.Add(1)
				select {
				case jobs <- sequencedPart{seq: seq, part: part}:
				case <-ctx.Done():
					active.Done()
					return
				}
				continue
			}

			// Structural handlers may change state read by block handlers.
			// Finish the preceding group before invoking the next handler.
			active.Wait()
			if ctx.Err() != nil {
				return
			}
			result, err := recoverHandle(p.inner.Name(), func() (*model.Part, error) {
				return baseTool.dispatch(ctx, part)
			})
			select {
			case results <- sequencedPart{seq: seq, part: result, err: err}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// An admitted part owns its slot until emission, so modulo indexing
	// cannot overwrite another pending result. Clear emitted slots promptly
	// to avoid retaining their blocks while upstream is idle.
	pending := make([]orderedPart, p.concurrency)
	var nextSeq uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result, ok := <-results:
			if !ok {
				return ctx.Err()
			}
			if result.err != nil {
				return result.err
			}
			pending[result.seq%uint64(len(pending))] = orderedPart{part: result.part, ready: true}
			for pending[nextSeq%uint64(len(pending))].ready {
				slot := &pending[nextSeq%uint64(len(pending))]
				if slot.part != nil {
					select {
					case out <- slot.part:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				*slot = orderedPart{}
				nextSeq++
				<-window
			}
		}
	}
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
