package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/venue/schema"
)

// Where a project's context reaches.
//
// The context lives in this machine account's workspace, which every clone and
// worktree of the project on this machine reads. A recipe can name a place to
// share it through under `context:` (a git ref, a folder, an S3 bucket), and
// `kapi share` pulls and pushes through it. A `bowrain:` block connects the
// project to a Bowrain server. The Share page reads all three here and writes
// the `context:` block, through the same recipe save every other settings
// edit takes.

// sharingTimeout caps one read of a project's sharing, and syncTimeout one
// sync, which reaches a remote and may wait on the network.
const (
	sharingTimeout = 15 * time.Second
	syncTimeout    = 2 * time.Minute
)

// errNoCheckout refuses a sharing edit or a sync in a tab with no recipe.
var errNoCheckout = errors.New("this tab holds the project's context and no checkout of it; open the project's folder to name where its context is shared")

// projectSharingAt reads where the project at recipe shares its context. A
// recipe whose `context:` block cannot be read reports the workspace alone,
// which is where its context stays until the block is fixed.
func (a *App) projectSharingAt(ctx context.Context, recipe string, proj *project.KapiProject, workspaceDir string) *ProjectSharing {
	out := &ProjectSharing{Kind: project.ContextBackendLocal, From: "default", Workspace: workspaceDir}
	if recipe != "" {
		if info, err := host.ResolveContextBackend(recipe); err == nil {
			out.Kind, out.Location, out.From = info.Kind, info.Location, info.From
			if info.Kind != project.ContextBackendLocal {
				out.Status = a.hostEngine().ContextSyncStatus(ctx, filepath.Dir(recipe))
			}
		}
	}
	if proj != nil {
		if venue, ok := proj.Venue(); ok && venue.URL != "" {
			out.VenueURL = venue.URL
			out.VenueHost = venueHost(venue.URL)
		}
	}
	return out
}

// venueHost is the server host a project URL names, without its scheme.
func venueHost(url string) string {
	server := schema.ParseProjectURL(url).ServerURL
	if i := strings.Index(server, "://"); i >= 0 {
		server = server[i+3:]
	}
	return server
}

// workspaceLocation is the workspace directory on this machine.
func (a *App) workspaceLocation(ctx context.Context) (string, error) {
	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return "", err
	}
	return ws.Describe().Location, nil
}

// ShareView is the Share page: the rung a project's rules reach, the place
// they are shared through and how far apart this machine and that place were
// at the last sync. Rung is host.ShareRungMachine, ShareRungTeam or
// ShareRungBowrain, read the way `kapi share --status` reads it.
type ShareView struct {
	Rung    string          `json:"rung"`
	Sharing *ProjectSharing `json:"sharing"`
	// Rules counts the rules in force at the project's own point, the count
	// the Map shows. Nil when they could not be counted.
	Rules *int `json:"rules,omitempty"`
	// BowrainPlugin reports whether the kapi-bowrain plugin is installed,
	// which connecting a project to a Bowrain server needs.
	BowrainPlugin bool `json:"bowrain_plugin"`
	// Error is what reading the recipe's place reported, such as a
	// `context:` block the recipe check refuses. The page then stands on the
	// first rung, where the context stays until the block is fixed.
	Error string `json:"error,omitempty"`
}

// ShareView reads the Share page for a tab through the host's `kapi share
// --status`: the rung, the place, the counts and the last sync, without
// reaching the place. A tab holding the context alone stands on the first
// rung, since sharing is named in a recipe and that tab has none. A recipe
// whose place cannot be read stands there too, with the error beside it.
func (a *App) ShareView(tabID string) (*ShareView, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), sharingTimeout)
	defer cancel()
	location, err := a.workspaceLocation(ctx)
	if err != nil {
		return nil, err
	}
	a.mu.RLock()
	path := op.Path
	a.mu.RUnlock()
	out := &ShareView{
		Rung:    host.ShareRungMachine,
		Sharing: &ProjectSharing{Kind: project.ContextBackendLocal, From: "default", Workspace: location},
	}
	if op.contextOnly || path == "" {
		return out, nil
	}
	res, err := a.hostEngine().Share(ctx, host.ShareRequest{Project: path, Status: true})
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	out.Rung, out.BowrainPlugin = res.Rung, res.BowrainPlugin
	if b := res.Backend; b != nil {
		out.Sharing.Kind, out.Sharing.Location, out.Sharing.From = b.Kind, b.Location, b.From
	}
	out.Sharing.Status = res.Status
	// A `bowrain:` block without a URL still connects the project, and the
	// host then names the server in words, which are no URL to open.
	if strings.Contains(res.Server, "://") {
		out.Sharing.VenueURL = res.Server
		out.Sharing.VenueHost = venueHost(res.Server)
	}
	if n, err := a.mapRuleCount(ctx, op); err == nil {
		out.Rules = &n
	} else {
		a.logger.Printf("count the rules of %s: %v", path, err)
	}
	return out, nil
}

// SetProjectSharing writes the place a tab's project shares its context
// through into the `context:` block of its recipe, and reports the sharing as
// it then stands. A local spec removes the block, since keeping the context on
// this machine is what a recipe without one does. A spec the recipe would
// refuse is refused here with the words the recipe check uses, and nothing is
// written.
func (a *App) SetProjectSharing(tabID string, spec project.ContextBackend) (*ProjectSharing, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	op := a.projects[tabID]
	if op == nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.contextOnly || op.Project == nil || op.Path == "" {
		a.mu.Unlock()
		return nil, errNoCheckout
	}
	if spec.Shared() {
		block := spec
		op.Project.Context = &block
	} else {
		op.Project.Context = nil
	}
	err := a.saveRecipe(op)
	path, proj := op.Path, op.Project
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), sharingTimeout)
	defer cancel()
	location, err := a.workspaceLocation(ctx)
	if err != nil {
		return nil, err
	}
	// The home lists every project's sharing, and a recipe edit moves no
	// workspace log head for its watcher to see.
	a.emitEvent("workspace:changed", nil)
	return a.projectSharingAt(ctx, path, proj, location), nil
}

// SyncProjectContext pulls what other machines shared through the place a
// tab's recipe names, and pushes what this machine recorded: the sync `kapi
// share` and `kapi context sync` run. A place that cannot be reached returns
// the host's error as it is, with nothing changed here.
func (a *App) SyncProjectContext(tabID string) (*host.ContextSyncResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	a.mu.RLock()
	path := op.Path
	a.mu.RUnlock()
	if op.contextOnly || path == "" {
		return nil, errNoCheckout
	}
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	res, err := a.hostEngine().SyncProjectContext(ctx, host.ContextSyncRequest{Project: path})
	if err != nil {
		return nil, err
	}
	if syncMoved(res) {
		a.emitEvent("workspace:changed", nil)
	}
	return &res, nil
}

// syncMoved reports a sync that merged or pushed something, which changes
// what the home lists: the merged operations, and how far apart this machine
// and the place are.
func syncMoved(res host.ContextSyncResult) bool {
	return (res.Pull != nil && res.Pull.Merged > 0) || (res.Push != nil && res.Push.Pushed > 0)
}
