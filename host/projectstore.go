package host

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
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
}

// ensureProjectStores returns the App's store holder, creating it on first use.
// It mirrors ensurePluginRuntime: the clone pre-seeds the parent's value so a
// worker's fresh sync.Once never builds a second holder.
func (a *App) ensureProjectStores() *projectStores {
	a.projectStoresOnce.Do(func() {
		if a.projectStores != nil {
			return // pre-seeded from a parent App (converge worker clone)
		}
		a.projectStores = &projectStores{dbs: map[string]*projectdb.DB{}}
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

// projectLayoutAt is the layout of the project rooted at an absolute root. Only
// the state directory matters to the store — the recipe may be named anything
// when it arrived via `-p` — so the recipe path is deliberately left unset
// rather than guessed.
func projectLayoutAt(root string) project.Layout {
	return project.Layout{Root: root, StateDir: filepath.Join(root, project.StateDirName)}
}

// ShareProjectStores binds other to the stores this App holds, so the two Apps
// reach one *projectdb.DB — and therefore one connection pool — per project.
//
// It is the exported form of what a converge worker gets by construction
// (convergeWorker pre-seeds the parent's holder), and it exists for the same
// reason. An embedding host that cannot live with a single App — the desktop
// builds a fresh one per run so per-run state such as TargetLang stays owned —
// would otherwise open `.kapi/store.db` once per App. The FIFO write gate is
// per pool: two pools on one file cannot order their writers against each
// other, and the tab-side working-set writes a review loop makes would go back
// to losing to a run's content-memory writes on SQLite's busy backoff.
//
// Ownership does not move. Shutdown on the RECEIVER closes the stores; the
// borrower must not be Shut down, exactly as a converge worker must not be,
// since it would close handles the owner is still handing out. Call this before
// the borrower opens anything — a borrower that has already built its own
// holder keeps it.
func (a *App) ShareProjectStores(other *App) {
	if other == nil || other == a {
		return
	}
	other.projectStores = a.ensureProjectStores()
}

// CloseProjectDB closes and forgets the store this App holds for one project,
// so the next ProjectDB for that root opens a fresh handle.
//
// Shutdown is the ordinary way to release stores; this is for a host whose
// projects come and go inside one process. The desktop closes a tab, or resets
// a sample project by renaming its directory out from under itself — and an
// open handle would keep a file descriptor on the moved file and then hand that
// stale handle to the reopened tab.
//
// It is the caller's job to know nothing is using the store: a run in flight
// holds subsystem handles whose pool this closes. An unknown root is not an
// error.
func (a *App) CloseProjectDB(root string) error {
	if root == "" {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("project store: resolve root %q: %w", root, err)
	}
	s := a.projectStores
	if s == nil {
		return nil
	}
	s.mu.Lock()
	db, ok := s.dbs[abs]
	delete(s.dbs, abs)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	return db.Close()
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
