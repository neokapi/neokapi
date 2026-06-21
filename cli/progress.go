package cli

import (
	"fmt"
	"io"
)

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

// downloadProgress renders concurrent multi-file *byte* progress for model-asset
// downloads. Two implementations back it: an mpb multi-bar when stderr is a
// terminal (progress_other.go) and a plain line logger otherwise — including
// the browser, where mpb does not build (progress_js.go). The logger impl lives
// here because it is pure Go and shared by both builds.
type downloadProgress interface {
	// file returns a handle for one file's byte progress. total is the expected
	// size, or <= 0 when unknown.
	file(name string, total int64) downloadFile
	// wait blocks until every bar has drained (mpb) / no-op (logger).
	wait()
}

// downloadFile tracks one file within a downloadProgress.
type downloadFile interface {
	// wrap returns a reader that advances this file's progress as it is read.
	wrap(r io.Reader) io.Reader
	// done finalizes the file at n bytes on success.
	done(n int64)
	// abort removes/cancels the file's bar on failure.
	abort()
}

// logProgress is the non-terminal downloadProgress: one "downloading" line when
// a file starts and one "downloaded" line when it finishes — no per-chunk spam,
// so concurrent CI logs stay readable. It is shared by the !js and js builds.
type logProgress struct {
	logf func(format string, args ...any)
}

func (lp logProgress) file(name string, total int64) downloadFile {
	if lp.logf == nil {
		return logFile{}
	}
	if total > 0 {
		lp.logf("downloading %s (%s)", name, humanBytes(total))
	} else {
		lp.logf("downloading %s", name)
	}
	return logFile{logf: lp.logf, name: name}
}

func (lp logProgress) wait() {}

type logFile struct {
	logf func(format string, args ...any)
	name string
}

func (f logFile) wrap(r io.Reader) io.Reader { return r } // no per-chunk lines
func (f logFile) done(n int64) {
	if f.logf != nil {
		f.logf("downloaded %s (%s)", f.name, humanBytes(n))
	}
}
func (f logFile) abort() {}

// humanBytes formats a byte count as a short human-readable string (e.g. 1.6GB).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
