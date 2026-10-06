package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitQueued blocks until n writers are queued on g. A writer that has joined
// the queue has an arrival order the gate will honour, so the tests below
// sequence their writers on this instead of on a sleep.
func waitQueued(t *testing.T, g *writeGate, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return g.queued() == n }, 10*time.Second, time.Millisecond,
		"expected %d writers queued for the permit", n)
}

// TestWriteGate_ServesInArrivalOrder is the property the gate exists for. A
// mutex would not have it: Go's sync.Mutex hands a contended lock to whichever
// goroutine the scheduler happens to wake, and only rescues a waiter after a
// millisecond of starvation. SQLite's busy handler does not have it either,
// which is what let a stream of large writes shut a drip of small ones out
// entirely.
func TestWriteGate_ServesInArrivalOrder(t *testing.T) {
	g := newWriteGate()
	ctx := t.Context()
	require.NoError(t, g.acquire(ctx, true)) // the holder everyone queues behind

	const waiters = 6
	var (
		mu     sync.Mutex
		served []int
		wg     sync.WaitGroup
	)
	for i := range waiters {
		wg.Go(func() {
			if err := g.acquire(context.Background(), false); err != nil {
				return
			}
			mu.Lock()
			served = append(served, i)
			mu.Unlock()
			g.release()
		})
		waitQueued(t, g, i+1)
	}

	g.release()
	wg.Wait()

	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, served,
		"the gate served waiters out of arrival order, which is the unfairness it exists to remove")
}

// TestWriteGate_CancelWhileQueued: a queued writer whose context is cancelled
// gives up its place rather than the process. It must also not have taken the
// permit — a waiter that returned an error while holding it would hang every
// writer behind it.
func TestWriteGate_CancelWhileQueued(t *testing.T) {
	g := newWriteGate()
	require.NoError(t, g.acquire(t.Context(), true))

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- g.acquire(ctx, false) }()

	waitQueued(t, g, 1)
	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled waiter never returned")
	}
	assert.Zero(t, g.queued(), "the cancelled waiter is still in the queue")

	// The permit is still ours, and releasing it must not double-release.
	g.release()
	require.NoError(t, g.acquire(t.Context(), false), "the cancelled waiter left the permit taken")
	g.release()
}

// TestWriteGate_CountsTheReleasesAWriterWaitsThrough pins the statistic the
// contention test reads. A writer's wait is counted in the releases between
// its arrival and its own grant, or the moment it gives up, the holder's
// included when it arrives while the permit is held.
func TestWriteGate_CountsTheReleasesAWriterWaitsThrough(t *testing.T) {
	db := &DB{gate: newWriteGate()}
	g := db.gate
	require.NoError(t, g.acquire(t.Context(), true)) // grant 1: the holder

	// a and c queue behind the holder, then b, whose wait is cancelled
	// while c holds the permit.
	var wg sync.WaitGroup
	cHolds, cRelease := make(chan struct{}), make(chan struct{})
	queued := 0
	queue := func(fn func()) {
		wg.Go(fn)
		queued++
		waitQueued(t, g, queued) // in the queue before the next arrives
	}
	queue(func() {
		if g.acquire(context.Background(), false) == nil {
			g.release()
		}
	})
	queue(func() {
		if g.acquire(context.Background(), false) == nil {
			close(cHolds)
			<-cRelease
			g.release()
		}
	})
	bCtx, cancelB := context.WithCancel(context.Background())
	bDone := make(chan error, 1)
	queue(func() { bDone <- g.acquire(bCtx, false) })

	g.release()
	<-cHolds // a has been granted and released, c holds: b has waited through both
	cancelB()
	require.ErrorIs(t, <-bDone, context.Canceled)
	close(cRelease)
	wg.Wait()

	assert.Equal(t, WriteGateStats{Grants: 3, MostWaited: 2}, db.WriteGateStats(),
		"a waited through the holder's release, c through the holder's and a's, and b through the same two before it gave up")
	assert.Equal(t, WriteGateStats{}, (&DB{}).WriteGateStats(), "an ungated handle has nothing to report")
}

// TestWriteGate_ReentrantAcquisitionIsReported: a goroutine holding a
// transaction that asks for the permit again is asking to wait for itself. The
// gate says so instead of hanging.
func TestWriteGate_ReentrantAcquisitionIsReported(t *testing.T) {
	g := newWriteGate()
	require.NoError(t, g.acquire(t.Context(), true))
	defer g.release()

	err := g.acquire(t.Context(), false)
	require.ErrorIs(t, err, ErrWriteGateReentrant)
	assert.Contains(t, err.Error(), "already holds a write transaction")
}

// TestWriteGate_StatementHolderIsNotMistakenForReentrancy: only a
// transaction-scoped acquisition records an owner. A goroutine blocked behind
// someone else's statement must queue, not be told it is deadlocking itself.
func TestWriteGate_StatementHolderIsNotMistakenForReentrancy(t *testing.T) {
	g := newWriteGate()
	require.NoError(t, g.acquire(t.Context(), false))

	result := make(chan error, 1)
	go func() { result <- g.acquire(context.Background(), false) }()

	waitQueued(t, g, 1)
	g.release()

	select {
	case err := <-result:
		require.NoError(t, err)
		g.release()
	case <-time.After(5 * time.Second):
		t.Fatal("a queued writer never got the permit")
	}
}

// TestWriteGate_NilIsUngated keeps every write path writable as one expression:
// a handle opened without Options.SerializeWrites has a nil gate, and the same
// acquire/release pair must be a no-op on it.
func TestWriteGate_NilIsUngated(t *testing.T) {
	var g *writeGate
	require.NoError(t, g.acquire(t.Context(), true))
	g.release()
}

// TestSQLiteDSN_ImmediateTxIsOptIn pins the DSN plumbing on whichever driver
// this build uses. Both spell the parameter `_txlock=immediate`; neither
// accepts it as a pragma, so it is asserted as a substring rather than
// reconstructed.
func TestSQLiteDSN_ImmediateTxIsOptIn(t *testing.T) {
	const path = "/tmp/does-not-need-to-exist.db"
	assert.NotContains(t, sqliteDSN(path, Options{}), "_txlock",
		"a plain Open must not change how any other caller's transactions begin")
	assert.Contains(t, sqliteDSN(path, Options{ImmediateTx: true}), "_txlock=immediate")

	// An in-memory database is private to its pool: it has no cross-connection
	// lock to upgrade, and its DSN is passed through untouched.
	assert.Equal(t, ":memory:", sqliteDSN(":memory:", Options{ImmediateTx: true}))
}
