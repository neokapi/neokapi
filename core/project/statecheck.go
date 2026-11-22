package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/rootpath"
)

// The state directory (.kapi/) and the directories kapi keeps inside it are
// read and written by path: SQLite opens its databases by name, and the
// caches and vaults are kapi's own files. A repository could still commit a
// .kapi/ entry, or one beneath it, that is a symbolic link leading out of the
// project, and every state write would follow it. CheckState refuses such a
// layout before anything reads or writes there, so the by-path access that
// follows stays inside the project.

// CheckState reports an error when the state directory, or a directory kapi
// keeps beneath it, resolves outside the project root. A directory that does
// not exist yet passes: it is created beneath a state directory that passed.
func (l Layout) CheckState() error {
	if l.Root == "" {
		return nil
	}
	for _, dir := range []string{l.StateDir, l.WorkDir(), l.CacheDir(), l.VaultDir(), l.SyncDir(), l.ExtractionsDir(), l.CollectionsDir()} {
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if _, err := rootpath.Contain(l.Root, dir); err != nil {
			rel, rerr := filepath.Rel(l.Root, dir)
			if rerr != nil {
				rel = filepath.Base(dir)
			}
			return fmt.Errorf("project: %s leads outside the project, so kapi keeps no state there. Remove the link and run again: %w", filepath.ToSlash(rel), err)
		}
	}
	return nil
}
