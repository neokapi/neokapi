// Package heldlock lets the change service and the file home tell a commit
// that the lock of its file is already held by the call it runs inside, so it
// writes under that lock instead of asking for it again.
//
// A hold is a live value, not a name: Release marks it released, and a
// context that still carries it then says nothing, so a context kept past the
// hold, or used by a goroutine that outlives it, commits like any other caller
// and takes the lock. Only packages under core/change can construct one: the
// package is internal to core/change.
package heldlock

import (
	"context"
	"sync/atomic"
)

// Hold is one held lock, by its key, until Release.
type Hold struct {
	key      string
	released atomic.Bool
}

// New is the hold of the lock with key, which the caller holds now.
func New(key string) *Hold { return &Hold{key: key} }

// Release marks the hold released. The caller calls it before it gives the
// lock back.
func (h *Hold) Release() {
	if h != nil {
		h.released.Store(true)
	}
}

// ctxKey keys the holds a context carries.
type ctxKey struct{}

// With returns ctx carrying holds beside those it already carries.
func With(ctx context.Context, holds ...*Hold) context.Context {
	if len(holds) == 0 {
		return ctx
	}
	prev, _ := ctx.Value(ctxKey{}).([]*Hold)
	all := make([]*Hold, 0, len(prev)+len(holds))
	all = append(all, prev...)
	all = append(all, holds...)
	return context.WithValue(ctx, ctxKey{}, all)
}

// Holds reports whether ctx carries a hold of key that is not released.
func Holds(ctx context.Context, key string) bool {
	holds, _ := ctx.Value(ctxKey{}).([]*Hold)
	for _, h := range holds {
		if h.key == key && !h.released.Load() {
			return true
		}
	}
	return false
}
