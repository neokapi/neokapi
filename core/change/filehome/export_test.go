package filehome

// SetBeforeCommit sets the hook that runs between the containment check and
// the rename, for a test that swaps a link in there.
func SetBeforeCommit(f func(path string)) { beforeCommit = f }

// EnsureLockDir creates and checks a lock directory as a commit does, for a
// test of a directory no data root can name.
func EnsureLockDir(dir string) error { return ensureLockDir(dir) }
