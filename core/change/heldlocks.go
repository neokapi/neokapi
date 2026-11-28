package change

import (
	"context"
	"slices"
)

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

// heldLocksKey keys the lock keys an applying change set holds in a context.
type heldLocksKey struct{}

// WithHeldLocks returns ctx carrying the lock keys a change set holds while
// it applies its asset operations, so a write those operations make to a
// file whose lock is among them runs under the held lock instead of asking
// for it again.
func WithHeldLocks(ctx context.Context, keys []string) context.Context {
	if len(keys) == 0 {
		return ctx
	}
	return context.WithValue(ctx, heldLocksKey{}, slices.Clone(keys))
}

// HoldsLock reports whether ctx carries key among the lock keys an applying
// change set holds (WithHeldLocks).
func HoldsLock(ctx context.Context, key string) bool {
	keys, _ := ctx.Value(heldLocksKey{}).([]string)
	return slices.Contains(keys, key)
}
