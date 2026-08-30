package backend

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host"
)

// Store ownership in the desktop.
//
// A project keeps one local store, `.kapi/work/store.db`, holding its content
// memory, terms, block cache and unit working set. The rule the whole
// arrangement rests on is that ONE connection pool per store file per process
// exists: the in-process write gate that stops a converge run's content-memory
// writes from starving the review loop's working-set writes orders writers
// within a pool, and can do nothing across two.
//
// The desktop cannot meet that with a single host.App — TargetLang and the
// per-run tool slots are single-occupancy, so a run needs its own — and it
// cannot meet it by having each surface open the store itself, which is what a
// tab-cached block store and a per-tab content-memory pool amounted to. So one
// App owns the stores and every other borrows them:
//
//	hostEngine()          the owner. Derives convergence reports, records review
//	                      decisions, and holds every open project store.
//	borrowEngine(app)     any App built for one run or one job. It gets its own
//	                      per-run state and the owner's stores.
//	projectStore(op)      the tab-side reach into a project's store.
//
// A borrower must never be Shut down: its stores belong to the owner, and
// shutdownEngine closes them once, on service teardown.

// hostEngine returns the desktop's shared host.App, building it on first use so
// its format and tool registries register once rather than per request. It is
// the owner of every project store this process holds, and of the discovered
// plugins.
func (a *App) hostEngine() *host.App {
	a.engineMu.Lock()
	defer a.engineMu.Unlock()
	if a.engine == nil {
		e := &host.App{}
		e.InitRegistries()
		// Discovery is what lets a plugin contribute a command the desktop
		// dispatches — the venue plumbing behind "Bring up to date" in a
		// connected project, the same route `kapi up` takes. Once, on the
		// owner: a rescan per run would pay the discovery cost every time and
		// could hand two runs different plugin sets.
		e.InitPluginHost()
		a.engine = e
	}
	return a.engine
}

// borrowEngine binds a run-scoped host.App to the engine's project stores and
// discovered plugins, and returns it, so a run reads and writes the same
// handles the tab surfaces do rather than opening a second pool on the
// project's store file, and resolves a command route from the same plugins.
//
// The caller keeps ownership of everything else on the App it passed —
// registries, credentials, the config preprocessor — and must not Shut it down.
func (a *App) borrowEngine(run *host.App) *host.App {
	owner := a.hostEngine()
	owner.ShareProjectStores(run)
	run.PluginHost = owner.PluginHost
	return run
}

// shutdownEngine releases the project stores, at service teardown. It is the
// only place that may: every other host.App the desktop builds borrows these
// handles, so a Shutdown anywhere else would close a store still being handed
// out. A desktop that never built an engine has nothing to release.
func (a *App) shutdownEngine() {
	a.engineMu.Lock()
	engine := a.engine
	a.engine = nil
	a.engineMu.Unlock()
	if engine != nil {
		engine.Shutdown()
	}
}

// projectStore returns the open store for a tab's project: the one handle over
// its content memory, terms, block cache and working set. Opening it creates it
// if absent, so callers that must not write to a project they are only looking
// at (autoOpenProjectResources) check for the file first.
func (a *App) projectStore(op *openProject) (*projectdb.DB, error) {
	root, ok := projectRoot(op)
	if !ok {
		return nil, errors.New("project has no file path; save it first")
	}
	return a.hostEngine().ProjectDB(context.Background(), root)
}
