package storage

// A database file belongs to the driver that holds it. Natively that is a file
// on disk beside its write-ahead log; in the browser it is a database in the
// SQLite WebAssembly module's memory, which the page's file system never sees.
// Code that asks whether a database is there, deletes one, moves one or looks
// for the databases in a directory therefore asks this package instead of
// reaching for os.Stat, os.Remove, os.Rename or os.ReadDir on a database path.
//
// Each function takes the path a caller would hand Open. Everything else beside
// a database (a recipe, a lock file, a JSON sidecar) stays a plain file and is
// read and written through os.

// Exists reports whether a database is held at path.
func Exists(path string) (bool, error) { return dbExists(path) }

// Remove deletes the database at path together with the journal files beside
// it. A database that is not there is not an error. Close every pool on the
// database first: the browser driver refuses to remove one that is open.
func Remove(path string) error { return dbRemove(path) }

// Rename moves the database at from to to, replacing whatever database was
// there. Both must be closed.
func Rename(from, to string) error { return dbRename(from, to) }

// List returns the databases at or below dir, sorted. A directory that does
// not exist holds none.
func List(dir string) ([]string, error) { return dbList(dir) }

// RemoveAll deletes every database at or below dir. It is what a caller that
// clears a directory with os.RemoveAll runs first, so a database the driver
// keeps outside the file system goes with the directory.
func RemoveAll(dir string) error {
	paths, err := List(dir)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if err := Remove(p); err != nil {
			return err
		}
	}
	return nil
}
