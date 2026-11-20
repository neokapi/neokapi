package backend

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host"
)

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
		// The person at the keyboard is the one acting, whichever shell
		// launched the app: a terminal inside an agent host leaves that
		// host's marker in the environment, and read from there the
		// person's imports and decisions would be refused as an agent's.
		e.ActAs(contextop.Actor{Kind: contextop.ActorPerson})
		a.engine = e
	}
	return a.engine
}

// newProjectEngine gives one operation its own locale, encoding, and parse cache.
// The desktop owns its registries, credentials, and store handles.
func (a *App) newProjectEngine(op *openProject) *host.App {
	owner := a.hostEngine()
	run := &host.App{
		FormatReg:   a.formatReg,
		ToolReg:     a.toolReg,
		SchemaReg:   a.schemaReg,
		Config:      a.aiConfig,
		Credentials: a.credentials,
		PluginHost:  owner.PluginHost,
		Quiet:       true,
	}
	if op != nil && op.Project != nil {
		run.ResolveSourceLang(op.Project.Defaults.SourceLanguage)
		run.ResolveEncoding(op.Project.Defaults.Encoding)
	}
	owner.ShareProjectStores(run)
	run.ActAs(contextop.Actor{Kind: contextop.ActorPerson})
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
