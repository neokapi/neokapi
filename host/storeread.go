package host

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// NoStoreError is a command that only reads a store, pointed at a store that
// does not exist yet: a standalone store file, or a project whose work
// directory nothing has opened. Opening it would create the store as a side
// effect of a read, so the read refuses instead.
type NoStoreError struct {
	// Kind names the store: "content memory" or "terms store".
	Kind string
	// Path is the store file that does not exist.
	Path string
	// Import is the command that creates one.
	Import string
	// Project is the root of the project that has no store yet, empty for a
	// standalone store file.
	Project string
}

func (e *NoStoreError) Error() string {
	if e.Project != "" {
		return fmt.Sprintf("there is no %s in the project at %s yet. %s or kapi up creates it", e.Kind, e.Project, e.Import)
	}
	return fmt.Sprintf("there is no %s at %s. %s creates one, or pass -p to read a project's own", e.Kind, e.Path, e.Import)
}

// missingProjectStore returns a NoStoreError when the project at root has no
// store yet: nothing has opened its work directory. Opening it to read would
// create the store and register the project in the workspace.
func (a *App) missingProjectStore(root, kind, importCmd string) error {
	if _, err := os.Stat(a.ProjectStorePath(root)); errors.Is(err, fs.ErrNotExist) {
		return &NoStoreError{Kind: kind, Project: root, Import: importCmd}
	}
	return nil
}

// missingStore returns a NoStoreError when path names no file.
func missingStore(path, kind, importCmd string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return &NoStoreError{Kind: kind, Path: path, Import: importCmd}
	}
	return nil
}

// OpenMemoryForRead is OpenMemorySQLite for a command that only reads the
// content memory. A standalone store file that does not exist is a
// NoStoreError, and nothing is created.
func (a *App) OpenMemoryForRead(cmd Command) (memory.Store, string, func(), error) {
	sel, err := a.ResolveMemoryStore(cmd)
	if err != nil {
		return nil, "", func() {}, err
	}
	if sel.InProject() {
		if err := a.missingProjectStore(sel.Root, "content memory", "kapi store import"); err != nil {
			return nil, "", func() {}, err
		}
	} else if err := missingStore(sel.Path, "content memory", "kapi memory import"); err != nil {
		return nil, sel.Path, func() {}, err
	}
	return a.OpenMemorySQLite(cmd)
}

// OpenTermsForRead is OpenTermsSQLite for a command that only reads the terms
// store. A standalone store file that does not exist is a NoStoreError, and
// nothing is created.
func (a *App) OpenTermsForRead(cmd Command) (terms.Terminology, string, func(), error) {
	sel, err := a.ResolveTermsCmdStore(cmd)
	if err != nil {
		return nil, "", func() {}, err
	}
	switch {
	case sel.InProject():
		if err := a.missingProjectStore(sel.Root, "terms store", "kapi store import"); err != nil {
			return nil, "", func() {}, err
		}
	case sel.Path != "":
		if err := missingStore(sel.Path, "terms store", "kapi terms import"); err != nil {
			return nil, sel.Path, func() {}, err
		}
	}
	return a.OpenTermsSQLite(cmd)
}
