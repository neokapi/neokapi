package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/host/output"
)

// What a decision records for a named path that is not a regular file: one
// that is not there (a file that appears later asks again), and a directory,
// whose content is not checked.
const (
	fileAbsent    = "absent"
	fileDirectory = "directory"
)

// execCheckLimit is what every question about a recipe's commands says about
// what kapi checks.
const execCheckLimit = "kapi checks the content of the files listed, not what those files or the commands run or read."

// execSurface is everything a project can make kapi execute, as a decision
// covers it.
type execSurface struct {
	Sites []project.ExecSite
	// Digest fingerprints the sites, what they name and the project.
	Digest string
	// Files maps each regular file the sites name inside the project,
	// slash-separated and relative to the project, to its SHA-256. These are
	// the files the decision checks.
	Files map[string]string
	// NotChecked names the directories the sites name in the project: named,
	// and their content not checked.
	NotChecked []string
	// Shells names the sites that hand a command string to a shell, whose
	// files kapi cannot see.
	Shells []string
}

// execSurfaceOf reads everything the project at recipePath can make kapi
// execute (project.ExecSurfaceIn: the recipe and its flow files) together
// with what the commands name. A command that names a regular file in the
// project (a script_file, an executable given by a path, an interpreter's
// script) runs that file, so the decision covers its content, the command
// line and the project's id.
//
// A relative path is read against the project root, the directory a recipe's
// command runs in (recipeCommandConfig), and through it. One that is or
// passes through a link out of the project is refused. A named directory is
// listed as not checked. A recipe that sets a key kapi owns is refused.
func execSurfaceOf(recipePath string, proj *project.KapiProject) (execSurface, error) {
	root := filepath.Dir(execTrustKey(recipePath))
	sites := project.ExecSurfaceIn(proj, root)
	if len(sites) == 0 {
		return execSurface{}, nil
	}
	surface := execSurface{Files: map[string]string{}}
	dirs := map[string]bool{}
	for i, site := range sites {
		if keys := site.KapiOwnedKeys(); len(keys) > 0 {
			return execSurface{}, fmt.Errorf("%s %s sets %s, which kapi sets itself: a recipe's command runs in the project root and learns where kapi started from %s. Remove %s from the recipe",
				site.Where, site.Name, strings.Join(keys, " and "), envInvokeDir, strings.Join(keys, " and "))
		}
		named := map[string]string{}
		for _, p := range site.NamedPaths() {
			rel, state, err := hashNamedFile(root, p)
			if err != nil {
				return execSurface{}, fmt.Errorf("%s %s names %s: %w", site.Where, site.Name, p, err)
			}
			if rel == "" {
				continue
			}
			named[rel] = state
			switch state {
			case fileAbsent:
			case fileDirectory:
				dirs[rel] = true
			default:
				surface.Files[rel] = state
			}
		}
		if site.RunsShellString() {
			surface.Shells = append(surface.Shells, site.Where+" "+site.Name)
		}
		facts := map[string]any{"project": proj.ID}
		if len(named) > 0 {
			facts["files_sha256"] = named
		}
		sites[i] = site.WithFacts(facts)
	}
	for d := range dirs {
		surface.NotChecked = append(surface.NotChecked, d)
	}
	sort.Strings(surface.NotChecked)
	surface.Sites, surface.Digest = sites, project.ExecSurfaceDigest(sites)
	return surface, nil
}

// envInvokeDir is the variable a recipe's command reads the directory kapi
// was started in from.
const envInvokeDir = "KAPI_INVOKE_DIR"

// hashNamedFile resolves p against the project root, as a recipe's command
// resolves it. When it lies in the project it returns its project-relative
// name and the SHA-256 of a regular file, fileDirectory for a directory, or
// fileAbsent for anything else. A path outside the project (a system
// program) returns "". A link that leads out of the project is refused.
func hashNamedFile(root, p string) (rel, state string, err error) {
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	r, err := filepath.Rel(root, abs)
	if err != nil || !filepath.IsLocal(r) {
		if r, err = filepath.Rel(realRoot, abs); err != nil || !filepath.IsLocal(r) {
			return "", "", nil
		}
		root = realRoot
	}
	rel = filepath.ToSlash(r)
	f, err := rootpath.OpenRegular(root, abs)
	var escape *rootpath.EscapeError
	switch {
	case errors.As(err, &escape):
		return "", "", errors.New("a link that leads out of the project, which kapi refuses rather than reads: name the file itself")
	case errors.Is(err, rootpath.ErrNotRegular):
		if info, serr := rootpath.Stat(root, abs); serr == nil && info.IsDir() {
			return rel, fileDirectory, nil
		}
		return rel, fileAbsent, nil
	case errors.Is(err, fs.ErrNotExist):
		return rel, fileAbsent, nil
	case err != nil:
		return "", "", err
	}
	defer f.Close()
	sum, err := cachedSHA256(f)
	return rel, sum, err
}

// fileHashes remembers each hashed file's SHA-256 with the size and
// modification time it had, so a check at every command run reads a file
// again only when it changed.
var fileHashes = struct {
	sync.Mutex
	m map[string]cachedHash
}{m: map[string]cachedHash{}}

type cachedHash struct {
	size    int64
	modTime time.Time
	sum     string
}

// cachedSHA256 hashes the open file f, reusing the hash remembered for it
// while its size and modification time are unchanged.
func cachedSHA256(f *os.File) (string, error) {
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	key := f.Name()
	fileHashes.Lock()
	c, ok := fileHashes.m[key]
	fileHashes.Unlock()
	if ok && c.size == info.Size() && c.modTime.Equal(info.ModTime()) {
		return c.sum, nil
	}
	sum, err := sha256Of(f)
	if err != nil {
		return "", err
	}
	fileHashes.Lock()
	fileHashes.m[key] = cachedHash{size: info.Size(), modTime: info.ModTime(), sum: sum}
	fileHashes.Unlock()
	return sum, nil
}

// sha256Of hashes what r holds. Every file an execution decision covers is
// hashed with it, a recipe's named files and a formatter's alike.
func sha256Of(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// changedExecFiles lists the checked files whose content differs from what
// the decision recorded at key saw, so a question can say which file changed.
// A decision recorded before files were covered lists none.
func changedExecFiles(key string, now map[string]string) []string {
	rec, ok := loadExecTrust().Projects[key]
	if !ok || rec.Files == nil {
		return nil
	}
	var changed []string
	for name, sum := range now {
		if rec.Files[name] != sum {
			changed = append(changed, name)
		}
	}
	for name := range rec.Files {
		if _, ok := now[name]; !ok {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

// addedExecSites lists the sites a decision recorded at key did not show,
// such as a step in a flow file a pull added. A decision recorded before
// sites were kept lists none.
func addedExecSites(key string, sites []project.ExecSite) []string {
	rec, ok := loadExecTrust().Projects[key]
	if !ok || rec.Sites == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, s := range rec.Sites {
		seen[s] = true
	}
	var added []string
	for _, s := range siteNames(sites) {
		if !seen[s] {
			added = append(added, s)
		}
	}
	return added
}

// siteNames locates each site by where it is declared and what it runs.
func siteNames(sites []project.ExecSite) []string {
	out := make([]string, 0, len(sites))
	for _, s := range sites {
		out = append(out, s.Where+" "+s.Name)
	}
	return out
}

// formatExecCoverage says what an answer about the commands covers: the files
// it checks, the ones named and not checked, the commands that go through a
// shell, what was added or changed since the last answer, and the limit.
func formatExecCoverage(surface execSurface, key string) string {
	var b strings.Builder
	if names := sortedKeys(surface.Files); len(names) > 0 {
		fmt.Fprintf(&b, "Checked, by content: %s\n", strings.Join(names, ", "))
	} else {
		fmt.Fprintln(&b, "Checked, by content: no file; the commands name none in the project")
	}
	if len(surface.NotChecked) > 0 {
		fmt.Fprintf(&b, "Named and not checked (directories): %s\n", strings.Join(surface.NotChecked, ", "))
	}
	for _, s := range surface.Shells {
		fmt.Fprintf(&b, "Runs through a shell, so kapi cannot see which files it uses: %s\n", s)
	}
	if added := addedExecSites(key, surface.Sites); len(added) > 0 {
		fmt.Fprintf(&b, "Added since it was answered: %s\n", strings.Join(added, ", "))
	}
	if changed := changedExecFiles(key, surface.Files); len(changed) > 0 {
		fmt.Fprintf(&b, "Changed since it was answered: %s\n", strings.Join(changed, ", "))
	}
	fmt.Fprintln(&b, execCheckLimit)
	return b.String()
}

// coverageSummary is formatExecCoverage in one line, for kapi trust list.
func coverageSummary(surface execSurface) string {
	parts := []string{"checks " + pluralUnit(len(surface.Files), "file", "files")}
	if len(surface.NotChecked) > 0 {
		parts = append(parts, "not checked: "+strings.Join(surface.NotChecked, ", "))
	}
	if len(surface.Shells) > 0 {
		parts = append(parts, "through a shell: "+strings.Join(surface.Shells, ", "))
	}
	return strings.Join(parts, "; ") + "; not what they run or read"
}

// recipeCommandConfig fixes what a recipe's exec-class step resolves against:
// the project root. Every flow builds its tools through toolFromStep or
// buildToolByName, and both pass the step's configuration through here. A
// recipe then means the same command, named files and working directory from
// wherever kapi is started: the CLI anywhere under the project, the desktop,
// a daemon, a convergence run.
//
// external-command runs in the root, reads a relative command from it, and
// gets the directory kapi was started in as KAPI_INVOKE_DIR. The script tool
// reads a relative script_file from the root. A recipe that sets a key kapi
// owns is refused. With no project bound the configuration passes through,
// as for `kapi exec`, whose command a person typed.
func (a *App) recipeCommandConfig(toolName string, config map[string]any) (map[string]any, error) {
	if !project.IsExecClassTool(toolName) || a.ProjectContext == nil || a.ProjectContext.ProjectDir == "" {
		return config, nil
	}
	for _, key := range []string{"working_dir", "invoke_dir"} {
		if _, set := config[key]; set {
			return nil, fmt.Errorf("the %s step sets %s, which kapi sets itself: a recipe's command runs in the project root and learns where kapi started from %s. Remove %s from the recipe",
				toolName, key, envInvokeDir, key)
		}
	}
	root := a.ProjectContext.ProjectDir
	out := make(map[string]any, len(config)+2)
	maps.Copy(out, config)
	switch toolName {
	case "external-command":
		out["working_dir"] = root
		if cwd, err := os.Getwd(); err == nil {
			out["invoke_dir"] = cwd
		}
	case "script":
		if f, ok := out["script_file"].(string); ok && f != "" && !filepath.IsAbs(f) {
			out["script_file"] = filepath.Join(root, f)
		}
	}
	return out, nil
}

// ExecTrustRequest is what a surface that asks through its own dialog (the
// desktop) shows before a run: what the project's commands run, what an
// answer checks and does not, what changed, and the limit.
type ExecTrustRequest struct {
	Recipe     string             `json:"recipe"`
	Sites      []output.TrustSite `json:"sites"`
	Checked    []string           `json:"checked"`
	NotChecked []string           `json:"not_checked,omitempty"`
	Shells     []string           `json:"shells,omitempty"`
	Added      []string           `json:"added,omitempty"`
	Changed    []string           `json:"changed,omitempty"`
	Limit      string             `json:"limit"`
	Declined   bool               `json:"declined,omitempty"`
	// Digest is what an answer covers; ApproveExecTrustAt records it only
	// while the project still digests to it.
	Digest string `json:"digest"`
}

// PendingExecTrust is the question the project at recipePath's commands need
// answered before a run, or nil when they need none: no commands, an allow
// recorded for them as they stand, or the environment grant.
func PendingExecTrust(recipePath string) (*ExecTrustRequest, error) {
	if execTrustImplied || execTrustEnvGranted() {
		return nil, nil
	}
	key := execTrustKey(recipePath)
	proj, err := project.LoadWithOptions(key, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, err
	}
	surface, err := execSurfaceOf(key, proj)
	if err != nil || len(surface.Sites) == 0 {
		return nil, err
	}
	decision, found := lookupExecTrust(key, surface.Digest)
	if found && decision == execTrustAllow {
		return nil, nil
	}
	return &ExecTrustRequest{
		Recipe:     key,
		Sites:      trustSites(surface.Sites),
		Checked:    sortedKeys(surface.Files),
		NotChecked: surface.NotChecked,
		Shells:     surface.Shells,
		Added:      addedExecSites(key, surface.Sites),
		Changed:    changedExecFiles(key, surface.Files),
		Limit:      execCheckLimit,
		Declined:   found && decision == execTrustDeny,
		Digest:     surface.Digest,
	}, nil
}

// ApproveExecTrustAt records that a person allows the project's commands, as
// the request with digest showed them. It refuses when the project changed
// since (the person is asked again) and when an agent is running.
func (a *App) ApproveExecTrustAt(recipePath, digest string) error {
	if a.agentRunning() {
		return errors.New("an agent is running this command, and only a person can allow a project's commands")
	}
	key := execTrustKey(recipePath)
	proj, err := project.LoadWithOptions(key, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return err
	}
	surface, err := execSurfaceOf(key, proj)
	if err != nil {
		return err
	}
	if surface.Digest == "" || surface.Digest != digest {
		return errors.New("the project's commands or the files they name changed since they were shown: look at them again before allowing them")
	}
	return recordExecTrustSurface(key, execTrustAllow, surface)
}
