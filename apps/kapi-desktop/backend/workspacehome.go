package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// The app's first screen reads the workspace.
//
// A project registers in the workspace the first time kapi runs in it, from
// whichever surface ran: a terminal, an agent's MCP server, this app. So the
// list here is every project this machine account works on, and a repository
// set up from a terminal a minute ago is on it without anyone telling the app.
//
// The workspace is also how the app learns that something changed while it was
// open. Every registration appends to the workspace's operation log, and the
// log's head is one number this process reads on a timer (workspaceWatcher).
// There is no channel to another kapi process and no service in the middle.

// workspaceHomeTimeout caps one read of the workspace. The home screen is
// interactive, and a registry that has not answered in this long is better
// reported than waited on.
const workspaceHomeTimeout = 15 * time.Second

// WorkspaceCheckout is one directory a project has been seen at on this
// machine.
type WorkspaceCheckout struct {
	// Path is the directory.
	Path string `json:"path"`
	// Recipe is the recipe file inside it. The frontend opens the project by
	// this path.
	Recipe string `json:"recipe"`
	// Missing reports that no recipe is readable at Recipe any more, because
	// the directory was moved, deleted, or is on a volume that is not mounted.
	// The checkout stays in the list either way: the project and its context
	// are unaffected, and dropping the row loses the only record of where it
	// was.
	Missing bool `json:"missing"`
}

// WorkspaceProject is one project the workspace registry holds.
type WorkspaceProject struct {
	// Key is the project's identity in the workspace: the recipe's `id:`, or
	// its `name:` where the recipe carries no id.
	Key string `json:"key"`
	// Name is what to call the project on screen.
	Name string `json:"name"`
	// LastActive is when kapi last ran in this project, RFC3339 in UTC.
	LastActive string `json:"last_active"`
	// Checkouts are the places this project has been seen on this machine,
	// which is empty for a project only ever opened elsewhere. Several
	// checkouts of one repository (a second clone, a git worktree) are several
	// rows under one project.
	Checkouts []WorkspaceCheckout `json:"checkouts"`
	// ContextFiles names the context files a checkout carries when this
	// project's store has never held its context, and the command that reads
	// them. The first readable checkout answers, since the rest carry their
	// own branch's copy of the same files.
	ContextFiles *ContextFilesNoticeDTO `json:"context_files,omitempty"`
}

// WorkspaceHome is the first screen: where this machine account's workspace is
// and which projects it holds.
type WorkspaceHome struct {
	// Location is the workspace directory, for a screen that has to say where
	// the context it lists is kept.
	Location string `json:"location"`
	// ReadOnly reports a workspace that answers reads and refuses writes.
	ReadOnly bool `json:"read_only"`
	// Projects are the registered projects, most recently active first.
	Projects []WorkspaceProject `json:"projects"`
}

// WorkspaceRemoval is what removing a project from the workspace will delete.
// The frontend names each part in the confirmation, so nobody agrees to a
// removal without reading what goes.
type WorkspaceRemoval struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Store is the file holding this project's terms, voice profiles, content
	// memory and recorded decisions. It is deleted.
	Store string `json:"store"`
	// Checkouts are the directories the project has been seen at. Their files
	// are untouched, and running kapi in one registers the project again with
	// an empty context.
	Checkouts []string `json:"checkouts"`
}

// ListWorkspaceProjects returns the workspace and the projects it holds.
func (a *App) ListWorkspaceProjects() (*WorkspaceHome, error) {
	ctx, cancel := context.WithTimeout(context.Background(), workspaceHomeTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	desc := ws.Describe()
	home := &WorkspaceHome{
		Location: desc.Location,
		ReadOnly: desc.ReadOnly,
		Projects: []WorkspaceProject{},
	}
	regs, err := ws.Projects(ctx)
	if err != nil {
		return nil, err
	}
	for _, reg := range regs {
		row := workspaceProjectOf(reg)
		for _, checkout := range row.Checkouts {
			if checkout.Missing {
				continue
			}
			if notice := a.contextFilesNotice(checkout.Recipe); notice != nil {
				row.ContextFiles = notice
			}
			break
		}
		home.Projects = append(home.Projects, row)
	}
	return home, nil
}

// workspaceProjectOf renders one registration for the home screen, stamping
// each checkout with whether a recipe is still readable there.
func workspaceProjectOf(reg workspace.Registration) WorkspaceProject {
	out := WorkspaceProject{
		Key:       string(reg.Key),
		Name:      workspaceDisplayName(reg),
		Checkouts: []WorkspaceCheckout{},
	}
	if !reg.LastActive.IsZero() {
		out.LastActive = reg.LastActive.UTC().Format(time.RFC3339)
	}
	for _, dir := range reg.Checkouts {
		recipe := filepath.Join(dir, project.RecipeFileName)
		available, _ := recipeStatus(recipe)
		out.Checkouts = append(out.Checkouts, WorkspaceCheckout{
			Path:    dir,
			Recipe:  recipe,
			Missing: !available,
		})
	}
	return out
}

// workspaceDisplayName is what to call a project on screen: the name its recipe
// carries, the folder it was last seen in, or its key.
//
// A recipe may carry no `name:`, and a key derived from a checkout is a digest
// that reads as nothing. Falling through to the folder gives that project the
// label a person recognises.
func workspaceDisplayName(reg workspace.Registration) string {
	if reg.Name != "" {
		return reg.Name
	}
	if len(reg.Checkouts) > 0 {
		return filepath.Base(reg.Checkouts[len(reg.Checkouts)-1])
	}
	return string(reg.Key)
}

// WorkspaceRemovalFor describes what removing one project will delete, for the
// confirmation the user reads before asking for it.
func (a *App) WorkspaceRemovalFor(key string) (*WorkspaceRemoval, error) {
	if key == "" {
		return nil, errors.New("name the project to remove")
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceHomeTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	reg, ok, err := ws.Lookup(ctx, workspace.ProjectKey(key))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("this workspace holds no project %q", key)
	}
	desc := ws.Describe()
	out := &WorkspaceRemoval{
		Key:       key,
		Name:      workspaceDisplayName(reg),
		Checkouts: reg.Checkouts,
	}
	if out.Checkouts == nil {
		out.Checkouts = []string{}
	}
	if desc.Kind == workspace.KindLocal {
		out.Store = filepath.Join(desc.Location, workspace.ProjectsDirName,
			workspace.FileNameFor(workspace.ProjectKey(key))+".db")
	}
	return out, nil
}

// ForgetWorkspaceProject removes a project from the workspace: its
// registration, the checkouts recorded against it, and the context store
// holding its terms, voice profiles, content memory and recorded decisions.
//
// Nothing else calls it. Closing a tab, deleting a folder and resetting a
// sample all leave the project registered, because a project whose context
// disappeared as a side effect of something else is the loss this separation
// exists to prevent.
func (a *App) ForgetWorkspaceProject(key string) error {
	if key == "" {
		return errors.New("name the project to remove")
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceHomeTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return err
	}
	// A tab on this project is holding its store open, and the store cannot be
	// deleted under it.
	a.closeTabsForProject(workspace.ProjectKey(key))
	if err := ws.Forget(ctx, workspace.ProjectKey(key)); err != nil {
		return err
	}
	a.emitEvent("workspace:changed", nil)
	return nil
}

// closeTabsForProject closes every tab holding the named project, releasing the
// handles it borrows from the workspace.
func (a *App) closeTabsForProject(key workspace.ProjectKey) {
	a.mu.Lock()
	var closing []*openProject
	for id, op := range a.projects {
		if op.workspaceKey != key {
			continue
		}
		delete(a.projects, id)
		closing = append(closing, op)
	}
	a.mu.Unlock()
	for _, op := range closing {
		a.releaseProjectResources(op)
		if root, ok := projectRoot(op); ok {
			if err := a.hostEngine().CloseProjectDB(root); err != nil {
				a.logger.Printf("release project store for %s: %v", root, err)
			}
		}
		a.emitEvent("tab:closed", op.ID)
	}
}

// OpenWorkspaceContext opens a project the workspace holds and no checkout on
// this machine carries: a tab over its context store alone.
//
// The context is fully readable there. The content is not, because reading a
// project's files needs the files, so the tab carries no recipe and the
// surfaces that parse content stay out of it. Cloning the repository anywhere
// and running kapi once reunites the two: the checkout registers against the
// same key and the context is already in place.
func (a *App) OpenWorkspaceContext(key string) (*TabInfo, error) {
	if key == "" {
		return nil, errors.New("name the project to open")
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceHomeTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	pk := workspace.ProjectKey(key)
	reg, ok, err := ws.Lookup(ctx, pk)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("this workspace holds no project %q", key)
	}

	a.mu.RLock()
	for _, op := range a.projects {
		if op.contextOnly && op.workspaceKey == pk {
			a.mu.RUnlock()
			return &TabInfo{ID: op.ID, Name: op.contextName, ContextOnly: true}, nil
		}
	}
	a.mu.RUnlock()

	db, err := ws.Context(ctx, pk)
	if err != nil {
		return nil, err
	}
	op := &openProject{
		ID:           id.New(),
		workspaceKey: pk,
		contextOnly:  true,
		contextName:  workspaceDisplayName(reg),
	}
	// The workspace owns the pool, so both handles are borrowed: closing the
	// tab drops the ids and leaves the store to the workspace.
	if tb, terr := terms.NewSQLiteStoreFromDB(db); terr == nil {
		op.tbHandle = a.tbHandles.Adopt(tb)
	} else {
		a.logger.Printf("bind the terms of %s: %v", key, terr)
	}
	if tm, merr := memory.NewSQLiteStoreFromDB(db); merr == nil {
		op.memoryHandle = a.memoryHandles.Adopt(tm)
	} else {
		a.logger.Printf("bind the content memory of %s: %v", key, merr)
	}

	a.mu.Lock()
	a.projects[op.ID] = op
	a.mu.Unlock()
	return &TabInfo{ID: op.ID, Name: op.contextName, ContextOnly: true}, nil
}

// workspaceWatcher keeps the home screen current while other processes work.
//
// A CLI run, an agent's MCP server and this app all register into one
// workspace, and each registration appends to its operation log. The watcher
// reads the log's head on a timer and emits `workspace:changed` when it has
// moved, which the frontend turns into a refetch.
//
// Polling rather than watching the files: a SQLite database in WAL mode changes
// three files in an order a filesystem event says nothing useful about, and the
// head is one indexed integer read. A poll costs a single-row query per second
// against an open pool, so an idle app does one query a second and no work
// beyond it. There is no IPC with the other processes and no background
// service; the store is where they meet.
type workspaceWatcher struct {
	app      *App
	interval time.Duration

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}

	mu   sync.Mutex
	head int64
}

// workspaceWatchInterval is how often the head is read. A second is under the
// delay a person reads as "it noticed", and slow enough that the query is
// nothing next to an idle UI process.
const workspaceWatchInterval = time.Second

func newWorkspaceWatcher(app *App, interval time.Duration) *workspaceWatcher {
	if interval <= 0 {
		interval = workspaceWatchInterval
	}
	return &workspaceWatcher{
		app:      app,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start runs the watcher until Stop, or until ctx is done.
func (w *workspaceWatcher) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *workspaceWatcher) run(ctx context.Context) {
	defer close(w.done)
	// The first read establishes where the log stands, so the app does not
	// announce a change for everything recorded before it started.
	w.poll(ctx, false)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll(ctx, true)
		}
	}
}

// poll reads the log head and reports whether it moved. It emits only when
// announce is set, so the first read arms the watcher silently.
func (w *workspaceWatcher) poll(parent context.Context, announce bool) bool {
	ctx, cancel := context.WithTimeout(parent, workspaceHomeTimeout)
	defer cancel()

	ws, err := w.app.hostEngine().Workspace(ctx)
	if err != nil {
		// A workspace that will not open will not open on the next tick
		// either; the home screen reports the failure from its own read.
		return false
	}
	head, err := ws.Head(ctx)
	if err != nil {
		return false
	}
	w.mu.Lock()
	moved := head != w.head
	w.head = head
	w.mu.Unlock()
	if moved && announce {
		w.app.emitEvent("workspace:changed", nil)
	}
	return moved
}

// Stop ends the watcher and waits for it to finish.
func (w *workspaceWatcher) Stop() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// recipeStatus reports whether a recipe path still resolves to a file on disk,
// and when it does not, which shape of loss it is. The two cases read very
// differently to a person: a folder that vanished was moved or removed
// wholesale, a folder that survived without its recipe lost one file.
func recipeStatus(path string) (available bool, reason string) {
	if path == "" {
		return false, recipeGoneMoved
	}
	if _, err := os.Stat(path); err == nil {
		return true, ""
	}
	if fi, err := os.Stat(filepath.Dir(path)); err == nil && fi.IsDir() {
		return false, recipeGoneDeleted
	}
	return false, recipeGoneMoved
}

// The shapes of a recipe that cannot be read.
const (
	// recipeGoneMoved: the project folder itself is gone from that location.
	recipeGoneMoved = "moved"
	// recipeGoneDeleted: the folder is still there, the recipe inside it is not.
	recipeGoneDeleted = "deleted"
)

// recipeGoneError describes a recipe that cannot be opened, in the terms of
// whichever loss recipeStatus found.
func recipeGoneError(path, reason string) error {
	dir := filepath.Dir(path)
	if reason == recipeGoneDeleted {
		return fmt.Errorf("%s is missing from %s", filepath.Base(path), dir)
	}
	return fmt.Errorf("the project folder %s no longer exists", dir)
}

// workspaceKeyFor is the key a checkout registers under, derived the way the
// host derives it so a tab and the registry agree on which project this is: the
// recipe's stable `id:`, its `name:` where it carries no usable id, and a
// digest of the checkout for a recipe that states neither.
func workspaceKeyFor(proj *project.KapiProject, root string) workspace.ProjectKey {
	if proj != nil {
		if proj.ID != "" && project.ValidateID(proj.ID) == nil {
			return workspace.ProjectKey(proj.ID)
		}
		if proj.Name != "" {
			return workspace.ProjectKey(proj.Name)
		}
	}
	return workspace.KeyForCheckout(host.NormalizeCheckoutPath(root))
}

// registerInWorkspace records that a checkout was opened here, so a project
// opened through "Open folder" appears on the home from then on.
//
// It writes to the workspace and nothing else. Opening a project in the app is
// not a reason to create a store inside somebody's checkout, which is what
// registering through the project store would do.
func (a *App) registerInWorkspace(op *openProject) {
	root, ok := projectRoot(op)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), workspaceHomeTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		a.logger.Printf("open the workspace: %v", err)
		return
	}
	var name string
	if op.Project != nil {
		name = op.Project.Name
	}
	if _, err := ws.Register(ctx, op.workspaceKey, name, host.NormalizeCheckoutPath(root)); err != nil {
		if !errors.Is(err, workspace.ErrReadOnly) {
			a.logger.Printf("register %s in the workspace: %v", op.workspaceKey, err)
		}
		return
	}
	a.emitEvent("workspace:changed", nil)
}
