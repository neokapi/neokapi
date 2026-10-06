package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
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
// that to zero, because the queue is Go's, not SQLite's.
//
// The queue is an explicit list of waiters under a mutex, and a release hands
// the permit straight to the waiter at its head. Joining the queue, reading how
// many releases have happened so far, and the handoff that ends a wait each
// happen inside the mutex, so the order a writer is served in and the number of
// releases it is counted as waiting through describe the same queue. A channel
// would serve senders in order too, but a writer cannot read a counter in the
// same step as it blocks on a send, and a writer descheduled between the two
// would be charged for releases it never queued behind.
//
// What it cannot do: reach another process. Two `kapi` processes on the same
// file still contend at the file level, softened by BEGIN IMMEDIATE (which puts
// the wait somewhere `busy_timeout` applies) but not ordered. In-process
// starvation is the failure this removes; cross-process contention is the
// residue it leaves.
type writeGate struct {
	mu sync.Mutex

	// busy reports that some writer holds the permit. While it is set, a newcomer
	// joins the back of waiters; a release with writers waiting passes the
	// permit to the first of them and leaves busy set, so no newcomer can take
	// it in between.
	busy    bool
	waiters []*gateWaiter

	// grants counts the permits granted, and releases the permits handed back.
	// mostWaited is the largest number of releases one acquisition waited
	// through, from joining the queue to its grant or to the moment it gave up:
	// one per writer served before it, the holder at its arrival included. See
	// WriteGateStats. All three are guarded by mu.
	grants     uint64
	releases   uint64
	mostWaited uint64

	// holder is the goroutine id holding a transaction-scoped permit, or 0 when
	// the permit is free or held only for the duration of a single statement.
	// A statement-scoped holder need not be recorded: the goroutine running one
	// Exec cannot be the goroutine blocked on the next one. It is read outside
	// mu, on the way into the queue.
	holder atomic.Int64
}

// gateWaiter is one writer queued for the permit. granted is closed when the
// permit is handed to it; arrived is the release count when it joined.
type gateWaiter struct {
	granted chan struct{}
	arrived uint64
}

func newWriteGate() *writeGate { return &writeGate{} }

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
	// Uncontended: take it without paying for the goroutine-id lookup.
	if g.tryAcquire() {
		g.claim(held)
		return nil
	}
	if owner := g.holder.Load(); owner != 0 && owner == goID() {
		return fmt.Errorf("%w: goroutine %d already holds a write transaction on this database "+
			"and would wait on itself forever", ErrWriteGateReentrant, owner)
	}

	g.mu.Lock()
	if !g.busy {
		// Released while the owner was being checked.
		g.busy = true
		g.grants++
		g.mu.Unlock()
		g.claim(held)
		return nil
	}
	w := &gateWaiter{granted: make(chan struct{}), arrived: g.releases}
	g.waiters = append(g.waiters, w)
	g.mu.Unlock()

	select {
	case <-w.granted:
		g.claim(held)
		return nil
	case <-ctx.Done():
	}

	g.mu.Lock()
	if i := slices.Index(g.waiters, w); i >= 0 {
		// Still queued: leave the queue and count the wait to here.
		g.waiters = slices.Delete(g.waiters, i, i+1)
		g.noteWaited(w)
		g.mu.Unlock()
		return ctx.Err()
	}
	g.mu.Unlock()
	// The permit was handed over as the context ended. It is ours, and the
	// writer behind us is waiting for it.
	g.release()
	return ctx.Err()
}

// tryAcquire takes the permit if it is free, which it never is while a writer
// is queued: a release with waiters hands the permit on without freeing it.
func (g *writeGate) tryAcquire() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy {
		return false
	}
	g.busy = true
	g.grants++
	return true
}

// noteWaited records how many releases w waited through. The caller holds mu.
func (g *writeGate) noteWaited(w *gateWaiter) {
	g.mostWaited = max(g.mostWaited, g.releases-w.arrived)
}

func (g *writeGate) claim(held bool) {
	if held {
		g.holder.Store(goID())
		return
	}
	g.holder.Store(0)
}

// release returns the permit, handing it to the longest-waiting writer if there
// is one. It is safe on a nil gate, so every caller can be written once for
// both gated and ungated handles.
func (g *writeGate) release() {
	if g == nil {
		return
	}
	g.holder.Store(0)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releases++
	if len(g.waiters) == 0 {
		g.busy = false
		return
	}
	w := g.waiters[0]
	g.waiters[0] = nil
	g.waiters = g.waiters[1:]
	g.grants++
	g.noteWaited(w)
	close(w.granted)
}

// stats reads the counters WriteGateStats reports.
func (g *writeGate) stats() (grants, mostWaited uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.grants, g.mostWaited
}

// queued reports how many writers are waiting for the permit.
func (g *writeGate) queued() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.waiters)
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
	before, _, ok0 := bytes.Cut(line, []byte{' '})
	if !ok0 {
		return 0
	}
	id, err := strconv.ParseInt(string(before), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
