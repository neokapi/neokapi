package projectdb_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory"
)

// These are the regression tests for what a single project file costs and how
// it is paid for. The measurement they descend from is scripts/contention-harness,
// which runs the same shapes at dogfood scale for a minute apiece; this file
// runs them for a second or two, which is enough to tell a fair queue from an
// unfair one.

// isBusy classifies a SQLite lock refusal. Neither driver exposes a portable
// sentinel through database/sql, so the phrasing is what there is to match on —
// the same test core/storage makes internally.
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "database is locked") ||
		strings.Contains(s, "SQLITE_BUSY") ||
		strings.Contains(s, "table is locked")
}

func memoryEntry(i int) memory.Entry {
	return memory.Entry{
		ID: fmt.Sprintf("e%06d", i),
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR(fmt.Sprintf(
				"Entry %d exists to give the write a realistic size, because a write that "+
					"costs nothing cannot starve anything.", i))},
			"nb": {model.TextR(fmt.Sprintf("Oppføring %d finnes for å gi skrivingen en realistisk størrelse.", i))},
		},
		HintSrcLang: "en",
	}
}

func unitState(i int) state.UnitState {
	return state.UnitState{
		Unit:        fmt.Sprintf("k1_%040x", i),
		Variant:     model.Variant("nb"),
		Status:      model.TargetStatusTranslated,
		ContentHash: fmt.Sprintf("k1_%040x", i),
		Scope:       "docs",
		Updated:     time.Now().UTC().Format(time.RFC3339),
	}
}

// TestWriteGate_SmallWritesAreNotStarved is the regression test for the failure
// that motivated the whole design. Fat writers (a converge run teaching the
// content memory) run flat out against a drip of small writes (a review loop
// recording decisions) on ONE handle. Before the gate the drip completed
// roughly one per cent of its attempts and the rest failed SQLITE_BUSY after
// the five-second timeout; the busy handler's backoff has no memory of who has
// been waiting longest, so a two-millisecond writer loses to a two-second one
// indefinitely.
//
// The floor is deliberately generous — this asserts the shape (the drip runs,
// and no write is ever refused for the lock), not a throughput number, which
// belongs to the harness and to the machine it runs on.
func TestWriteGate_SmallWritesAreNotStarved(t *testing.T) {
	if testing.Short() {
		t.Skip("contention regression: seconds of deliberate lock pressure")
	}
	db := openStore(t, newLayout(t))
	mem := db.Memory()
	require.NotNil(t, mem)
	work := db.Work()

	const (
		fatWriters = 4
		window     = 2 * time.Second
		dripEvery  = 20 * time.Millisecond
	)
	// What the drip would manage with nothing in its way, give or take: one
	// write per tick. The floor is a quarter of that, far below the ~90% the
	// harness measures at scale, because a laptop under test load is noisy and
	// this test is about zero busy errors and a drip that is not shut out.
	dripFloor := int64(window/dripEvery) / 4

	ctx, cancel := context.WithTimeout(t.Context(), window)
	defer cancel()

	var (
		wg       sync.WaitGroup
		busy     atomic.Int64
		other    atomic.Int64
		dripOK   atomic.Int64
		dripBusy atomic.Int64
	)
	record := func(err error) {
		switch {
		case err == nil, isCtxErr(err):
		case isBusy(err):
			busy.Add(1)
		default:
			other.Add(1)
		}
	}

	for w := range fatWriters {
		wg.Go(func() {
			batch := make([]memory.Entry, 0, 25)
			for n := 0; ctx.Err() == nil; n++ {
				batch = batch[:0]
				for k := range 25 {
					batch = append(batch, memoryEntry(w*100_000+n*25+k))
				}
				record(mem.BulkAddWithStream(ctx, batch, ""))
			}
		})
	}

	wg.Go(func() {
		tick := time.NewTicker(dripEvery)
		defer tick.Stop()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			err := work.Put(ctx, unitState(i))
			switch {
			case err == nil:
				dripOK.Add(1)
			case isCtxErr(err):
			case isBusy(err):
				dripBusy.Add(1)
			default:
				other.Add(1)
			}
		}
	})
	wg.Wait()

	assert.Zero(t, busy.Load(), "a write on the gated handle was refused for the lock")
	assert.Zero(t, dripBusy.Load(), "the drip writer was refused for the lock — the starvation is back")
	assert.Zero(t, other.Load(), "a write failed for a reason other than the lock")
	assert.GreaterOrEqualf(t, dripOK.Load(), dripFloor,
		"the drip completed %d writes in %s, below the floor of %d — it is being starved",
		dripOK.Load(), window, dripFloor)
}

func isCtxErr(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), context.DeadlineExceeded.Error()) ||
			strings.Contains(err.Error(), context.Canceled.Error()))
}

// TestWriteGate_ServesWritersInArrivalOrder: the queue is Go's, and Go serves
// blocked channel senders in the order they blocked. This is the property
// SQLite's busy handler does not have, and the reason a mutex would not do
// either.
func TestWriteGate_ServesWritersInArrivalOrder(t *testing.T) {
	db := openStore(t, newLayout(t))
	raw := db.Raw()
	require.NotNil(t, raw)

	// Hold the permit so everyone queues behind a known point.
	holder, err := raw.BeginTx(t.Context(), nil)
	require.NoError(t, err)

	const waiters = 5
	var (
		mu     sync.Mutex
		served []int
		wg     sync.WaitGroup
	)
	for i := range waiters {
		started := make(chan struct{})
		wg.Go(func() {
			close(started)
			if _, err := raw.ExecContext(context.Background(),
				`INSERT INTO store_meta (key, value) VALUES (?, ?)
				 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
				fmt.Sprintf("order-%d", i), "x"); err != nil {
				return
			}
			mu.Lock()
			served = append(served, i)
			mu.Unlock()
		})
		<-started
		time.Sleep(20 * time.Millisecond) // let this one reach the queue before the next
	}

	require.NoError(t, holder.Rollback())
	wg.Wait()

	assert.Equal(t, []int{0, 1, 2, 3, 4}, served,
		"writers were served out of arrival order")
}

// TestWriteGate_QueuedWriterHonoursCancellation: a writer that gives up while
// queued returns its caller's context error and leaves the queue intact for
// everyone behind it.
func TestWriteGate_QueuedWriterHonoursCancellation(t *testing.T) {
	db := openStore(t, newLayout(t))
	raw := db.Raw()
	require.NotNil(t, raw)

	holder, err := raw.BeginTx(t.Context(), nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() {
		_, err := raw.ExecContext(ctx, `INSERT INTO store_meta (key, value) VALUES ('c', 'x')`)
		queued <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-queued:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(10 * time.Second):
		t.Fatal("a cancelled writer waited for the transaction anyway")
	}

	require.NoError(t, holder.Rollback())

	// The handle is still writable: the abandoned waiter took nothing with it.
	_, err = raw.ExecContext(t.Context(), `INSERT INTO store_meta (key, value) VALUES ('after', 'x')`)
	require.NoError(t, err)
}

// TestBlockSession_HoldsTheGateForItsWholeLife pins the extraction contract. A
// session from Blocks() is one transaction over a purge-and-refill, so it is the
// project store's single writer from Begin to Commit — a content-memory write
// issued meanwhile queues, and lands the moment the session ends.
func TestBlockSession_HoldsTheGateForItsWholeLife(t *testing.T) {
	db := openStore(t, newLayout(t))
	mem := db.Memory()
	require.NotNil(t, mem)

	sess, err := db.Blocks().Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("docs", &blockstore.Block{
		ID: "u1", Hash: "k1_" + strings.Repeat("a", 40), Translatable: true,
		Source: []model.Run{model.TextR("held")},
	}))

	written := make(chan error, 1)
	go func() { written <- mem.Add(context.Background(), memoryEntry(1)) }()

	select {
	case err := <-written:
		t.Fatalf("a content-memory write landed while an extraction session held the store: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	require.NoError(t, sess.Commit())

	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the queued content-memory write never ran after the session committed")
	}
}

// TestBlockSession_WritingFromTheHoldingGoroutineIsReported: the session holds
// the permit, so the goroutine holding the session cannot also write elsewhere
// on the handle. That is a deadlock, and the gate says so rather than stopping
// the process. It is the one piece of discipline a call site still has to know,
// which is why it fails loudly.
func TestBlockSession_WritingFromTheHoldingGoroutineIsReported(t *testing.T) {
	db := openStore(t, newLayout(t))
	mem := db.Memory()
	require.NotNil(t, mem)

	sess, err := db.Blocks().Begin(t.Context())
	require.NoError(t, err)
	defer func() { _ = sess.Rollback() }()

	err = mem.Add(t.Context(), memoryEntry(2))
	require.ErrorIs(t, err, storage.ErrWriteGateReentrant)
	assert.Contains(t, err.Error(), "already holds a write transaction")
}

// TestAutocommitBlocks_DoNotHoldTheGate: the autocommit block store is the mode
// concurrent flow runs use precisely because they cannot afford a run-long
// transaction. Each of its writes takes and returns the permit on its own, so
// several sessions write at once.
func TestAutocommitBlocks_DoNotHoldTheGate(t *testing.T) {
	db := openStore(t, newLayout(t))
	store := db.BlocksAutocommit()
	require.NotNil(t, store)

	a, err := store.Begin(t.Context())
	require.NoError(t, err)
	defer a.Close()

	done := make(chan error, 1)
	go func() {
		b, err := store.Begin(context.Background())
		if err != nil {
			done <- err
			return
		}
		defer b.Close()
		done <- b.PutBlock("docs", &blockstore.Block{
			ID: "u2", Hash: "k1_" + strings.Repeat("b", 40),
			Source: []model.Run{model.TextR("concurrent")},
		})
	}()

	require.NoError(t, a.PutBlock("docs", &blockstore.Block{
		ID: "u1", Hash: "k1_" + strings.Repeat("a", 40),
		Source: []model.Run{model.TextR("concurrent")},
	}))

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("an autocommit session blocked another one")
	}
}

// TestProjectStore_OpensWithImmediateTransactions asserts the DSN half reached
// the handle: a transaction on the project store takes its write lock at BEGIN,
// where busy_timeout applies, instead of upgrading a read lock later, where it
// does not.
func TestProjectStore_OpensWithImmediateTransactions(t *testing.T) {
	db := openStore(t, newLayout(t))
	raw := db.Raw()
	require.NotNil(t, raw)

	// A second handle on the same file stands in for a second kapi process:
	// the gate cannot reach it, so what it observes is SQLite's own locking.
	// It is opened BEFORE the transaction begins, because opening asserts the
	// journal mode and that would itself queue behind a held write lock.
	other, err := storage.Open(db.Path())
	require.NoError(t, err)
	defer other.Close()

	tx, err := raw.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	// The transaction has issued no statement. A DEFERRED transaction would
	// hold nothing at this point and the write below would sail through; an
	// IMMEDIATE one took the write lock at BEGIN, so it does not.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err = other.ExecContext(ctx,
		`INSERT INTO store_meta (key, value) VALUES ('other', 'x')`)
	require.Error(t, err, "the project store's transaction did not take its write lock at BEGIN")
	assert.True(t, isBusy(err) || isCtxErr(err), "unexpected failure: %v", err)

	// And the lock it holds is a WAL write lock, not an exclusive database
	// lock: a reader on the other handle is unaffected.
	var n int
	require.NoError(t, other.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM store_meta`).Scan(&n))
}

// Compile-time reminder that projectdb.DB is the handle these properties belong
// to; a future refactor that hands out raw *sql.DB would silently drop them.
var _ = func(db *projectdb.DB) *storage.DB { return db.Raw() }
