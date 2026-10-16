package host

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// The fold of the context log compares the points two rules hold at (C-11):
// whether they can disagree, and whether a correction backs a suggestion. A
// project's recipe types its axes, so the fold reads a rule for the Nordics and
// a rule for Norway as meeting only when it knows the recipe. Every reader of a
// workspace on this machine therefore builds its ledger here, and they all
// fold the log the same way.

// contextLedger is the workspace's ledger under the shared policy, with each
// project's axes typed from the recipe of a checkout this machine knows.
func (a *App) contextLedger(ctx context.Context, ws *workspace.Workspace) *contextop.Ledger {
	return contextop.NewLedger(ws, contextop.PersonDecides).WithAxes(a.projectAxes(ctx, ws, "", nil))
}

// contextLedgerAt is contextLedger for a session that has loaded one project's
// recipe already: that project's axes come from it, and the rest from the
// registry.
func (a *App) contextLedgerAt(ctx context.Context, ws *workspace.Workspace, key workspace.ProjectKey, proj *project.KapiProject) *contextop.Ledger {
	return contextop.NewLedger(ws, contextop.PersonDecides).WithAxes(a.projectAxes(ctx, ws, key, proj))
}

// projectAxes resolves a project's axis space from the first of its checkouts
// whose recipe loads. A project with no checkout here, or whose recipe types
// nothing, compares its coordinates exactly. The registry is read once per
// ledger, on the first project the fold asks about.
func (a *App) projectAxes(ctx context.Context, ws *workspace.Workspace, known workspace.ProjectKey, proj *project.KapiProject) func(workspace.ProjectKey) axis.Space {
	var (
		once      sync.Once
		checkouts map[workspace.ProjectKey][]string
	)
	return func(key workspace.ProjectKey) axis.Space {
		if known != "" && key == known {
			return proj.AxisSpace()
		}
		once.Do(func() {
			checkouts = map[workspace.ProjectKey][]string{}
			regs, err := ws.Projects(ctx)
			if err != nil {
				return
			}
			for _, r := range regs {
				checkouts[r.Key] = r.Checkouts
			}
		})
		for _, dir := range checkouts[key] {
			// A directory that now holds another project is passed over, as
			// registeredCheckout passes it over.
			recipe := project.LayoutAt(dir).RecipePath
			if identity, _ := recipeIdentity(recipe); identity != string(key) {
				continue
			}
			if space, ok := recipeAxes(recipe); ok {
				return space
			}
		}
		return nil
	}
}

// recipeAxesCache holds each recipe's axis space by path, valid while the
// recipe's modification time and size are unchanged.
var recipeAxesCache sync.Map // path -> recipeAxesEntry

type recipeAxesEntry struct {
	mod   time.Time
	size  int64
	space axis.Space
	ok    bool
}

// recipeAxes reads the axis space a recipe declares, and whether it loaded.
func recipeAxes(path string) (axis.Space, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, false
	}
	if v, hit := recipeAxesCache.Load(path); hit {
		e := v.(recipeAxesEntry)
		if e.mod.Equal(info.ModTime()) && e.size == info.Size() {
			return e.space, e.ok
		}
	}
	e := recipeAxesEntry{mod: info.ModTime(), size: info.Size()}
	if proj, lerr := project.Load(path); lerr == nil {
		e.space, e.ok = proj.AxisSpace(), true
	}
	recipeAxesCache.Store(path, e)
	return e.space, e.ok
}
