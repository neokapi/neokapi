package resilience

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrUnavailable is the sentinel every open-circuit rejection matches. Callers
// branch on errors.Is(err, resilience.ErrUnavailable) to degrade; the concrete
// [UnavailableError] carries the detail needed to render or defer.
var ErrUnavailable = errors.New("dependency unavailable")

// UnavailableError reports that a call was rejected *without being attempted*
// because the dependency's circuit is open. It is deliberately distinct from a
// call that was attempted and failed: nothing was sent upstream, no quota was
// consumed, and the caller is expected to degrade — queue the work, or tell the
// user it will be retried — rather than treat it as a hard failure.
type UnavailableError struct {
	// Dependency is the breaker name, e.g. "ai:bedrock" or "connector:figma".
	Dependency string
	// Kind is the dependency class, used to pick the user-facing wording.
	Kind Kind
	// RetryAfter is how long until the breaker next admits a probe. It is an
	// estimate derived from the cooldown, never negative, and always at least a
	// second so a caller can put it straight into a Retry-After header.
	RetryAfter time.Duration
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("%s is unavailable (circuit open, retry in %s)",
		e.Dependency, e.RetryAfter.Round(time.Second))
}

// Unwrap makes errors.Is(err, ErrUnavailable) true.
func (e *UnavailableError) Unwrap() error { return ErrUnavailable }

// RetryAfterSeconds is RetryAfter as whole seconds, rounded up and floored at
// 1 — the shape an HTTP Retry-After header and a queue delay both want.
func (e *UnavailableError) RetryAfterSeconds() int {
	s := int(math.Ceil(e.RetryAfter.Seconds()))
	if s < 1 {
		return 1
	}
	return s
}

// AsUnavailable extracts the [UnavailableError] from an error chain.
func AsUnavailable(err error) (*UnavailableError, bool) {
	var u *UnavailableError
	ok := errors.As(err, &u)
	return u, ok
}

// IsUnavailable reports whether err is (or wraps) an open-circuit rejection.
func IsUnavailable(err error) bool { return errors.Is(err, ErrUnavailable) }

// callerFaultError marks an error as attributable to the caller — a malformed
// request, a bad credential, an exhausted quota — rather than to the health of
// the dependency. It is counted as neither success nor failure, so one
// workspace's misconfiguration can never trip the breaker shared by all of them.
type callerFaultError struct{ err error }

func (e *callerFaultError) Error() string { return e.err.Error() }
func (e *callerFaultError) Unwrap() error { return e.err }

// CallerFault marks err as a caller-side fault so it does not count against the
// breaker. Use it when a call site knows more than the shared classifier does —
// for example a connector that has already read a 401 off the response.
// CallerFault(nil) is nil.
func CallerFault(err error) error {
	if err == nil {
		return nil
	}
	return &callerFaultError{err: err}
}

// isCallerFault reports whether err was explicitly marked with [CallerFault].
func isCallerFault(err error) bool {
	var c *callerFaultError
	return errors.As(err, &c)
}
