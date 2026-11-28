package change

// AssetFiles is implemented by Assets that write a file when they apply an
// asset operation: the recipe, for a recipe operation. The service takes the
// lock of each such file with the locks of the change set's documents, in one
// sorted order, so the operation applies under a lock the change set already
// holds rather than one it asks for after its documents landed.
type AssetFiles interface {
	// AssetFiles names the files op writes when it is applied.
	AssetFiles(op Op) []string
}

// LockAdder is implemented by a Staged that takes further locks with its
// own: the locks of the files an asset operation of the change set writes.
type LockAdder interface {
	// LockKeysFor names the lock keys of the files at paths, as LockKeys
	// names the stage's own.
	LockKeysFor(paths []string) ([]string, error)
	// AddLockKeys adds keys to the locks the stage takes.
	AddLockKeys(keys []string)
}
