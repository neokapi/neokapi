package host

import (
	"path/filepath"
	"sort"
	"sync"
	"testing"

	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The graph is memoized the same way the store is: one handle per (App, root),
// on the store's own pool. A second handle would mean a second migration run
// against a live database for no gain.
func TestProjectGraph_OneHandlePerRoot(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	first, err := a.ProjectGraph(t.Context(), root)
	require.NoError(t, err)
	second, err := a.ProjectGraph(t.Context(), filepath.Join(root, "sub", ".."))
	require.NoError(t, err)
	assert.Same(t, first, second, "the graph handle is memoized per resolved root")
}

func TestProjectGraph_DistinctRootsGetDistinctHandles(t *testing.T) {
	a := &App{}
	defer a.Shutdown()

	one, err := a.ProjectGraph(t.Context(), storeRoot(t))
	require.NoError(t, err)
	two, err := a.ProjectGraph(t.Context(), storeRoot(t))
	require.NoError(t, err)
	assert.NotSame(t, one, two, "two projects are two graphs")
}

func TestProjectGraph_ConcurrentFirstOpenYieldsOneHandle(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	const callers = 8
	handles := make([]any, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			g, err := a.ProjectGraph(t.Context(), root)
			assert.NoError(t, err)
			handles[i] = g
		})
	}
	wg.Wait()
	for i := 1; i < callers; i++ {
		assert.Same(t, handles[0], handles[i], "every concurrent caller gets the one graph")
	}
}

func TestProjectGraph_EmptyRootIsAnError(t *testing.T) {
	a := &App{}
	defer a.Shutdown()

	_, err := a.ProjectGraph(t.Context(), "")
	require.Error(t, err)
}

// The graph rides in `.kapi/work/store.db` under its own `graph` ledger, beside the
// block cache, the terms store, the content memory and the unit working set.
// Ledger coexistence is the whole premise of the merged store: each subsystem
// migrates independently, so none of them can replay another's history.
func TestProjectGraph_LedgersCoexistInOneFile(t *testing.T) {
	a := &App{}
	defer a.Shutdown()
	root := storeRoot(t)

	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	g, err := a.ProjectGraph(t.Context(), root)
	require.NoError(t, err)

	// Every subsystem's bookkeeping table is present in the one file.
	rows, err := db.Raw().QueryContext(t.Context(),
		`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	require.NoError(t, rows.Err())
	sort.Strings(tables)
	for _, want := range []string{
		"graph", "graph_nodes", "graph_edges", // the graph and its ledger
		"cache_migrations", "blocks", // the block cache
		"termbase_migrations", "tb_concepts", // the terms store
		"sievepen_migrations", "tm_entries", // the content memory
	} {
		assert.Contains(t, tables, want, "one file holds every subsystem's tables")
	}

	// And the graph is usable through the shared pool.
	require.NoError(t, g.CreateNode(t.Context(), &coreg.Node{ID: "n1", Label: "Concept"}))
	got, err := g.GetNode(t.Context(), "n1")
	require.NoError(t, err)
	assert.Equal(t, "Concept", got.Label)

	// Reopening the file must not replay anyone's migrations: a second App on
	// the same root migrates every subsystem again and finds nothing to do.
	a.Shutdown()
	b := &App{}
	defer b.Shutdown()
	reopened, err := b.ProjectGraph(t.Context(), root)
	require.NoError(t, err)
	again, err := reopened.GetNode(t.Context(), "n1")
	require.NoError(t, err)
	assert.Equal(t, "Concept", again.Label, "the graph survives a reopen intact")
}

// Shutdown drops the graph handles with the stores. The graph never owned the
// pool, so its Close must be a detach — closing it would pull the file out from
// under four other subsystems.
func TestProjectGraph_ShutdownReleasesHandles(t *testing.T) {
	a := &App{}
	root := storeRoot(t)

	g, err := a.ProjectGraph(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, g.CreateNode(t.Context(), &coreg.Node{ID: "n1", Label: "Concept"}))

	a.Shutdown()
	assert.Empty(t, a.projectStores.graphs, "Shutdown drops the graph handles")
	assert.Empty(t, a.projectStores.dbs, "Shutdown drops the store handles")

	// A fresh App reopens the same file and reads what was written.
	b := &App{}
	defer b.Shutdown()
	reopened, err := b.ProjectGraph(t.Context(), root)
	require.NoError(t, err)
	got, err := reopened.GetNode(t.Context(), "n1")
	require.NoError(t, err)
	assert.Equal(t, "Concept", got.Label)
}
