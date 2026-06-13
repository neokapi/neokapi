package safeio

import (
	"errors"
	"fmt"
)

// Sentinel errors. Every limit breach returns a *[LimitError] whose Unwrap
// returns one of these, so callers can branch with errors.Is without depending
// on the concrete type.
var (
	// ErrByteBudget is returned when a reader or writer exceeds its total
	// byte budget.
	ErrByteBudget = errors.New("safeio: byte budget exceeded")

	// ErrTooDeep is returned when recursion exceeds the configured maximum
	// nesting depth. Returned as an error (never a stack-overflow panic),
	// because a Go stack overflow is not recoverable.
	ErrTooDeep = errors.New("safeio: maximum nesting depth exceeded")

	// ErrEntryTooLarge is returned when a single zip entry's uncompressed
	// size exceeds the per-entry maximum (POI ZipSecureFile MaxEntrySize).
	ErrEntryTooLarge = errors.New("safeio: zip entry exceeds maximum uncompressed size")

	// ErrTotalTooLarge is returned when the cumulative uncompressed size of a
	// zip archive exceeds the total maximum.
	ErrTotalTooLarge = errors.New("safeio: zip total uncompressed size exceeds maximum")

	// ErrInflateRatio is returned when a zip entry's compression ratio is
	// better than the minimum inflate ratio — the classic zip-bomb signal
	// (POI ZipSecureFile MinInflateRatio).
	ErrInflateRatio = errors.New("safeio: zip entry compression ratio too high (possible zip bomb)")

	// ErrTooManyEntries is returned when a zip archive declares more entries
	// than the configured maximum.
	ErrTooManyEntries = errors.New("safeio: zip entry count exceeds maximum")

	// ErrPathEscape is returned when a content-derived path is absolute or
	// escapes its containing root via "..".
	ErrPathEscape = errors.New("safeio: path escapes root")
)

// LimitError is the concrete error type returned by every safeio limit breach.
// It carries the configured limit, the observed value that breached it, and an
// optional name (a zip entry name or path) for diagnostics. It unwraps to one
// of the package sentinel errors so errors.Is works:
//
//	if errors.Is(err, safeio.ErrInflateRatio) { ... }
type LimitError struct {
	// Limit is the configured bound that was exceeded.
	Limit int64
	// Got is the observed value that breached the bound. Best effort: when a
	// breach is detected at a streaming boundary, Got is the running total at
	// the point of detection, which may equal Limit+1.
	Got int64
	// Name is optional context — a zip entry name, a rejected path, etc.
	Name string

	base error // sentinel returned by Unwrap
}

// Error implements error.
func (e *LimitError) Error() string {
	var b string
	if e.Name != "" {
		b = fmt.Sprintf("%s: %q", e.base.Error(), e.Name)
	} else {
		b = e.base.Error()
	}
	// Ratio breaches carry their numbers in Limit/Got as scaled integers that
	// are not meaningful to print, so only append byte/depth/count figures.
	switch {
	case errors.Is(e.base, ErrInflateRatio):
		return b
	default:
		if e.Limit > 0 {
			return fmt.Sprintf("%s (limit %d, got %d)", b, e.Limit, e.Got)
		}
		return b
	}
}

// Unwrap returns the sentinel error for errors.Is.
func (e *LimitError) Unwrap() error { return e.base }

// newLimitError builds a *LimitError wrapping the given sentinel.
func newLimitError(base error, limit, got int64, name string) *LimitError {
	return &LimitError{Limit: limit, Got: got, Name: name, base: base}
}
