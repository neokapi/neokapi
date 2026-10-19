package host

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/ignore"
)

// ignoreFileNames are the files whose rules an expansion honours in each
// directory: git's, and the project's own in the same syntax.
var ignoreFileNames = []string{".gitignore", ".kapiignore"}

// walkIgnores holds the ignore rules that govern an expansion of a directory or
// a glob. Each scope's rules match paths relative to the directory holding the
// file they came from, as git reads a nested .gitignore. The rules above the
// expansion's root apply from the top of the git repository that holds it.
type walkIgnores struct {
	scopes []ignoreScope
	loaded map[string]bool
}

type ignoreScope struct {
	dir string
	m   *ignore.Matcher
}

// newWalkIgnores returns the rules for an expansion rooted at root: those in
// root, and those in each directory above it up to the top of its git
// repository. Outside a repository only root and what lies beneath it hold
// rules.
func newWalkIgnores(root string) *walkIgnores {
	w := &walkIgnores{loaded: map[string]bool{}}
	abs, err := filepath.Abs(root)
	if err != nil {
		return w
	}
	if !isRepoTop(abs) {
		var above []string
		for d := filepath.Dir(abs); ; d = filepath.Dir(d) {
			above = append(above, d)
			if isRepoTop(d) {
				for _, dir := range above {
					w.load(dir)
				}
				break
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	w.load(abs)
	return w
}

// isRepoTop reports whether dir is the top of a git checkout or worktree.
func isRepoTop(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// load adds the rules in dir's ignore files, once per directory. dir is
// absolute.
func (w *walkIgnores) load(dir string) {
	if w.loaded[dir] {
		return
	}
	w.loaded[dir] = true
	var m *ignore.Matcher
	for _, name := range ignoreFileNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			continue
		}
		if m == nil {
			m = &ignore.Matcher{}
		}
		_ = m.LoadFile(filepath.Join(dir, name))
	}
	if m != nil {
		w.scopes = append(w.scopes, ignoreScope{dir: dir, m: m})
	}
}

// ignored reports whether a rule of a directory above path ignores it. path is
// absolute.
func (w *walkIgnores) ignored(path string, isDir bool) bool {
	for _, s := range w.scopes {
		rel, err := filepath.Rel(s.dir, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if s.m.Match(filepath.ToSlash(rel), isDir) {
			return true
		}
	}
	return false
}

// ignoredBelow reports whether the rules ignore rel, a slash path under root,
// or a directory between them, loading the rules of each directory on the way.
// Root itself is never ignored: a name the user typed selects it.
func (w *walkIgnores) ignoredBelow(root, rel string, isDir bool) bool {
	abs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	segments := strings.Split(rel, "/")
	dir := abs
	for i, seg := range segments {
		path := filepath.Join(dir, seg)
		last := i == len(segments)-1
		if w.ignored(path, !last || isDir) {
			return true
		}
		if !last {
			w.load(path)
		}
		dir = path
	}
	return false
}
