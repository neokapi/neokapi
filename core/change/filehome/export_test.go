package filehome

// SetBeforeCommit sets the hook that runs between the containment check and
// the rename, for a test that swaps a link in there.
func SetBeforeCommit(f func(path string)) { beforeCommit = f }
