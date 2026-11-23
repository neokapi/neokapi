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

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
)

// fileAbsent is what a decision records for a named file that is not there,
// so a file that appears later asks again.
const fileAbsent = "absent"

// execSurface is a recipe's exec surface as a decision covers it.
type execSurface struct {
	Sites []project.ExecSite
	// Digest fingerprints the sites, the files they name and the project.
	Digest string
	// Files maps each file the sites name inside the project, slash-separated
	// and relative to the project, to its SHA-256 or fileAbsent.
	Files map[string]string
}

// execSurfaceOf reads the recipe's exec surface together with what it names.
// A command that names a file in the project (a script_file, an executable
// given by a path, an interpreter's script) runs that file. The decision
// covers the file's content, the command line and the project's id, so an
// edited script or another repository at the same path asks again.
//
// A relative path is read against the project root, the directory a recipe's
// command runs in (recipeCommandConfig), and through it, so the approval
// covers the file the command runs from wherever kapi is started. One that is
// or passes through a link out of the project is refused. What a command
// reads at run time without the recipe naming it is not covered.
func execSurfaceOf(recipePath string, proj *project.KapiProject) (execSurface, error) {
	sites := project.ExecSurface(proj)
	if len(sites) == 0 {
		return execSurface{}, nil
	}
	root := filepath.Dir(execTrustKey(recipePath))
	files := map[string]string{}
	for i, site := range sites {
		named := map[string]string{}
		for _, p := range site.NamedPaths() {
			rel, sum, err := hashNamedFile(root, p)
			if err != nil {
				return execSurface{}, fmt.Errorf("%s %s names %s: %w", site.Where, site.Name, p, err)
			}
			if rel != "" {
				named[rel], files[rel] = sum, sum
			}
		}
		facts := map[string]any{"project": proj.ID}
		if len(named) > 0 {
			facts["files_sha256"] = named
		}
		sites[i] = site.WithFacts(facts)
	}
	return execSurface{Sites: sites, Digest: project.ExecSurfaceDigest(sites), Files: files}, nil
}

// hashNamedFile resolves p against the project root, as a recipe's command
// resolves it, and, when it lies in the project, returns its project-relative
// name and SHA-256. A path outside the project (a system program) returns "".
// A link that leads out of the project is refused.
func hashNamedFile(root, p string) (rel, sum string, err error) {
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
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, rootpath.ErrNotRegular):
		return rel, fileAbsent, nil
	case err != nil:
		return "", "", err
	}
	defer f.Close()
	sum, err = sha256Of(f)
	return rel, sum, err
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

// changedExecFiles lists the named files whose content differs from what the
// decision recorded at key saw, so a prompt can say which file changed. A
// decision recorded before files were covered lists none.
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

// formatExecFiles says which files the decision covers and which changed.
func formatExecFiles(files map[string]string, changed []string) string {
	if len(files) == 0 && len(changed) == 0 {
		return ""
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	if len(names) > 0 {
		fmt.Fprintf(&b, "The answer covers the content of: %s\n", strings.Join(names, ", "))
	}
	if len(changed) > 0 {
		fmt.Fprintf(&b, "Changed since it was answered: %s\n", strings.Join(changed, ", "))
	}
	return b.String()
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
// reads a relative script_file from the root. A working directory the recipe
// gave is replaced. With no project bound the configuration passes through,
// as for `kapi exec`, whose command a person typed.
func (a *App) recipeCommandConfig(toolName string, config map[string]any) map[string]any {
	if !project.IsExecClassTool(toolName) || a.ProjectContext == nil || a.ProjectContext.ProjectDir == "" {
		return config
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
	return out
}
