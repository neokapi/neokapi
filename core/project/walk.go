package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/ignore"
)

// WalkProjectDir walks the project tree rooted at root in lexical order. It
// is the one walk behind every listing of project files (the desktop's file
// browser, its file watcher, emptiness probes), so they agree on what counts
// as project content. Hidden entries (.kapi/, .git/) and entries the ignore
// rules match (ignore.ForProjectDir) are skipped, directories with all they
// hold.
//
// visit receives each other entry, file or directory, as its root-relative
// path and FileInfo; the root itself is not visited. An error from visit
// stops the walk and is returned; filepath.SkipDir and filepath.SkipAll keep
// their usual meaning. An entry that cannot be read is skipped.
//
// The walk lists through an os.Root opened on root. A symbolic link is
// visited as the link it is and never descended into, and a directory
// swapped for a link during the walk cannot lead the listing out of root.
func WalkProjectDir(root string, visit func(rel string, info os.FileInfo) error) error {
	ig := ignore.ForProjectDir(root)
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil //nolint:nilerr // best-effort: an unreadable root lists nothing
	}
	defer r.Close()
	return fs.WalkDir(r.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort: unreadable entries are skipped
		}
		if path == "." {
			// The starting point is not an entry of the project, so none of the
			// rules below apply to it. A root given as "." reads as a hidden
			// entry, and pruning it would return an empty walk with no error.
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ig.Match(path, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil //nolint:nilerr // best-effort: an entry gone since the listing is skipped
		}
		return visit(filepath.FromSlash(path), info)
	})
}
