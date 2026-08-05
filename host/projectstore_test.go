package host

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeRoot scaffolds a project directory the store can be opened for.
func storeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, project.EnsureLayout(project.Layout{
		Root: root, StateDir: filepath.Join(root, project.StateDirName),
	}))
	return root
}

// One App, one project, one handle — however many times it is asked for. Two
// handles would be two connection pools on one SQLite file, which is exactly the
// arrangement the merged store exists to remove: nothing in this process could
// then serialize the two sets of writers against each other.
func TestProjectDB_OneHandlePerRoot(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	first, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	second, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	assert.Same(t, first, second, "the handle is memoized per project root")

	// And the subsystem handles come off the one pool.
	assert.Same(t, first.Memory(), second.Memory())
	assert.Same(t, first.Terms(), second.Terms())
	assert.Same(t, first.Work(), second.Work())
}

// A path that names the same directory differently must not open a second
// handle: the memo is keyed by the resolved root, not by the string.
func TestProjectDB_KeyedByResolvedRoot(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	first, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	second, err := a.ProjectDB(t.Context(), filepath.Join(root, "sub", ".."))
	require.NoError(t, err)
	assert.Same(t, first, second, "a differently-spelled path is the same project")
}

// Two projects are two handles. The memo must key on the root, not collapse
// every project in a process onto one store.
func TestProjectDB_DistinctRootsGetDistinctHandles(t *testing.T) {
	a := &App{}
	defer a.Shutdown()

	one, err := a.ProjectDB(t.Context(), storeRoot(t))
	require.NoError(t, err)
	two, err := a.ProjectDB(t.Context(), storeRoot(t))
	require.NoError(t, err)
	assert.NotSame(t, one, two)
}

// A convergence pass clones the App once per locale and runs them concurrently.
// Every worker must land on the parent's handle: a worker opening its own would
// put a second pool on `.kapi/store.db` for the duration of the pass, and the
// per-locale writes would contend with each other through the filesystem instead
// of through one process's writer.
//
// The clone pre-seeds the holder the way it pre-seeds the plugin runtime, so
// this also pins that a NEW App field of this kind must be shared rather than
// owned — the accounting TestConvergeWorker_CoversEveryAppField enforces.
func TestProjectDB_SharedAcrossConvergeWorkers(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	parent, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)

	locales := []string{"fr", "de", "nb", "ja", "es", "it", "pt", "nl"}
	handles := make([]*projectdb.DB, len(locales))
	var wg sync.WaitGroup
	for i, locale := range locales {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := a.convergeWorker(locale, newConvergeTap(locale))
			db, werr := worker.ProjectDB(context.Background(), root)
			require.NoError(t, werr)
			handles[i] = db
		}()
	}
	wg.Wait()

	for i, db := range handles {
		assert.Same(t, parent, db, "worker %d (%s) must share the parent's handle", i, locales[i])
	}
}

// The same, without a parent handle to inherit: the workers race to be first.
// Exactly one open must win, because the holder — not the handle — is what the
// clone shares, and it is what serializes them.
func TestProjectDB_ConcurrentFirstOpenYieldsOneHandle(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	const workers = 8
	handles := make([]*projectdb.DB, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker := a.convergeWorker("fr", newConvergeTap("fr"))
			db, err := worker.ProjectDB(context.Background(), root)
			require.NoError(t, err)
			handles[i] = db
		}()
	}
	wg.Wait()

	for i := 1; i < workers; i++ {
		assert.Same(t, handles[0], handles[i], "concurrent first opens must settle on one handle")
	}
}

// Shutdown closes what the App opened. Without this the desktop — which holds
// one App for the life of the process and opens a store per project the user
// visits — would accumulate connection pools until it ran out of file handles.
func TestProjectDB_ShutdownClosesEveryStore(t *testing.T) {
	a := &App{}
	rootA, rootB := storeRoot(t), storeRoot(t)

	dbA, err := a.ProjectDB(t.Context(), rootA)
	require.NoError(t, err)
	dbB, err := a.ProjectDB(t.Context(), rootB)
	require.NoError(t, err)

	a.Shutdown()

	assert.Nil(t, dbA.Raw(), "the pool is released")
	assert.Nil(t, dbB.Raw())

	// Idempotent: a second teardown after an early one must not panic.
	a.Shutdown()

	// And the App still works afterwards — Shutdown releases the stores, it does
	// not poison the accessor.
	reopened, err := a.ProjectDB(t.Context(), rootA)
	require.NoError(t, err)
	assert.NotNil(t, reopened.Raw())
	a.Shutdown()
}

// An empty root is a caller error, not a silently-skipped open: "there is no
// project" is a decision the caller makes before asking.
func TestProjectDB_EmptyRootIsAnError(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	_, err := a.ProjectDB(t.Context(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no project root")
}

// Opening does not inherit the caller's cancellation. The open runs every
// subsystem's migrations and seeds the working set, and the first caller happens
// to carry whichever context that command was cancelled with — a run cancelled
// before it started would otherwise fail with "context canceled" from the store
// layer, and could leave a half-migrated store for the next one.
func TestProjectDB_OpenSurvivesACancelledCaller(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err, "a cancelled caller must not leave the project unopenable")
	require.NotNil(t, db.Raw())
	assert.FileExists(t, filepath.Join(root, project.StateDirName, project.StoreFileName))
}

// A borrower reaches the owner's handle. An embedding host that must build a
// second App — the desktop builds one per run so per-run state stays owned —
// would otherwise put a second connection pool on `.kapi/store.db`, and the
// write gate, which is per pool, could no longer order the two sets of writers.
func TestShareProjectStores_BorrowerReachesTheSameHandle(t *testing.T) {
	owner := &App{}
	defer owner.Shutdown()
	root := storeRoot(t)

	borrower := &App{}
	owner.ShareProjectStores(borrower)

	fromBorrower, err := borrower.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	fromOwner, err := owner.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	assert.Same(t, fromOwner, fromBorrower, "one handle per project across both Apps")

	// And the direction is symmetric: a store the OWNER opened first is the one
	// the borrower gets, not merely the other way round.
	otherRoot := storeRoot(t)
	first, err := owner.ProjectDB(t.Context(), otherRoot)
	require.NoError(t, err)
	second, err := borrower.ProjectDB(t.Context(), otherRoot)
	require.NoError(t, err)
	assert.Same(t, first, second)
}

// Sharing is a no-op in the degenerate cases rather than a panic: a nil
// borrower, or an App handed to itself.
func TestShareProjectStores_IgnoresNilAndSelf(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	a.ShareProjectStores(nil)
	a.ShareProjectStores(a)

	db, err := a.ProjectDB(t.Context(), storeRoot(t))
	require.NoError(t, err)
	assert.NotNil(t, db.Raw())
}

// Closing one project's store leaves the others alone and lets the next open
// build a fresh handle. A host whose projects come and go inside one process —
// the desktop closing a tab, or renaming a sample's directory out from under
// itself — needs that, or the reopened project gets a handle on the old file.
func TestCloseProjectDB_ClosesOneAndReopensFresh(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	rootA, rootB := storeRoot(t), storeRoot(t)

	dbA, err := a.ProjectDB(t.Context(), rootA)
	require.NoError(t, err)
	dbB, err := a.ProjectDB(t.Context(), rootB)
	require.NoError(t, err)

	require.NoError(t, a.CloseProjectDB(rootA))
	assert.Nil(t, dbA.Raw(), "the closed project's pool is released")
	assert.NotNil(t, dbB.Raw(), "the other project is untouched")

	reopened, err := a.ProjectDB(t.Context(), rootA)
	require.NoError(t, err)
	assert.NotSame(t, dbA, reopened, "the next open builds a fresh handle")
	assert.NotNil(t, reopened.Raw())
}

// Closing a project this App never opened — or closing before anything has been
// opened at all — is not an error. Tab teardown runs on paths that may never
// have reached a store.
func TestCloseProjectDB_UnknownRootIsNotAnError(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	require.NoError(t, a.CloseProjectDB(storeRoot(t)))
	require.NoError(t, a.CloseProjectDB(""))
}
