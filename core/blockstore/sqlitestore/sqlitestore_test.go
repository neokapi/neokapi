//go:build !wasm

package sqlitestore_test

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Store/Session contract itself is covered by the shared conformance suite
// in core/blockstore (runStoreSuite, run against sqlitestore.New). These tests
// cover what that suite does not reach: the AUTOCOMMIT session mode, the
// transaction semantics that separate the two constructors, and the error
// paths.

func newStore(t *testing.T, autocommit bool) blockstore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cache.db")
	var (
		s   blockstore.Store
		err error
	)
	if autocommit {
		s, err = sqlitestore.NewAutocommit(path)
	} else {
		s, err = sqlitestore.New(path)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func block(hash string, translatable bool) *blockstore.Block {
	return &blockstore.Block{ID: "id-" + hash, Hash: hash, Translatable: translatable}
}

// --- the two session modes -------------------------------------------------

// TestRollbackDiscardsWritesInTransactionalMode pins the all-or-nothing
// contract New exists for: extraction purges and refills, and a failure part
// way through must leave the previous block set intact.
func TestRollbackDiscardsWritesInTransactionalMode(t *testing.T) {
	s := newStore(t, false)
	ctx := context.Background()

	sess, err := s.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("ui", block("abandoned", true)))
	require.NoError(t, sess.Rollback())

	after, err := s.Begin(ctx)
	require.NoError(t, err)
	defer after.Close()
	_, err = after.GetBlock("abandoned")
	assert.ErrorIs(t, err, blockstore.ErrNotFound, "a rolled-back write must not be visible")
}

// TestRollbackKeepsWritesInAutocommitMode pins the documented divergence:
// autocommit writes are durable the moment they return, and Rollback only
// closes the session. Overlays are idempotent per key, so the work a cancelled
// run completed is valid work a later pass reuses.
func TestRollbackKeepsWritesInAutocommitMode(t *testing.T) {
	s := newStore(t, true)
	ctx := context.Background()

	sess, err := s.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("ui", block("kept", true)))
	require.NoError(t, sess.Rollback())

	after, err := s.Begin(ctx)
	require.NoError(t, err)
	defer after.Close()
	got, err := after.GetBlock("kept")
	require.NoError(t, err)
	assert.Equal(t, "kept", got.Hash)
}

// TestUncommittedWritesAreInvisibleToAnotherSession holds the transactional
// mode to read isolation.
func TestUncommittedWritesAreInvisibleToAnotherSession(t *testing.T) {
	s := newStore(t, false)
	ctx := context.Background()

	writer, err := s.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, writer.PutBlock("ui", block("inflight", true)))

	// A reader opened on the same store must not see the open transaction's
	// write; SQLite in WAL mode serves it the last committed snapshot.
	reader, err := s.Begin(ctx)
	require.NoError(t, err)
	_, err = reader.GetBlock("inflight")
	require.ErrorIs(t, err, blockstore.ErrNotFound)
	require.NoError(t, reader.Rollback())

	require.NoError(t, writer.Commit())
}

// TestConcurrentAutocommitSessions is the case NewAutocommit exists for:
// converge fans locales out and each file-run holds its own session. Under the
// transactional mode these would deadlock-avoid into immediate SQLITE_BUSY.
// Run with -race, this also covers the store's shared *storage.DB access.
func TestConcurrentAutocommitSessions(t *testing.T) {
	s := newStore(t, true)
	ctx := context.Background()

	const workers, perWorker = 8, 10
	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)

	for w := range workers {
		wg.Go(func() {
			sess, err := s.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			defer sess.Close()
			for i := range perWorker {
				hash := fmt.Sprintf("w%d-b%d", w, i)
				if err := sess.PutBlock("ui", block(hash, true)); err != nil {
					errs <- fmt.Errorf("put block %s: %w", hash, err)
					return
				}
				if err := sess.PutOverlay(blockstore.Overlay{
					Kind: "targets/nb", BlockHash: hash, Payload: []byte(`{"t":"x"}`),
				}); err != nil {
					errs <- fmt.Errorf("put overlay %s: %w", hash, err)
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	sess, err := s.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	var blocks, overlays int
	for _, err := range sess.Blocks(blockstore.BlockFilter{}) {
		require.NoError(t, err)
		blocks++
	}
	for _, err := range allOverlays(t, sess) {
		require.NoError(t, err)
		overlays++
	}
	assert.Equal(t, workers*perWorker, blocks, "every concurrent write should be durable")
	assert.Equal(t, workers*perWorker, overlays)
}

func TestCommitIsALifecycleNoOpInAutocommitMode(t *testing.T) {
	s := newStore(t, true)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)

	require.NoError(t, sess.PutBlock("ui", block("a", true)))
	require.NoError(t, sess.Commit())

	// The session is spent either way: a second Commit, and any further work,
	// must report the session closed rather than silently running on the pool.
	require.ErrorIs(t, sess.Commit(), blockstore.ErrClosed)
	require.ErrorIs(t, sess.PutBlock("ui", block("b", true)), blockstore.ErrClosed)
	_, err = sess.GetBlock("a")
	assert.ErrorIs(t, err, blockstore.ErrClosed)
}

func TestRollbackAfterCommitIsNotAnError(t *testing.T) {
	for _, autocommit := range []bool{false, true} {
		t.Run(fmt.Sprintf("autocommit=%v", autocommit), func(t *testing.T) {
			s := newStore(t, autocommit)
			sess, err := s.Begin(context.Background())
			require.NoError(t, err)
			require.NoError(t, sess.PutBlock("ui", block("a", true)))
			require.NoError(t, sess.Commit())

			// Session.Close is documented as rollback-if-not-committed, so the
			// common `defer sess.Close()` after an explicit Commit must be inert.
			assert.NoError(t, sess.Rollback())
			assert.NoError(t, sess.Close())
		})
	}
}

func TestClosedSessionRefusesEveryOperation(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, sess.Rollback())

	_, err = sess.GetBlock("x")
	require.ErrorIs(t, err, blockstore.ErrClosed)
	require.ErrorIs(t, sess.PutBlock("ui", block("x", true)), blockstore.ErrClosed)
	require.ErrorIs(t, purger(t, sess).DeleteBlocks(), blockstore.ErrClosed)
	require.ErrorIs(t, sess.PutOverlay(blockstore.Overlay{Kind: "k", BlockHash: "x"}), blockstore.ErrClosed)
	_, err = sess.GetOverlay("k", "x")
	require.ErrorIs(t, err, blockstore.ErrClosed)

	// The iterators report the closure through the sequence rather than
	// yielding nothing, so a caller cannot mistake a closed session for an
	// empty store.
	for _, err := range sess.Blocks(blockstore.BlockFilter{}) {
		require.ErrorIs(t, err, blockstore.ErrClosed)
	}
	for _, err := range sess.ListOverlays("k") {
		require.ErrorIs(t, err, blockstore.ErrClosed)
	}
	for _, err := range allOverlays(t, sess) {
		require.ErrorIs(t, err, blockstore.ErrClosed)
	}
}

func TestBeginAfterCloseReportsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	s, err := sqlitestore.New(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	_, err = s.Begin(context.Background())
	require.ErrorIs(t, err, blockstore.ErrClosed)

	assert.NoError(t, s.Close(), "closing twice is a no-op")
}

// --- validation ------------------------------------------------------------

func TestPutBlockRejectsAnUnkeyedBlock(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	// The hash is the primary key; without it the row would collide with every
	// other unkeyed block rather than store anything useful.
	require.Error(t, sess.PutBlock("ui", nil))
	require.Error(t, sess.PutBlock("ui", &blockstore.Block{Hash: ""}))
}

func TestPutOverlayRequiresBothKeyParts(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	require.Error(t, sess.PutOverlay(blockstore.Overlay{BlockHash: "h"}), "missing Kind")
	require.Error(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb"}), "missing BlockHash")
}

func TestPutOverlayStampsUpdatedAtWhenZero(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	before := time.Now().Unix()
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "h", Payload: []byte("{}")}))

	got, err := sess.GetOverlay("targets/nb", "h")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, got.UpdatedAt, before, "a zero UpdatedAt means the store picks")

	// A caller-supplied stamp is preserved verbatim.
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "h2", Payload: []byte("{}"), UpdatedAt: 42}))
	got, err = sess.GetOverlay("targets/nb", "h2")
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.UpdatedAt)
}

// --- upsert ----------------------------------------------------------------

func TestPutBlockUpsertsOnHash(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutBlock("ui", block("h", true)))
	require.NoError(t, sess.PutBlock("docs", block("h", false)))

	got, err := sess.GetBlock("h")
	require.NoError(t, err)
	assert.False(t, got.Translatable, "the second write replaces the payload")

	// The collection moved with it, so the old collection no longer matches.
	assert.Equal(t, 0, countBlocks(t, sess, blockstore.BlockFilter{Collection: "ui"}))
	assert.Equal(t, 1, countBlocks(t, sess, blockstore.BlockFilter{Collection: "docs"}))
}

func TestPutOverlayUpsertsOnKindAndHash(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "h", Payload: []byte(`{"v":1}`), UpdatedAt: 1}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "h", Payload: []byte(`{"v":2}`), UpdatedAt: 2}))

	got, err := sess.GetOverlay("targets/nb", "h")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v":2}`, string(got.Payload))
	assert.Equal(t, int64(2), got.UpdatedAt)

	// A different kind on the same block is a separate row, not a replacement:
	// targets and annotations coexist per block.
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "annotations/terms", BlockHash: "h", Payload: []byte(`{}`)}))
	assert.Equal(t, 2, countAllOverlays(t, sess))
}

// --- filters ---------------------------------------------------------------

func TestBlocksFilter(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutBlock("ui", block("a", true)))
	require.NoError(t, sess.PutBlock("ui", block("b", false)))
	require.NoError(t, sess.PutBlock("docs", block("c", true)))

	yes, no := true, false
	tests := []struct {
		name   string
		filter blockstore.BlockFilter
		want   int
	}{
		{"unfiltered", blockstore.BlockFilter{}, 3},
		{"by collection", blockstore.BlockFilter{Collection: "ui"}, 2},
		{"translatable", blockstore.BlockFilter{Translatable: &yes}, 2},
		{"not translatable", blockstore.BlockFilter{Translatable: &no}, 1},
		{"collection and flag", blockstore.BlockFilter{Collection: "ui", Translatable: &yes}, 1},
		{"limit", blockstore.BlockFilter{Limit: 2}, 2},
		{"limit above total", blockstore.BlockFilter{Limit: 99}, 3},
		{"unknown collection", blockstore.BlockFilter{Collection: "nope"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, countBlocks(t, sess, tt.filter))
		})
	}
}

func TestBlocksIterationIsOrderedByHash(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	for _, h := range []string{"c", "a", "b"} {
		require.NoError(t, sess.PutBlock("ui", block(h, true)))
	}

	var got []string
	for b, err := range sess.Blocks(blockstore.BlockFilter{}) {
		require.NoError(t, err)
		got = append(got, b.Hash)
	}
	// A stable order keeps a re-extraction diffable and the limit meaningful.
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

// TestBlocksIterationStopsOnBreak covers the yield contract: an early break
// must not be treated as an error, and must release the rows.
func TestBlocksIterationStopsOnBreak(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	for _, h := range []string{"a", "b", "c"} {
		require.NoError(t, sess.PutBlock("ui", block(h, true)))
	}

	seen := 0
	for _, err := range sess.Blocks(blockstore.BlockFilter{}) {
		require.NoError(t, err)
		seen++
		break
	}
	assert.Equal(t, 1, seen)

	// The session is still usable after an abandoned iteration.
	assert.Equal(t, 3, countBlocks(t, sess, blockstore.BlockFilter{}))
}

func TestListOverlaysIsScopedByKind(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "a", Payload: []byte("{}")}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "b", Payload: []byte("{}")}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/de", BlockHash: "a", Payload: []byte("{}")}))

	var nb []string
	for o, err := range sess.ListOverlays("targets/nb") {
		require.NoError(t, err)
		assert.Equal(t, "targets/nb", o.Kind)
		nb = append(nb, o.BlockHash)
	}
	assert.Equal(t, []string{"a", "b"}, nb, "ordered by block hash within a kind")

	assert.Equal(t, 3, countAllOverlays(t, sess))
	assert.Equal(t, 0, countOverlays(t, sess, "targets/fr"))
}

// --- purge -----------------------------------------------------------------

// TestDeleteBlocksLeavesOverlaysIntact is the contract a full re-extraction
// depends on: the block set is rebuilt from source, but the translations and
// annotations keyed to those hashes are the expensive part and must survive.
func TestDeleteBlocksLeavesOverlaysIntact(t *testing.T) {
	s := newStore(t, false)
	sess, err := s.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutBlock("ui", block("a", true)))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/nb", BlockHash: "a", Payload: []byte(`{"t":"x"}`)}))

	require.NoError(t, purger(t, sess).DeleteBlocks())

	assert.Equal(t, 0, countBlocks(t, sess, blockstore.BlockFilter{}))
	got, err := sess.GetOverlay("targets/nb", "a")
	require.NoError(t, err, "the overlay outlives the block it annotates")
	assert.JSONEq(t, `{"t":"x"}`, string(got.Payload))
}

// --- store ----------------------------------------------------------------

func TestCapabilities(t *testing.T) {
	for _, autocommit := range []bool{false, true} {
		t.Run(fmt.Sprintf("autocommit=%v", autocommit), func(t *testing.T) {
			s := newStore(t, autocommit)
			caps := s.Capabilities()
			assert.True(t, caps.RandomAccess)
			assert.True(t, caps.Concurrent)
			assert.True(t, caps.Writable)
			assert.True(t, caps.Persistent)
			assert.False(t, caps.Remote, "the cache is a local file")

			sess, err := s.Begin(context.Background())
			require.NoError(t, err)
			defer sess.Close()
			assert.Equal(t, caps, sess.Capabilities(), "a session reports its store's capabilities")
		})
	}
}

func TestOpenIsIdempotentAcrossModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")

	s1, err := sqlitestore.New(path)
	require.NoError(t, err)
	sess, err := s1.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("ui", block("shared", true)))
	require.NoError(t, sess.Commit())
	require.NoError(t, s1.Close())

	// Re-opening the same file in the other mode must find the migrations
	// already applied and the data intact — the cache outlives one flow's
	// choice of session mode.
	s2, err := sqlitestore.NewAutocommit(path)
	require.NoError(t, err)
	defer s2.Close()
	sess2, err := s2.Begin(context.Background())
	require.NoError(t, err)
	defer sess2.Close()
	got, err := sess2.GetBlock("shared")
	require.NoError(t, err)
	assert.Equal(t, "shared", got.Hash)
}

func TestOpenFailsOnAnUnusablePath(t *testing.T) {
	// A parent path component that is a regular file fails for any uid, unlike
	// a read-only directory, which root would happily write through.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	_, err := sqlitestore.New(filepath.Join(blocker, "cache.db"))
	assert.Error(t, err)
}

// --- helpers ---------------------------------------------------------------

// allOverlays and purger assert the two optional Session capabilities. The
// SQLite store implements both — callers treat their absence as "no bulk
// delete" and "no export", so losing one silently would degrade a
// re-extraction or an export rather than fail it.
func allOverlays(t *testing.T, sess blockstore.Session) iter.Seq2[blockstore.Overlay, error] {
	t.Helper()
	e, ok := sess.(blockstore.OverlayEnumerator)
	require.True(t, ok, "the SQLite session must implement OverlayEnumerator")
	return e.AllOverlays()
}

func purger(t *testing.T, sess blockstore.Session) blockstore.BlockPurger {
	t.Helper()
	p, ok := sess.(blockstore.BlockPurger)
	require.True(t, ok, "the SQLite session must implement BlockPurger")
	return p
}

func countBlocks(t *testing.T, sess blockstore.Session, f blockstore.BlockFilter) int {
	t.Helper()
	n := 0
	for _, err := range sess.Blocks(f) {
		require.NoError(t, err)
		n++
	}
	return n
}

func countOverlays(t *testing.T, sess blockstore.Session, kind string) int {
	t.Helper()
	n := 0
	for _, err := range sess.ListOverlays(kind) {
		require.NoError(t, err)
		n++
	}
	return n
}

func countAllOverlays(t *testing.T, sess blockstore.Session) int {
	t.Helper()
	n := 0
	for _, err := range allOverlays(t, sess) {
		require.NoError(t, err)
		n++
	}
	return n
}
