package host

import (
	"context"
	"encoding/json"
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
// `context:` (core/project.ContextBackend), or the one a person chose for this
// machine with `kapi context backend`. Sync is explicit: `kapi context pull`
// merges what other machines pushed, `kapi context push` writes what this one
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
	// From is "recipe", "machine" (this machine's override) or "default".
	From string `json:"from"`
	// Recipe is what the recipe declares, when the machine overrides it.
	Recipe string `json:"recipe,omitempty"`
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
	case "machine":
		fmt.Fprintf(w, "Context backend: %s, chosen on this machine", where)
		if b.Recipe != "" {
			fmt.Fprintf(w, "; the recipe declares %s", b.Recipe)
		}
		fmt.Fprintln(w, ".")
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
	line := fmt.Sprintf("Context: %d to push, %d to pull", st.ToPush, st.ToPull)
	switch {
	case st.Contacted.IsZero():
		line += " (never pulled; run `kapi context pull`)"
	case st.Error != "":
		line += fmt.Sprintf(" (the backend could not be reached at %s)", st.Contacted.Local().Format("2006-01-02 15:04"))
	default:
		line += fmt.Sprintf(" (as of %s)", st.Contacted.Local().Format("2006-01-02 15:04"))
	}
	return line + "."
}

// contextBackendOverrides is the machine configuration file that holds the
// backends a person chose on this machine, keyed by project identity.
func contextBackendOverrides() string {
	return filepath.Join(ConfigDir(), "context-backends.json")
}

func readBackendOverrides() (map[string]project.ContextBackend, error) {
	data, err := os.ReadFile(contextBackendOverrides())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]project.ContextBackend{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]project.ContextBackend{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", contextBackendOverrides(), err)
	}
	return out, nil
}

func writeBackendOverrides(m map[string]project.ContextBackend) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(contextBackendOverrides()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(contextBackendOverrides(), append(data, '\n'), 0o644)
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
	info, err := resolveBackend(layout, w.Key())
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

// resolveBackend combines the recipe's backend with this machine's choice.
func resolveBackend(layout project.Layout, key workspace.ProjectKey) (ContextBackendInfo, error) {
	declared, err := recipeContextBackend(layout.RecipePath)
	if err != nil {
		return ContextBackendInfo{}, err
	}
	overrides, err := readBackendOverrides()
	if err != nil {
		return ContextBackendInfo{}, err
	}
	info := ContextBackendInfo{From: "default"}
	spec := project.ContextBackend{}
	if declared != nil {
		spec, info.From = *declared, "recipe"
	}
	if o, ok := overrides[string(key)]; ok {
		if declared != nil {
			info.Recipe = declared.Kind()
		}
		spec, info.From = o, "machine"
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

// SetContextBackend records this machine's choice of backend for the project:
// "local" keeps its context here, "file" shares it through a directory, and
// "recipe" drops the choice so the recipe's backend applies again.
func (a *App) SetContextBackend(ctx context.Context, projectPath, kind, path string) (ContextBackendInfo, error) {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return ContextBackendInfo{}, err
	}
	w, err := a.Projector(ctx, layout.Root)
	if err != nil {
		return ContextBackendInfo{}, err
	}
	overrides, err := readBackendOverrides()
	if err != nil {
		return ContextBackendInfo{}, err
	}
	switch kind {
	case "recipe":
		delete(overrides, string(w.Key()))
	case project.ContextBackendLocal:
		overrides[string(w.Key())] = project.ContextBackend{Backend: project.ContextBackendLocal}
	case project.ContextBackendFile:
		if path == "" {
			return ContextBackendInfo{}, errors.New("name the directory: kapi context backend file <dir>")
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return ContextBackendInfo{}, err
		}
		overrides[string(w.Key())] = project.ContextBackend{Backend: project.ContextBackendFile, Path: abs}
	default:
		return ContextBackendInfo{}, fmt.Errorf("%q is not a choice here; use local, file <dir>, or recipe (git and s3 backends are declared in kapi.yaml)", kind)
	}
	if err := writeBackendOverrides(overrides); err != nil {
		return ContextBackendInfo{}, err
	}
	return a.ContextBackend(ctx, projectPath)
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
	info, err := resolveBackend(layout, w.Key())
	if err != nil {
		return nil, err
	}
	if info.Kind == project.ContextBackendLocal {
		why := "the recipe declares no context backend"
		if info.From == "machine" {
			why = "this machine keeps the project's context local (kapi context backend recipe undoes that)"
		}
		return nil, fmt.Errorf("there is nowhere to sync with: %s. Declare one in kapi.yaml, for example `context: {backend: git}`", why)
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
		fmt.Fprintf(w, "Pushed %s to %s in %.1fs.\n",
			pluralUnit(r.Pushed, "operation", "operations"), describeRemote(r.Remote), r.Seconds)
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

// syncError explains a failed pull or push and gives it its exit code.
func syncError(err error, verb string, queued int) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, workspace.ErrRemoteUnreachable):
		return WithExitCode(ExitUnreachable, fmt.Errorf(
			"%w\nNothing changed here. %s to be pushed; run `kapi context %s` again when the backend is reachable",
			err, pluralUnit(queued, "operation waits", "operations wait"), verb))
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
	info, err := resolveBackend(layout, w.Key())
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
