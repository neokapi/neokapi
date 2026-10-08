package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

// A project's context is shared through the backend its recipe declares under
// `context:` (core/project.ContextBackend). Sync is explicit: `kapi context
// sync` merges what other machines pushed and then writes what this one
// recorded, and every answer that reads the context says how far apart the two
// were at the last contact.

// RemoteOpener builds the remote for one backend kind. The S3 adapter
// registers itself this way, so this package does not link its client.
type RemoteOpener func(ctx context.Context, spec project.ContextBackend) (workspace.Remote, error)

var (
	remoteOpenersMu sync.RWMutex
	remoteOpeners   = map[string]RemoteOpener{}
)

// RegisterContextRemote makes a backend kind available to every project.
func RegisterContextRemote(kind string, open RemoteOpener) {
	remoteOpenersMu.Lock()
	defer remoteOpenersMu.Unlock()
	remoteOpeners[kind] = open
}

// HasContextRemote reports whether this build can open a backend of the kind:
// local, file and git always, any other kind once a package registered it.
func HasContextRemote(kind string) bool {
	switch kind {
	case project.ContextBackendLocal, project.ContextBackendFile, project.ContextBackendGit:
		return true
	}
	remoteOpenersMu.RLock()
	defer remoteOpenersMu.RUnlock()
	return remoteOpeners[kind] != nil
}

// ContextBackendInfo says which backend a project's context is shared
// through, and where that choice was made.
type ContextBackendInfo struct {
	// Kind is local, file, git or s3.
	Kind string `json:"kind"`
	// Location is where a shared backend keeps the context.
	Location string `json:"location,omitempty"`
	// From is "recipe" when kapi.yaml declares the backend, "default" when it
	// declares none and the context stays on this machine.
	From string `json:"from"`
	// Status is how far apart this machine and the backend were at the last
	// contact. Empty for a local backend.
	Status *workspace.SyncStatus `json:"status,omitempty"`

	spec project.ContextBackend
}

// FormatText renders the backend for a reader.
func (b ContextBackendInfo) FormatText(w io.Writer) error {
	where := b.Kind
	if b.Location != "" {
		where += " (" + b.Location + ")"
	}
	switch b.From {
	case "recipe":
		fmt.Fprintf(w, "Context backend: %s, declared in the recipe.\n", where)
	default:
		fmt.Fprintln(w, "Context backend: local. The context stays on this machine; declare context.backend in kapi.yaml to share it.")
	}
	if b.Status != nil {
		fmt.Fprintln(w, SyncLine(b.Status))
	}
	return nil
}

// SyncLine renders how far apart this machine and the backend are, the way
// every answer that reads the context reports it.
func SyncLine(st *workspace.SyncStatus) string {
	if st == nil {
		return ""
	}
	line := fmt.Sprintf("Context: %d to push%s", st.ToPush, pushedSplit(st.ToPush, st.ToPushLogged))
	if st.Contacted.IsZero() {
		// Nothing has been read from the backend, so what waits there is not
		// known: saying 0 would claim it is empty.
		return line + "; what waits to pull is not known until the first pull (run `kapi context sync`)."
	}
	line += fmt.Sprintf(", %d to pull", st.ToPull)
	switch {
	case st.Error != "":
		line += fmt.Sprintf(" (the backend could not be reached at %s)", st.Contacted.Local().Format("2006-01-02 15:04"))
	default:
		line += fmt.Sprintf(" (as of %s)", st.Contacted.Local().Format("2006-01-02 15:04"))
	}
	return line + "."
}

// pushedSplit says how many of the operations a push counts `kapi context log`
// lists, when not all of them: the rest are the writes to the project's stores
// that keeping a rule or importing a voice or terms made, which travel with
// the context and are not context operations of their own.
func pushedSplit(total, logged int) string {
	if total == 0 || logged == total {
		return ""
	}
	return fmt.Sprintf(" (%d in `kapi context log`, %s)", logged,
		pluralUnit(total-logged, "store write", "store writes"))
}

// recipeContextBackend reads the recipe's `context:` block and nothing else.
func recipeContextBackend(recipePath string) (*project.ContextBackend, error) {
	data, err := os.ReadFile(recipePath)
	if err != nil {
		return nil, err
	}
	var head struct {
		Context *project.ContextBackend `yaml:"context"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("%s: %w", recipePath, err)
	}
	if err := head.Context.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", recipePath, err)
	}
	return head.Context, nil
}

// ContextBackend reports which backend the project's context is shared
// through, and how far apart this machine and it were at the last contact.
func (a *App) ContextBackend(ctx context.Context, projectPath string) (ContextBackendInfo, error) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return ContextBackendInfo{}, err
	}
	w, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return ContextBackendInfo{}, err
	}
	info, err := resolveBackend(layout)
	if err != nil {
		return info, err
	}
	if s, err := a.contextSync(ctx, layout, info, w); err == nil && s != nil {
		st, err := s.Status(ctx)
		if err == nil {
			info.Status = &st
		}
	}
	return info, nil
}

// resolveBackend reads the backend the recipe declares.
func resolveBackend(layout project.Layout) (ContextBackendInfo, error) {
	declared, err := recipeContextBackend(layout.RecipePath)
	if err != nil {
		return ContextBackendInfo{}, err
	}
	info := ContextBackendInfo{From: "default"}
	spec := project.ContextBackend{}
	if declared != nil {
		spec, info.From = *declared, "recipe"
	}
	info.Kind, info.spec = spec.Kind(), spec
	if spec.Kind() == project.ContextBackendFile && !filepath.IsAbs(spec.Path) {
		info.spec.Path = filepath.Join(layout.Root, spec.Path)
	}
	switch spec.Kind() {
	case project.ContextBackendFile:
		info.Location = info.spec.Path
	case project.ContextBackendGit:
		info.Location = workspace.NewGitRemote("", spec.Remote, spec.Ref).Describe().Location
	case project.ContextBackendS3:
		info.Location = "s3://" + spec.Bucket + "/" + spec.Prefix
	}
	return info, nil
}

// openRemote builds the remote a backend names.
func openRemote(ctx context.Context, layout project.Layout, info ContextBackendInfo) (workspace.Remote, error) {
	spec := info.spec
	switch spec.Kind() {
	case project.ContextBackendLocal:
		return nil, nil
	case project.ContextBackendFile:
		return workspace.NewFileRemote(spec.Path), nil
	case project.ContextBackendGit:
		repo, ok := workspace.GitRepoRoot(ctx, layout.Root)
		if !ok {
			return nil, fmt.Errorf("the recipe keeps the context on a git ref, and %s is not in a git repository", layout.Root)
		}
		return workspace.NewGitRemote(repo, spec.Remote, spec.Ref), nil
	}
	remoteOpenersMu.RLock()
	open := remoteOpeners[spec.Kind()]
	remoteOpenersMu.RUnlock()
	if open == nil {
		return nil, fmt.Errorf("this build of kapi has no %s backend", spec.Kind())
	}
	return open(ctx, spec)
}

// contextSync links the project to its backend; nil for a local one.
func (a *App) contextSync(ctx context.Context, layout project.Layout, info ContextBackendInfo, w *projector.Projector) (*workspace.Sync, error) {
	if !w.Logged() || info.Kind == project.ContextBackendLocal {
		return nil, nil
	}
	remote, err := openRemote(ctx, layout, info)
	if err != nil || remote == nil {
		return nil, err
	}
	ws, err := a.projectWorkspace(ctx, layout.Root)
	if err != nil {
		return nil, err
	}
	return ws.NewSync(remote, w.Key(), w.Syncer(), workspace.SyncOptions{LocalKinds: projector.LocalKinds}), nil
}

// projectSync resolves the project's backend and links it, refusing a local
// one with the way to declare another.
func (a *App) projectSync(ctx context.Context, projectPath string) (*workspace.Sync, error) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return nil, err
	}
	w, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return nil, err
	}
	info, err := resolveBackend(layout)
	if err != nil {
		return nil, err
	}
	if info.Kind == project.ContextBackendLocal {
		return nil, errors.New("there is nowhere to sync with: the recipe declares no context backend. Declare one in kapi.yaml, for example `context: {backend: git}`")
	}
	s, err := a.contextSync(ctx, layout, info, w)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("this project's store has no workspace log to sync")
	}
	return s, nil
}

// ContextPull reports what a pull merged.
type ContextPull struct {
	workspace.PullReport
	Seconds float64 `json:"seconds"`
}

// FormatText renders a pull.
func (r ContextPull) FormatText(w io.Writer) error {
	switch {
	case r.Empty:
		fmt.Fprintf(w, "Nothing has been shared through %s yet.\n", describeRemote(r.Remote))
	case r.Merged == 0:
		fmt.Fprintf(w, "Up to date with %s.\n", describeRemote(r.Remote))
	default:
		from := ""
		if r.Checkpoint != "" {
			from = ", starting from checkpoint " + workspace.ShortOpID(r.Checkpoint)
		}
		fmt.Fprintf(w, "Pulled %s from %s%s in %.1fs.\n",
			pluralUnit(r.Merged, "operation", "operations"), describeRemote(r.Remote), from, r.Seconds)
	}
	if r.Error != "" {
		fmt.Fprintf(w, "  note: %s\n", r.Error)
	}
	fmt.Fprintln(w, SyncLine(&r.SyncStatus))
	return nil
}

// ContextPush reports what a push wrote.
type ContextPush struct {
	workspace.PushReport
	Seconds float64 `json:"seconds"`
}

// FormatText renders a push.
func (r ContextPush) FormatText(w io.Writer) error {
	if r.Pushed == 0 {
		fmt.Fprintf(w, "Nothing to push to %s.\n", describeRemote(r.Remote))
	} else {
		fmt.Fprintf(w, "Pushed %s%s to %s in %.1fs.\n",
			pluralUnit(r.Pushed, "operation", "operations"), pushedSplit(r.Pushed, r.PushedLogged),
			describeRemote(r.Remote), r.Seconds)
	}
	if r.Checkpoint != "" {
		fmt.Fprintf(w, "Wrote a checkpoint through %s for a fast first pull.\n", workspace.ShortOpID(r.Checkpoint))
	}
	fmt.Fprintln(w, SyncLine(&r.SyncStatus))
	return nil
}

func describeRemote(d workspace.RemoteDescriptor) string {
	return d.Kind + " " + d.Location
}

// PullProjectContext merges what other machines pushed to the project's
// backend and applies it to the stores.
func (a *App) PullProjectContext(ctx context.Context, projectPath string) (ContextPull, error) {
	s, err := a.projectSync(ctx, projectPath)
	if err != nil {
		return ContextPull{}, err
	}
	start := time.Now()
	report, err := s.Pull(ctx)
	res := ContextPull{PullReport: report, Seconds: time.Since(start).Seconds()}
	return res, syncError(err, "pull", res.ToPush)
}

// PushProjectContext writes the operations this machine recorded to the
// project's backend.
func (a *App) PushProjectContext(ctx context.Context, projectPath string) (ContextPush, error) {
	s, err := a.projectSync(ctx, projectPath)
	if err != nil {
		return ContextPush{}, err
	}
	start := time.Now()
	report, err := s.Push(ctx)
	res := ContextPush{PushReport: report, Seconds: time.Since(start).Seconds()}
	return res, syncError(err, "push", res.ToPush)
}

// ContextSync reports a pull and a push through one remote; each is nil when
// it was not asked for.
type ContextSync struct {
	Pull *ContextPull `json:"pull,omitempty"`
	Push *ContextPush `json:"push,omitempty"`
}

// SyncProjectContextWith pulls the project's context from a remote the
// caller holds, then pushes what this machine recorded to it, whatever
// backend the recipe declares. It is how the browser engine shares a
// project's context through a folder the page was given access to, which no
// recipe can name. What the workspace knows about the remote is kept, keyed
// by the remote's descriptor, so the next sync with it moves only what is
// new.
func (a *App) SyncProjectContextWith(ctx context.Context, projectPath string, remote workspace.Remote, pull, push bool) (ContextSync, error) {
	var res ContextSync
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	w, ws, err := a.projectLog(ctx, layout)
	if err != nil {
		return res, err
	}
	s := ws.NewSync(remote, w.Key(), w.Syncer(), workspace.SyncOptions{LocalKinds: projector.LocalKinds})
	if pull {
		start := time.Now()
		report, err := s.Pull(ctx)
		res.Pull = &ContextPull{PullReport: report, Seconds: time.Since(start).Seconds()}
		if err != nil {
			return res, syncError(err, "pull", res.Pull.ToPush)
		}
	}
	if push {
		start := time.Now()
		report, err := s.Push(ctx)
		res.Push = &ContextPush{PushReport: report, Seconds: time.Since(start).Seconds()}
		if err != nil {
			return res, syncError(err, "push", res.Push.ToPush)
		}
	}
	return res, nil
}

// syncError explains a failed pull or push and gives it its exit code.
func syncError(err error, verb string, queued int) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspace.ErrRemoteUnreachable):
		return WithExitCode(ExitUnreachable, fmt.Errorf(
			"%w\nNothing changed here at the %s. %s to be shared; run `kapi context sync` again when the backend is reachable",
			err, verb, pluralUnit(queued, "operation waits", "operations wait")))
	case errors.Is(err, workspace.ErrObjectExists):
		return fmt.Errorf("%w\nThe backend holds a file this machine was about to write with other bytes. "+
			"That happens when two machines share one kapi data directory; give each machine its own", err)
	}
	return err
}

// ContextSyncStatus reports how far apart this machine and the project's
// backend were at the last contact, without reaching the backend. It is nil
// for a project whose context stays on this machine, and for one whose
// backend cannot be resolved.
func (a *App) ContextSyncStatus(ctx context.Context, root string) *workspace.SyncStatus {
	layout, err := project.LayoutFor(root)
	if err != nil {
		return nil
	}
	w, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return nil
	}
	info, err := resolveBackend(layout)
	if err != nil || info.Kind == project.ContextBackendLocal {
		return nil
	}
	s, err := a.contextSync(ctx, layout, info, w)
	if err != nil || s == nil {
		return nil
	}
	st, err := s.Status(ctx)
	if err != nil {
		return nil
	}
	return &st
}

// ContextSyncRequest asks for one sync of a project's context with the backend
// its recipe declares.
type ContextSyncRequest struct {
	// Project is the recipe path.
	Project string
	// Merged, PR and Merger name a change that reached the default branch, as
	// ContextSettleRequest takes them. With Merged set, the sync also records
	// the evidence the change carries and establishes what it backs, between
	// the pull and the push.
	Merged string
	PR     int
	Merger string
	// NoPush reads what others shared and shares nothing back: a gate or a
	// fresh runner that only needs the context to answer from.
	NoPush bool
}

// ContextSyncResult reports what one sync did: the pull, the settling a merge
// asked for, and the push. Pull and Push are nil when the recipe declares no
// backend and the sync only settled a merge here.
type ContextSyncResult struct {
	Pull   *ContextPull         `json:"pull,omitempty"`
	Settle *ContextSettleResult `json:"settle,omitempty"`
	Push   *ContextPush         `json:"push,omitempty"`
}

// FormatText renders a sync in the order it ran.
func (r ContextSyncResult) FormatText(w io.Writer) error {
	if r.Pull != nil {
		if err := r.Pull.FormatText(w); err != nil {
			return err
		}
	}
	if r.Settle != nil {
		if err := r.Settle.FormatText(w); err != nil {
			return err
		}
	}
	if r.Push != nil {
		return r.Push.FormatText(w)
	}
	if r.Pull == nil && r.Settle != nil {
		_, err := fmt.Fprintln(w, "The recipe declares no context backend, so the context stays on this machine.")
		return err
	}
	return nil
}

// SyncProjectContext pulls what other machines shared through the project's
// backend, settles the merge the request names, and then pushes what this
// machine recorded. A pull that fails stops the sync before anything is
// pushed. A project whose recipe declares no backend has nothing to sync with:
// that is an error, unless the request names a merge, which is settled here.
// NoPush stops after the pull and the settling.
func (a *App) SyncProjectContext(ctx context.Context, req ContextSyncRequest) (ContextSyncResult, error) {
	var res ContextSyncResult
	if (req.PR != 0 || req.Merger != "") && req.Merged == "" {
		return res, errors.New("a pull request number and a merger describe a merge: name it with --merged")
	}
	s, err := a.projectSync(ctx, req.Project)
	if err != nil && req.Merged == "" {
		return res, err
	}
	if s != nil {
		start := time.Now()
		report, perr := s.Pull(ctx)
		res.Pull = &ContextPull{PullReport: report, Seconds: time.Since(start).Seconds()}
		if perr != nil {
			return res, syncError(perr, "pull", res.Pull.ToPush)
		}
	}
	if req.Merged != "" {
		settled, serr := a.SettleContext(ctx, ContextSettleRequest{
			Project: req.Project, Merged: req.Merged, PR: req.PR, Merger: req.Merger,
		})
		if serr != nil {
			return res, serr
		}
		res.Settle = &settled
	}
	if s != nil && !req.NoPush {
		start := time.Now()
		report, perr := s.Push(ctx)
		res.Push = &ContextPush{PushReport: report, Seconds: time.Since(start).Seconds()}
		if perr != nil {
			return res, syncError(perr, "push", res.Push.ToPush)
		}
	}
	return res, nil
}
