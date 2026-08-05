package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
)

// ErrWriteGateReentrant marks the one misuse of the write gate that cannot be
// waited out: a goroutine already holding this handle's write transaction
// asking for the permit again. The gate is deliberately NOT reentrant — a
// transaction holds it for its whole life, and SQLite would in any case refuse
// a second write transaction on the same file — so the second acquisition can
// only ever be a deadlock. It is detected and reported instead of hung.
//
// The fix at the call site is always the same shape: finish the transaction
// (Commit or Rollback) before writing again on the same handle, or do the
// second write inside the transaction you already hold.
var ErrWriteGateReentrant = errors.New("storage: the write gate is not reentrant")

// writeGate serializes write transactions on one database handle, in arrival
// order, within one process.
//
// Why it exists: several subsystems share the project's single SQLite file, so
// they queue behind one write lock. SQLite's own queue is `busy_timeout`, whose
// backoff has no memory of who has been waiting longest — a writer that wants
// the lock for two milliseconds loses to a writer that takes it for two seconds,
// over and over, until it exhausts the timeout and fails with SQLITE_BUSY.
// Measured at dogfood scale, a drip of small unit-state writes completed 32 of
// 2650 attempts against a saturating content-memory writer. A FIFO permit takes
// that to zero, because the queue is Go's, not SQLite's: a buffered channel's
// sender wait queue is served in the order senders blocked.
//
// What it cannot do: reach another process. Two `kapi` processes on the same
// file still contend at the file level, softened by BEGIN IMMEDIATE (which puts
// the wait somewhere `busy_timeout` applies) but not ordered. In-process
// starvation is the failure this removes; cross-process contention is the
// residue it leaves.
type writeGate struct {
	// permits has capacity 1. Acquiring sends, releasing receives. The
	// direction matters: a blocked *sender* queue is what Go serves FIFO, and
	// a receive by the releasing holder hands the slot to the longest-waiting
	// sender rather than leaving it open for a newcomer to barge into.
	permits chan struct{}

	// holder is the goroutine id holding a transaction-scoped permit, or 0 when
	// the permit is free or held only for the duration of a single statement.
	// A statement-scoped holder need not be recorded: the goroutine running one
	// Exec cannot be the goroutine blocked on the next one.
	holder atomic.Int64
}

func newWriteGate() *writeGate { return &writeGate{permits: make(chan struct{}, 1)} }

// acquire takes the permit, blocking in arrival order until it is free or ctx
// is done.
//
// held distinguishes the two lifetimes. A statement-scoped acquisition (false)
// is released before ExecContext returns. A transaction-scoped one (true) is
// released by Commit or Rollback, arbitrarily far away and with caller code in
// between — which is the only case that can deadlock against itself, and so the
// only one whose owner is recorded.
func (g *writeGate) acquire(ctx context.Context, held bool) error {
	if g == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Uncontended: take it without paying for the goroutine-id lookup. This
	// does not let a newcomer jump the queue — a buffered channel with senders
	// already blocked on it has no free slot for a non-blocking send.
	select {
	case g.permits <- struct{}{}:
		g.claim(held)
		return nil
	default:
	}
	if owner := g.holder.Load(); owner != 0 && owner == goID() {
		return fmt.Errorf("%w: goroutine %d already holds a write transaction on this database "+
			"and would wait on itself forever", ErrWriteGateReentrant, owner)
	}
	select {
	case g.permits <- struct{}{}:
		g.claim(held)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *writeGate) claim(held bool) {
	if held {
		g.holder.Store(goID())
		return
	}
	g.holder.Store(0)
}

// release returns the permit. It is safe on a nil gate, so every caller can be
// written once for both gated and ungated handles.
func (g *writeGate) release() {
	if g == nil {
		return
	}
	g.holder.Store(0)
	<-g.permits
}

// goIDBufs keeps the scratch space for goID off the allocator's path. Sixty-four
// bytes is comfortably more than the "goroutine N [running]:" header goID reads;
// runtime.Stack truncates the rest, which is exactly what is wanted.
var goIDBufs = sync.Pool{New: func() any { b := make([]byte, 64); return &b }}

// goID returns the calling goroutine's id, read from the header the runtime
// writes at the top of every stack dump.
//
// The runtime exposes no supported accessor, and this is the only way to tell
// "the goroutine waiting for the permit is the one holding it" — the difference
// between reporting a deadlock and hanging in it. It is read only on the
// contended path and when a transaction claims the permit, never on the
// uncontended statement path. A parse failure returns 0, which the caller treats
// as "unknown owner" and never as a match, so a runtime that changed the header
// format would cost the diagnostic, not correctness.
func goID() int64 {
	bp := goIDBufs.Get().(*[]byte)
	defer goIDBufs.Put(bp)
	n := runtime.Stack(*bp, false)
	line, ok := bytes.CutPrefix((*bp)[:n], []byte("goroutine "))
	if !ok {
		return 0
	}
	end := bytes.IndexByte(line, ' ')
	if end < 0 {
		return 0
	}
	id, err := strconv.ParseInt(string(line[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
