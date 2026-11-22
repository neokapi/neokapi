package host

import (
	"fmt"

	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// NoStoreError is a command that only reads a store, pointed at a standalone
// store file that does not exist. Opening the file would create an empty store
// as a side effect of a read, so the read refuses instead.
type NoStoreError struct {
	// Kind names the store: "content memory" or "terms store".
	Kind string
	// Path is the store file that does not exist.
	Path string
	// Import is the command that creates one.
	Import string
}

func (e *NoStoreError) Error() string {
	return fmt.Sprintf("there is no %s at %s. %s creates one, or pass -p to read a project's own", e.Kind, e.Path, e.Import)
}

// missingStore returns a NoStoreError when no database is held at path.
func missingStore(path, kind, importCmd string) error {
	if !storeExists(path) {
		return &NoStoreError{Kind: kind, Path: path, Import: importCmd}
	}
	return nil
}

// storeExists reports whether a database is held at path. It asks the storage
// driver, which in the browser holds a database outside the file system the
// program sees. A path the driver cannot judge counts as present, and the
// open reports it.
func storeExists(path string) bool {
	ok, err := storage.Exists(path)
	return err != nil || ok
}

// OpenMemoryForRead is OpenMemorySQLite for a command that only reads the
// content memory, and it creates nothing. A project whose store nothing has
// created yet answers from an empty content memory held in memory. A
// standalone store file that does not exist is a NoStoreError.
func (a *App) OpenMemoryForRead(cmd Command) (memory.Store, string, func(), error) {
	sel, err := a.ResolveMemoryStore(cmd)
	if err != nil {
		return nil, "", func() {}, err
	}
	if sel.InProject() {
		if path := a.ProjectStorePath(sel.Root); !storeExists(path) {
			tm, err := memory.NewSQLiteStore(":memory:")
			if err != nil {
				return nil, "", func() {}, fmt.Errorf("open content memory: %w", err)
			}
			return tm, path, func() { _ = tm.Close() }, nil
		}
	} else if err := missingStore(sel.Path, "content memory", "kapi memory import"); err != nil {
		return nil, sel.Path, func() {}, err
	}
	return a.OpenMemorySQLite(cmd)
}

// OpenTermsForRead is OpenTermsSQLite for a command that only reads the terms
// store, and it creates nothing. A project whose store nothing has created yet
// answers from an empty terms store held in memory. A standalone store file
// that does not exist is a NoStoreError.
func (a *App) OpenTermsForRead(cmd Command) (terms.Terminology, string, func(), error) {
	sel, err := a.ResolveTermsCmdStore(cmd)
	if err != nil {
		return nil, "", func() {}, err
	}
	switch {
	case sel.InProject():
		if path := a.ProjectStorePath(sel.Root); !storeExists(path) {
			tb, err := terms.NewSQLiteStore(":memory:")
			if err != nil {
				return nil, "", func() {}, fmt.Errorf("open terms: %w", err)
			}
			return tb, path, func() { _ = tb.Close() }, nil
		}
	case sel.Path != "":
		if err := missingStore(sel.Path, "terms store", "kapi terms import"); err != nil {
			return nil, sel.Path, func() {}, err
		}
	}
	return a.OpenTermsSQLite(cmd)
}
