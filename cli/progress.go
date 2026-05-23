package cli

import "io"

// progressGroup and progressBar abstract the multi-file progress UI so the
// tool-run pipeline can be compiled for GOOS=js (the browser), where the
// underlying progress-bar library (vbauerster/mpb) doesn't build because it
// relies on terminal ioctls. The non-js implementation lives in
// progress_other.go; the js build gets a no-op from progress_js.go.
//
// progressGroup embeds io.Writer because warnf writes coordinated warnings
// through it (mpb pauses the bar while the line is printed).
type progressGroup interface {
	io.Writer
	Wait()
}

type progressBar interface {
	Increment()
}
