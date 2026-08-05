package host

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/host/storage/graph"
)

// projectStores memoizes the open project stores an App is using: at most one
// per project root, however many times and from however many goroutines the
// commands running under that App ask for one.
//
// The holder is a pointer so a converge worker can share the parent's — the
// worker clone is a field-by-field copy, and two copies each opening
// `.kapi/store.db` would be two connection pools on one SQLite file, which is
// the writer contention the merged store exists to remove.
type projectStores struct {
	mu  sync.Mutex
	dbs map[string]*projectdb.DB
	// graphs holds the property-graph handle bound to each store's pool. It is
	// memoized beside the store rather than inside projectdb because the graph
	// implementation lives in the host module: core/projectdb is framework code
	// and the arrow only points the other way.
	graphs map[string]*graph.SQLiteGraphStore
}

// ensureProjectStores returns the App's store holder, creating it on first use.
// It mirrors ensurePluginRuntime: the clone pre-seeds the parent's value so a
// worker's fresh sync.Once never builds a second holder.
func (a *App) ensureProjectStores() *projectStores {
	a.projectStoresOnce.Do(func() {
		if a.projectStores != nil {
			return // pre-seeded from a parent App (converge worker clone)
		}
		a.projectStores = &projectStores{
			dbs:    map[string]*projectdb.DB{},
			graphs: map[string]*graph.SQLiteGraphStore{},
		}
	})
	return a.projectStores
}

// ProjectDB returns the open store for the project rooted at root — the one
// database holding its content memory, terms, block cache and unit working set
// (core/projectdb).
//
// Lifetime: the App owns the handle. It is opened once per (App, project root),
// memoized, and safe to call concurrently; every caller under one App — including
// the per-locale workers a convergence pass fans out on — gets the same handle.
// Do NOT Close it, and do not Close the subsystem handles it hands out: they
// share its connection pool. Shutdown closes them all.
//
// Opening runs every subsystem's migrations and the predecessor sweep, so the
// memoization is what keeps that to once per project per process rather than
// once per command that touches a store.
//
// One handle is also the precondition for write discipline. Two pools on one
// SQLite file cannot serialize their writers in process, and deferred write
// transactions from several pools starve each other — the working store loses
// to whichever writer got there first and retries until it gives up. The gate
// that fixes it (IMMEDIATE write transactions behind a per-handle write mutex)
// belongs inside core/projectdb; what this accessor owes it is the invariant
// that there is exactly one handle per file per process to gate.
//
// root is the directory holding the recipe and the `.kapi/` state folder; an
// empty root is an error, because "no project" is a decision the caller makes.
func (a *App) ProjectDB(ctx context.Context, root string) (*projectdb.DB, error) {
	if root == "" {
		return nil, fmt.Errorf("project store: no project root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("project store: resolve root %q: %w", root, err)
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.dbs[abs]; ok {
		return db, nil
	}
	// Opening is App lifecycle, not request work, so it does not inherit the
	// caller's cancellation: the open runs every subsystem's migrations and
	// seeds the working set from the committed record, and the FIRST caller
	// happens to carry whichever context that command was cancelled with. A
	// cancelled run would otherwise fail to open the store — reporting
	// "count units: context canceled" as a run error — and, worse, could leave
	// a half-migrated store behind for the next one. Values still propagate;
	// only the deadline and cancellation are dropped.
	db, err := projectdb.Open(context.WithoutCancel(ctxOrBackground(ctx)), projectLayoutAt(abs))
	if err != nil {
		return nil, err
	}
	s.dbs[abs] = db
	return db, nil
}

// ErrNoProjectGraph reports that this build cannot hold a property graph,
// because it has no file-backed SQLite driver. It wraps storage.ErrNoSQLite, so
// a caller that already distinguishes the browser build needs no new check.
var ErrNoProjectGraph = fmt.Errorf("project graph: %w", storage.ErrNoSQLite)

// ProjectGraph returns the property graph bound to the project's store — the
// `graph_nodes` / `graph_edges` tables inside `.kapi/store.db`, migrated under
// their own `graph` ledger beside the block cache, the terms store, the content
// memory and the unit working set (AD-039).
//
// Lifetime matches ProjectDB exactly: one handle per (App, project root), opened
// on the store's own connection pool, memoized, safe to call concurrently. Do
// NOT Close it — it never owned the pool, and Shutdown releases the store.
//
// The graph relates what the other subsystems hold; it does not duplicate them.
// Edges key on durable identity (a block's content key, a unit key), never on a
// reader's positional id, so a re-parse that renumbers a document leaves the
// graph intact.
//
// On a build with no file-backed SQLite driver the store degrades to the JSON
// sidecar and there are no graph tables: this returns ErrNoProjectGraph.
func (a *App) ProjectGraph(ctx context.Context, root string) (*graph.SQLiteGraphStore, error) {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return nil, err
	}
	if db.Raw() == nil {
		return nil, ErrNoProjectGraph
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("project graph: resolve root %q: %w", root, err)
	}
	s := a.ensureProjectStores()
	s.mu.Lock()
	defer s.mu.Unlock()
	if g, ok := s.graphs[abs]; ok {
		return g, nil
	}
	g, err := graph.NewSQLiteGraphStore(db.Raw())
	if err != nil {
		return nil, fmt.Errorf("project graph: %w", err)
	}
	s.graphs[abs] = g
	return g, nil
}

// projectLayoutAt is the layout of the project rooted at an absolute root. Only
// the state directory matters to the store — the recipe may be named anything
// when it arrived via `-p` — so the recipe path is deliberately left unset
// rather than guessed.
func projectLayoutAt(root string) project.Layout {
	return project.Layout{Root: root, StateDir: filepath.Join(root, project.StateDirName)}
}

// closeProjectStores releases every store this App opened. Called by Shutdown;
// idempotent, so a second call after an early teardown is harmless.
func (a *App) closeProjectStores() {
	s := a.projectStores
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for root, g := range s.graphs {
		_ = g.Close() // detaches; the pool belongs to the store below
		delete(s.graphs, root)
	}
	for root, db := range s.dbs {
		_ = db.Close()
		delete(s.dbs, root)
	}
}

// StoreSelection says where a project-aware store command reads and writes: a
// STANDALONE store file, or the project's own merged store.
//
// The two are genuinely different things, which is why one type distinguishes
// them rather than a path with a magic value. A standalone store is a file the
// user named — `--memory ./corp.db`, `--termstore acme`, a profile's `terms:` —
// opened on its own pool and closed when the command ends. The project store is
// the App's shared handle, which the command must not close.
//
// The zero value means neither: nothing is bound, and the caller falls through
// to whatever "no store" means for it (usually: no leverage, no glossary).
type StoreSelection struct {
	// Path is the standalone store file. Empty when Root is set.
	Path string
	// Root is the project whose store governs. Empty when Path is set.
	Root string
	// Explicit records that the user named this store on the command line
	// (--termstore / --memory), as opposed to it being resolved from the
	// recipe. It is the difference between a store that is created on demand
	// because the user asked for it and one that is skipped because it does not
	// exist yet.
	Explicit bool
}

// InProject reports whether the project's own store governs this selection.
func (s StoreSelection) InProject() bool { return s.Root != "" }

// Bound reports whether the selection names a store at all.
func (s StoreSelection) Bound() bool { return s.Path != "" || s.Root != "" }

// ProjectStoreFor opens the project store for the project whose recipe path the
// command resolves to, and reports the project root alongside it. It returns
// (nil, "", nil) when no project is in scope — the shape every project-aware
// store resolution needs, since "no project" is not a failure.
func (a *App) ProjectStoreFor(cmd Command) (*projectdb.DB, string, error) {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return nil, "", err
	}
	root := filepath.Dir(projectPath)
	db, err := a.ProjectDB(CmdContext(cmd), root)
	if err != nil {
		return nil, "", err
	}
	return db, root, nil
}
