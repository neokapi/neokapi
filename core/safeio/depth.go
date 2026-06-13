package safeio

// DepthGuard bounds recursion depth for recursive-descent parsers. Call
// [DepthGuard.Enter] before descending into a nested structure and
// [DepthGuard.Leave] when unwinding (typically via defer), or wrap the
// recursive step in [DepthGuard.Do]. Once depth would exceed Max, Enter
// returns a [LimitError] wrapping [ErrTooDeep] — an error, never a panic,
// because a Go stack overflow is not recoverable.
//
// A nil *DepthGuard is a valid no-op: every method does nothing and Enter
// returns nil. This lets a reader hold an optional guard without nil checks at
// every call site.
//
// DepthGuard is not safe for concurrent use; give each parsing goroutine its
// own guard.
type DepthGuard struct {
	max   int
	depth int
}

// NewDepthGuard returns a DepthGuard bounded to max levels of nesting. A max
// of 0 (or negative) disables the cap.
func NewDepthGuard(max int) *DepthGuard {
	return &DepthGuard{max: max}
}

// Enter records descent into one level of nesting. It returns a [LimitError]
// wrapping [ErrTooDeep] if that would exceed the configured maximum, in which
// case the depth is left unchanged (the caller must not descend).
func (g *DepthGuard) Enter() error {
	if g == nil || g.max <= 0 {
		return nil
	}
	if g.depth >= g.max {
		return newLimitError(ErrTooDeep, int64(g.max), int64(g.depth)+1, "")
	}
	g.depth++
	return nil
}

// Leave records unwinding from one level of nesting. It never drops below zero,
// so an unbalanced Leave is harmless.
func (g *DepthGuard) Leave() {
	if g == nil {
		return
	}
	if g.depth > 0 {
		g.depth--
	}
}

// Depth reports the current nesting depth.
func (g *DepthGuard) Depth() int {
	if g == nil {
		return 0
	}
	return g.depth
}

// Do runs fn one level deeper, guarding the descent. If entering would exceed
// the maximum depth, fn is not called and the [ErrTooDeep] LimitError is
// returned; otherwise Do returns whatever fn returns and the depth is restored
// on the way out.
func (g *DepthGuard) Do(fn func() error) error {
	if err := g.Enter(); err != nil {
		return err
	}
	defer g.Leave()
	return fn()
}
