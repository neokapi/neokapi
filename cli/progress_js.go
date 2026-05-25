//go:build js

package cli

import "sync/atomic"

// newProgress is a no-op in the browser: there's no terminal to draw a
// progress bar on, and mpb doesn't build for GOOS=js. Returning nil leaves
// the tool-run pipeline's nil-guards (`if progress != nil`, `if bar != nil`)
// to skip all progress reporting.
func newProgress(_ int, _ *atomic.Int64) (progressGroup, progressBar) {
	return nil, nil
}
