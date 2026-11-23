package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/rootpath"
)

// The committed .kapi/ directory in a checkout (StateDir) holds source a
// project keeps there, such as a terms bundle or a voice profile, which
// `kapi store import` reads by path. A repository could commit a .kapi/ entry
// that is a symbolic link and lead that read out of the project. CheckState
// refuses such a layout before anything is read there. kapi keeps none of its
// own state in .kapi/: the checkout's state directory is under the data root.

// CheckState reports an error when the committed .kapi/ directory, or anything
// beneath it, is a symbolic link or another kind of file that is neither a
// directory nor a regular file, or when it resolves outside the project root.
// It reads the file system only. A .kapi/ that does not exist passes.
func (l Layout) CheckState() error {
	if l.Root == "" {
		return nil
	}
	if _, err := os.Lstat(l.StateDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	// A layout kapi places outside the project (the desktop keeps the state of
	// a folder with no recipe in its own data directory) is checked beneath
	// itself: no repository holds it, so git is not asked about it.
	inProject := rootpath.IsBeneath(l.Root, l.StateDir)
	containIn := l.Root
	if !inProject {
		containIn = l.StateDir
	}
	real, err := rootpath.Contain(containIn, l.StateDir)
	if err != nil {
		return stateError(l.Root, l.StateDir, "leads outside the project", err)
	}
	// The state directory may itself be a link that stays inside the project;
	// beneath it, nothing is.
	name := func(p string) string {
		rel, rerr := filepath.Rel(real, p)
		if rerr != nil {
			return l.StateDir
		}
		return filepath.Join(l.StateDir, rel)
	}
	err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // removed while the walk ran
			}
			return err
		}
		switch t := d.Type(); {
		case t&fs.ModeSymlink != 0:
			return stateError(l.Root, name(p), "is a symbolic link", nil)
		case t.IsDir(), t.IsRegular():
			return nil
		default:
			return stateError(l.Root, name(p), "is not a regular file", nil)
		}
	})
	return err
}

// stateError names p relative to root in a refusal of the state layout.
func stateError(root, p, what string, cause error) error {
	rel, rerr := filepath.Rel(root, p)
	if rerr != nil {
		rel = filepath.Base(p)
	}
	msg := fmt.Sprintf("project: %s %s, so kapi keeps no state there. Remove it and run again", filepath.ToSlash(rel), what)
	if cause != nil {
		return fmt.Errorf("%s: %w", msg, cause)
	}
	return errors.New(msg)
}
