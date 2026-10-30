package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// `kapi share`: how far a project's rules reach, and a sync through the place
// they are shared.
//
// A project's context reaches as far as one of three rungs: this machine's
// workspace, a place the team shares (the `context:` block of kapi.yaml), or a
// Bowrain server (the plugin's `bowrain:` block). Share names the rung in a
// person's words, writes the `context:` block the desktop's Share section
// writes, and syncs through it with the same sync `kapi context sync` runs.

// The rungs of the sharing ladder.
const (
	ShareRungMachine = "machine"
	ShareRungTeam    = "team"
	ShareRungBowrain = "bowrain"
)

// The places `kapi share --via` takes.
const (
	ShareViaGit     = "git"
	ShareViaFolder  = "folder"
	ShareViaS3      = "s3"
	ShareViaBowrain = "bowrain"
)

// ShareVias lists the places `--via` takes, in the order help names them.
var ShareVias = []string{ShareViaGit, ShareViaFolder, ShareViaS3, ShareViaBowrain}

// bowrainPlugin is the registry name of the plugin that connects a project to
// a Bowrain server, and bowrainConnect the handler of its `kapi init`
// contribution, which does the connecting.
const (
	bowrainPlugin  = "bowrain"
	bowrainConnect = "init-connect"
)

// ShareRequest asks for one `kapi share`.
type ShareRequest struct {
	// Project is the recipe path. The file may not exist yet: a share that
	// names a place writes it.
	Project string
	// Via names the place to share through: git, folder, s3 or bowrain.
	// Empty syncs through the place the recipe already names.
	Via string
	// Location is the value the place needs: the folder's path, the S3 URL,
	// or the Bowrain server's URL. Git needs none.
	Location string
	// Status reports the rung and the last sync without syncing.
	Status bool
	// Merged, PR, Merger, NoPush and FilesOnly pass to the sync with the
	// meaning ContextSyncRequest gives them.
	Merged    string
	PR        int
	Merger    string
	NoPush    bool
	FilesOnly bool
}

// ShareFile is one file a share wrote or found as it should be.
type ShareFile struct {
	// Path is project-relative and slash-separated.
	Path string `json:"path"`
	// Action is created, updated or kept, or what a sync did to a rules file.
	Action string `json:"action"`
}

// The actions a share reports for kapi.yaml.
const (
	ShareFileCreated = "created"
	ShareFileUpdated = "updated"
	ShareFileKept    = "kept"
)

// ShareRung is one rung of the ladder as `kapi share` prints it.
type ShareRung struct {
	Rung    string `json:"rung"`
	Label   string `json:"label"`
	Reach   string `json:"reach"`
	Current bool   `json:"current"`
	// Next is the command that moves the project to this rung.
	Next string `json:"next,omitempty"`
}

// ShareResult reports one `kapi share`.
type ShareResult struct {
	// Rung is where the project's rules reach: machine, team or bowrain.
	Rung string `json:"rung"`
	// Backend is the place the recipe shares the context through, nil when
	// it names none.
	Backend *ContextBackendInfo `json:"backend,omitempty"`
	// Server is the Bowrain server the recipe connects to, empty when none.
	Server string `json:"server,omitempty"`
	// Files are the files the share wrote or kept, kapi.yaml first.
	Files []ShareFile `json:"files,omitempty"`
	// Sync is what the sync did, nil when none ran.
	Sync *ContextSyncResult `json:"sync,omitempty"`
	// Status is how far apart this machine and the shared copy were at the
	// last sync, for a share that reports and does not sync.
	Status *workspace.SyncStatus `json:"status,omitempty"`
	// Ladder is every rung, printed when the rules stay on this machine.
	Ladder []ShareRung `json:"ladder,omitempty"`
	// BowrainPlugin reports whether the bowrain plugin is installed.
	BowrainPlugin bool `json:"bowrain_plugin"`

	now time.Time
}

// machineLabel names the first rung for the machine kapi runs on.
func machineLabel() string {
	if runtime.GOOS == "darwin" {
		return "On this Mac"
	}
	return "On this machine"
}

// shareLadder is the three rungs, with the current one marked.
func shareLadder(current string, plugin bool) []ShareRung {
	bowrainNext := "kapi share --via bowrain <server-url>"
	if !plugin {
		bowrainNext += "  (needs the kapi-bowrain plugin)"
	}
	return []ShareRung{
		{Rung: ShareRungMachine, Label: machineLabel(), Reach: "your rules, your assistants", Current: current == ShareRungMachine},
		{Rung: ShareRungTeam, Label: "With your team", Reach: "a shared folder or git", Current: current == ShareRungTeam,
			Next: "kapi share --via git | folder <path>"},
		{Rung: ShareRungBowrain, Label: "Managed on Bowrain", Reach: "a team queue, who kept what, the same screens in a browser",
			Current: current == ShareRungBowrain, Next: bowrainNext},
	}
}

// FormatText renders the share for a person.
func (r *ShareResult) FormatText(w io.Writer) error {
	for _, f := range r.Files {
		fmt.Fprintf(w, "%-10s %s\n", f.Path, f.Action)
	}
	if r.Ladder != nil {
		fmt.Fprintln(w, "How far your rules reach")
		for _, rung := range r.Ladder {
			mark := " "
			if rung.Current {
				mark = "*"
			}
			what := rung.Next
			if rung.Current || what == "" {
				what = rung.Reach
			}
			fmt.Fprintf(w, "  %s %-18s %s\n", mark, rung.Label, what)
		}
		return nil
	}
	fmt.Fprintln(w, r.rungLine())
	if r.Sync != nil {
		return r.formatSync(w)
	}
	if r.Backend != nil && r.Backend.Kind != project.ContextBackendLocal {
		fmt.Fprintf(w, "  %s\n", r.statusLine())
	}
	if r.Rung == ShareRungBowrain {
		fmt.Fprintln(w, "  send with kapi push · bring back with kapi pull")
	}
	return nil
}

// rungLine names the rung and the place, such as
// "With your team · through git (origin refs/kapi/context)".
func (r *ShareResult) rungLine() string {
	switch r.Rung {
	case ShareRungBowrain:
		line := "Managed on Bowrain"
		if r.Server != "" {
			line += " · " + r.Server
		}
		return line
	case ShareRungTeam:
		return "With your team · " + throughPlace(r.Backend)
	}
	return machineLabel() + " · your rules, your assistants"
}

// throughPlace names a shared place in a person's words.
func throughPlace(b *ContextBackendInfo) string {
	if b == nil {
		return ""
	}
	switch b.Kind {
	case project.ContextBackendGit:
		return "through git (" + b.Location + ")"
	case project.ContextBackendFile:
		return "through a shared folder (" + b.Location + ")"
	case project.ContextBackendS3:
		return "through S3 (" + b.Location + ")"
	}
	return "through " + b.Kind + " (" + b.Location + ")"
}

// formatSync renders what the sync did under the rung line.
func (r *ShareResult) formatSync(w io.Writer) error {
	s := r.Sync
	if s.filesOnly {
		if s.RulesFiles == nil || len(s.RulesFiles.Files) == 0 {
			fmt.Fprintln(w, "  no rules files: the project's context holds no rules yet")
		}
		return nil
	}
	var parts []string
	var contacted time.Time
	if s.Push != nil {
		parts = append(parts, "pushed "+changeCount(s.Push.Pushed))
		contacted = s.Push.Contacted
	} else if s.Pull != nil {
		parts = append(parts, "pushed nothing")
	}
	if s.Pull != nil {
		parts = append(parts, "pulled "+changeCount(s.Pull.Merged))
		if contacted.IsZero() {
			contacted = s.Pull.Contacted
		}
	}
	if len(parts) > 0 {
		parts = append(parts, "last sync "+r.when(contacted))
		fmt.Fprintf(w, "  %s\n", strings.Join(parts, " · "))
	}
	if s.Pull != nil && s.Pull.Error != "" {
		fmt.Fprintf(w, "  note: %s\n", s.Pull.Error)
	}
	if s.Settle != nil {
		if err := s.Settle.FormatText(w); err != nil {
			return err
		}
	}
	if s.RulesFilesError != "" {
		fmt.Fprintf(w, "  the rules files could not be refreshed: %s\n", s.RulesFilesError)
	}
	return nil
}

// statusLine is how far apart this machine and the shared copy were at the
// last sync, without syncing.
func (r *ShareResult) statusLine() string {
	st := r.Status
	if st == nil {
		return "not synced from this machine yet"
	}
	push := changeCount(st.ToPush) + " to push"
	if st.Contacted.IsZero() {
		return push + " · never synced"
	}
	line := push + " · " + changeCount(st.ToPull) + " to pull · last sync " + r.when(st.Contacted)
	if st.Error != "" {
		line += " (the place could not be reached)"
	}
	return line
}

// changeCount counts operations the way the share lines read them.
func changeCount(n int) string {
	if n == 0 {
		return "none"
	}
	return pluralUnit(n, "change", "changes")
}

// when says how long ago t was, in a person's words.
func (r *ShareResult) when(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	now := r.now
	if now.IsZero() {
		now = time.Now()
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return pluralUnit(int(d/time.Minute), "minute", "minutes") + " ago"
	case d < 24*time.Hour:
		return pluralUnit(int(d/time.Hour), "hour", "hours") + " ago"
	}
	return "on " + t.Local().Format("2006-01-02 15:04")
}

// Share reports how far the project's rules reach and syncs them through the
// place the recipe names. With Via set it first writes that place into the
// recipe, the way the desktop's Share section does, writing the recipe itself
// when the folder has none. With Status set it reports and does not sync.
func (a *App) Share(ctx context.Context, req ShareRequest) (*ShareResult, error) {
	if req.Project == "" {
		return nil, errors.New("share: no project path")
	}
	res := &ShareResult{BowrainPlugin: a.bowrainPlugin() != nil}
	if req.Via != "" {
		if req.Status {
			return nil, WithExitCode(ExitUsage, errors.New("--status reports where the rules reach and takes no --via"))
		}
		if err := a.shareVia(ctx, req, res); err != nil {
			return nil, err
		}
	} else if req.Location != "" {
		return nil, WithExitCode(ExitUsage, fmt.Errorf("%q names a place: give it with --via, such as kapi share --via folder %s", req.Location, req.Location))
	}
	if err := a.readRung(ctx, req.Project, res); err != nil {
		return nil, err
	}

	shared := res.Backend != nil && res.Backend.Kind != project.ContextBackendLocal
	switch {
	case req.Status:
		if !shared && res.Rung == ShareRungMachine {
			res.Ladder = shareLadder(res.Rung, res.BowrainPlugin)
		}
		return res, nil
	case req.FilesOnly || req.Merged != "":
	case !shared:
		if res.Rung == ShareRungMachine {
			res.Ladder = shareLadder(res.Rung, res.BowrainPlugin)
		}
		return res, nil
	}
	sync, err := a.SyncProjectContext(ctx, ContextSyncRequest{
		Project: req.Project, Merged: req.Merged, PR: req.PR, Merger: req.Merger,
		NoPush: req.NoPush, FilesOnly: req.FilesOnly,
	})
	if err != nil {
		return nil, err
	}
	res.Sync = &sync
	files := sync.RulesFiles.Changed()
	if req.FilesOnly && sync.RulesFiles != nil {
		files = sync.RulesFiles.Files
	}
	for _, f := range files {
		action := string(f.Action)
		if f.Action == RulesFileUnchanged {
			action = ShareFileKept
		}
		res.Files = append(res.Files, ShareFile{Path: f.Path, Action: action})
	}
	return res, nil
}

// readRung reads the recipe's place and server, and the last sync through the
// place without reaching it.
func (a *App) readRung(ctx context.Context, recipe string, res *ShareResult) error {
	res.Rung = ShareRungMachine
	if _, err := os.Stat(recipe); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	info, err := ResolveContextBackend(recipe)
	if err != nil {
		return err
	}
	res.Backend = &info
	if info.Kind != project.ContextBackendLocal {
		res.Rung = ShareRungTeam
		res.Status = a.ContextSyncStatus(ctx, filepath.Dir(recipe))
	}
	if server := recipeServer(recipe); server != "" {
		res.Rung, res.Server = ShareRungBowrain, server
	}
	return nil
}

// recipeServer is the URL of the Bowrain server the recipe connects to. The
// block belongs to the plugin, so a binary without it reads the plugin's
// recipe key directly.
func recipeServer(recipe string) string {
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return ""
	}
	if venue, ok := proj.Venue(); ok {
		return venue.URL
	}
	hint, ok := PluginHintFor(bowrainPlugin)
	if !ok {
		return ""
	}
	node, ok := proj.Extras[hint.RecipeKey]
	if !ok {
		return ""
	}
	var block struct {
		URL string `yaml:"url"`
	}
	if err := node.Decode(&block); err != nil || block.URL == "" {
		return "a Bowrain server"
	}
	return block.URL
}

// bowrainPlugin is the installed bowrain plugin, nil when it is not.
func (a *App) bowrainPlugin() *pluginhost.Plugin {
	if a.PluginHost == nil {
		return nil
	}
	return a.PluginHost.Plugin(bowrainPlugin)
}

// shareVia writes the place req names into the recipe, writing the recipe
// first when the folder has none. Everything the place needs is checked
// before anything is written.
func (a *App) shareVia(ctx context.Context, req ShareRequest, res *ShareResult) error {
	root := filepath.Dir(req.Project)
	if req.Via == ShareViaBowrain {
		return a.shareViaBowrain(ctx, req, res)
	}
	spec, err := shareSpec(ctx, req, root)
	if err != nil {
		return err
	}
	file, err := a.ensureRecipe(req.Project)
	if err != nil {
		return err
	}
	proj, err := project.LoadWithOptions(req.Project, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if proj.Context == nil || *proj.Context != spec {
		block := spec
		proj.Context = &block
		if err := project.Save(req.Project, proj); err != nil {
			return err
		}
		if file.Action == ShareFileKept {
			file.Action = ShareFileUpdated
		}
	}
	res.Files = append(res.Files, file)
	return nil
}

// shareSpec is the `context:` block a --via names, in the shape the desktop's
// Share section saves for the same place. A place that cannot be used from
// this folder is refused, with the flag or value that is missing.
func shareSpec(ctx context.Context, req ShareRequest, root string) (project.ContextBackend, error) {
	var spec project.ContextBackend
	switch req.Via {
	case ShareViaGit:
		if req.Location != "" {
			return spec, WithExitCode(ExitUsage, fmt.Errorf("--via git takes no value: the context is kept in this repository, outside every branch (%q was given)", req.Location))
		}
		if _, ok := workspace.GitRepoRoot(ctx, root); !ok {
			return spec, WithExitCode(ExitUsage, fmt.Errorf("%s is not in a git repository: share through a folder instead, with kapi share --via folder <path>", root))
		}
		spec = project.ContextBackend{Backend: project.ContextBackendGit}
	case ShareViaFolder:
		if req.Location == "" {
			return spec, WithExitCode(ExitUsage, errors.New("--via folder needs the folder: kapi share --via folder <path>"))
		}
		spec = project.ContextBackend{Backend: project.ContextBackendFile, Path: folderPath(req.Location, root)}
	case ShareViaS3:
		if req.Location == "" {
			return spec, WithExitCode(ExitUsage, errors.New("--via s3 needs the bucket: kapi share --via s3 s3://<bucket>/<prefix>"))
		}
		if !HasContextRemote(project.ContextBackendS3) {
			return spec, fmt.Errorf("this build of kapi has no %s backend", project.ContextBackendS3)
		}
		bucket, prefix := parseS3URL(req.Location)
		spec = project.ContextBackend{Backend: project.ContextBackendS3, Bucket: bucket, Prefix: prefix}
	default:
		return spec, WithExitCode(ExitUsage, fmt.Errorf("--via %q is not a place to share through; use %s", req.Via, strings.Join(ShareVias, ", ")))
	}
	return spec, spec.Validate()
}

// folderPath is the path the recipe keeps for a folder a person named. A
// relative path was typed against the working directory, and the recipe reads
// a relative path against its own directory, so it is rewritten to name the
// same folder from there.
func folderPath(path, root string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// parseS3URL splits s3://bucket/prefix, or bucket/prefix, into its parts.
func parseS3URL(url string) (bucket, prefix string) {
	rest := strings.TrimPrefix(url, "s3://")
	bucket, prefix, _ = strings.Cut(rest, "/")
	return bucket, prefix
}

// ensureRecipe writes the draft `kapi init` writes when the folder has no
// recipe, and reports kapi.yaml as created or kept.
func (a *App) ensureRecipe(recipe string) (ShareFile, error) {
	file := ShareFile{Path: project.RecipeFileName, Action: ShareFileKept}
	if _, err := os.Stat(recipe); err == nil {
		return file, nil
	} else if !os.IsNotExist(err) {
		return file, err
	}
	a.InitRegistries()
	if _, err := InitProject(filepath.Dir(recipe), InitOptions{Formats: a.FormatReg}); err != nil {
		return file, err
	}
	file.Action = ShareFileCreated
	return file, nil
}

// shareViaBowrain connects the project to a Bowrain server through the
// plugin's `kapi init --server` handler. Without the plugin nothing is
// written and the error says how to install it.
func (a *App) shareViaBowrain(ctx context.Context, req ShareRequest, res *ShareResult) error {
	plugin := a.bowrainPlugin()
	if plugin == nil {
		hint, _ := PluginHintFor(bowrainPlugin)
		return WithExitCode(ExitUsage, fmt.Errorf(
			"sharing on Bowrain needs the kapi-bowrain plugin, which is not installed: install it with `kapi plugin install %s` or `brew install %s`, then run kapi share --via bowrain <server-url>",
			hint.Plugin, hint.Formula))
	}
	if req.Location == "" {
		return WithExitCode(ExitUsage, errors.New("--via bowrain needs the server: kapi share --via bowrain <server-url>"))
	}
	file, err := a.ensureRecipe(req.Project)
	if err != nil {
		return err
	}
	before, err := os.ReadFile(req.Project)
	if err != nil {
		return err
	}
	args := []string{"command", bowrainConnect, "--server", req.Location}
	if err := pluginhost.RunContributionSubprocess(ctx, plugin, args, filepath.Dir(req.Project)); err != nil {
		return err
	}
	after, err := os.ReadFile(req.Project)
	if err != nil {
		return err
	}
	if file.Action == ShareFileKept && string(before) != string(after) {
		file.Action = ShareFileUpdated
	}
	res.Files = append(res.Files, file)
	return nil
}
