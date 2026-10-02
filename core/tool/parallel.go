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
// N goroutines while preserving Part ordering. The inner tool's non-Block
// handlers run on the dispatcher goroutine, one at a time. The fan-out applies
// when the inner tool is a plain *BaseTool with a typed block handler, where
// each block is independent work such as a subprocess or a remote lookup. Any
// other tool, including a type that embeds BaseTool, runs through its own
// Process or SessionProcess.
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

// nonBlockWindowPerBlock sizes the window for admitted non-block Parts as a
// multiple of the block window. Readers put a few structural Parts between
// consecutive blocks (PO a Data Part for an entry's comments or references,
// CSV a group start and end around each row), and each of them waits in the
// ring until the blocks before it are emitted. A window of this many per block keeps every worker busy across
// such a stream while still bounding what the stage holds.
const nonBlockWindowPerBlock = 4

// sequencedPart pairs a Part with a monotonic sequence number for ordering.
// block records which window the Part was admitted on, since a dropped block
// has no Part left to inspect.
type sequencedPart struct {
	seq   uint64
	part  *model.Part
	err   error
	block bool
}

// orderedPart is one slot in the bounded reassembly ring. A nil part with
// ready set represents a dropped part and still advances the sequence.
type orderedPart struct {
	part  *model.Part
	ready bool
	block bool
}

// Process runs independent block handlers on a fixed worker pool. At most
// concurrency blocks are admitted and not yet emitted, including completed
// results waiting behind a slower block, so downstream backpressure reaches
// the input even when workers finish out of order. Non-block Parts have their
// own window of nonBlockWindowPerBlock times that size, so structural Parts
// between blocks do not take the blocks' places.
//
// A non-block handler the inner tool sets runs after every earlier block
// handler has finished and before any later one starts, so layer and group
// state can be updated safely. A non-block Part with no handler passes through
// in order without that wait. Block handlers must be safe to call concurrently
// for distinct blocks and must honor their view's context. Process joins every
// worker before returning, including on error.
func (p *ParallelBlockTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	baseTool, isBase := p.inner.(*BaseTool)
	if p.concurrency <= 1 || !isBase || !baseTool.hasBlockHandler() {
		return p.inner.Process(ctx, in, out)
	}
	if err := baseTool.ValidateHandlers(); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	blockWindow := make(chan struct{}, p.concurrency)
	otherWindow := make(chan struct{}, nonBlockWindowPerBlock*p.concurrency)
	// Every admitted Part holds a permit in one of the two windows until it
	// is emitted, so the ring never holds more than both capacities together.
	ringSize := cap(blockWindow) + cap(otherWindow)
	jobs := make(chan sequencedPart)
	results := make(chan sequencedPart, ringSize)
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
			// Admit every Part on a block permit, acquired before reading, so
			// a run of blocks reads at most concurrency ahead. A non-block Part
			// trades it for a non-block permit once its type is known. Each
			// permit is kept until ordered emission, not released when a
			// worker finishes.
			select {
			case blockWindow <- struct{}{}:
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
				case jobs <- sequencedPart{seq: seq, part: part, block: true}:
				case <-ctx.Done():
					active.Done()
					return
				}
				continue
			}

			select {
			case otherWindow <- struct{}{}:
			case <-ctx.Done():
				return
			}
			<-blockWindow

			// A Part the inner tool does not handle changes no state, so it
			// skips the barrier and waits for its turn in the ring.
			if !baseTool.handlesPart(part.Type) {
				select {
				case results <- sequencedPart{seq: seq, part: part}:
				case <-ctx.Done():
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

	// The admitted Parts are a contiguous run of sequence numbers no longer
	// than the ring, so modulo indexing cannot overwrite another pending
	// result. Clear emitted slots promptly to avoid retaining their blocks
	// while upstream is idle.
	pending := make([]orderedPart, ringSize)
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
			pending[result.seq%uint64(len(pending))] = orderedPart{part: result.part, ready: true, block: result.block}
			for pending[nextSeq%uint64(len(pending))].ready {
				slot := &pending[nextSeq%uint64(len(pending))]
				if slot.part != nil {
					select {
					case out <- slot.part:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				if slot.block {
					<-blockWindow
				} else {
					<-otherWindow
				}
				*slot = orderedPart{}
				nextSeq++
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
// with nothing logged. The fan-out in Process applies to a *BaseTool inner,
// which is not a SessionTool, so no parallelism is given up here.
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
