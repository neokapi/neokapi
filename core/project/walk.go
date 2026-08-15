package project

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/ignore"
)

// WalkProjectDir walks the project tree rooted at root in lexical order,
// applying the project's standard visibility rules — the one walk behind
// every "files in this project" listing (the desktop's file browser, its
// file watcher, emptiness probes), so they cannot diverge on what counts as
// project content:
//
//   - hidden entries (dot-prefixed, e.g. .kapi/, .git/) are skipped, with
//     hidden directories pruned entirely;
//   - entries matched by the project's ignore rules (.kapiignore /
//     KAPI_IGNORE — ignore.ForProjectDir) are skipped the same way.
//
// visit receives each surviving entry — files AND directories — as its
// root-relative path (OS separators) and FileInfo; the root itself is not
// visited. A non-nil error from visit stops the walk and propagates
// (filepath.SkipDir and filepath.SkipAll keep their filepath.Walk meaning).
// Filesystem errors on individual entries are skipped, keeping the walk
// best-effort like a directory listing.
func WalkProjectDir(root string, visit func(rel string, info os.FileInfo) error) error {
	ig := ignore.ForProjectDir(root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort: unreadable entries are skipped
		}
		if path == root {
			// The starting point is not an entry of the project, so none of the
			// rules below apply to it. They cannot: a root given as "." (or as
			// any dot-prefixed directory) reads as a hidden entry, and pruning
			// it returns an empty walk with no error — a caller sees a project
			// with no files rather than a mistake.
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		if ig.Match(filepath.ToSlash(rel), info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return visit(rel, info)
	})
}
