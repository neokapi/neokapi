//go:build !js

package cli

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"golang.org/x/term"
)

// newProgress builds an mpb-backed multi-file progress bar. active is read by
// the trailing "(N active)" decorator; the caller increments/decrements it
// around each file.
func newProgress(numFiles int, active *atomic.Int64) (progressGroup, progressBar) {
	progress := mpb.New(mpb.WithOutput(os.Stderr))
	bar := progress.New(int64(numFiles),
		mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding(" ").Rbound("]"),
		mpb.PrependDecorators(decor.Elapsed(decor.ET_STYLE_GO)),
		mpb.AppendDecorators(
			decor.CountersNoUnit("[%d/%d]"),
			decor.Any(func(decor.Statistics) string {
				n := active.Load()
				if n > 0 {
					return fmt.Sprintf(" (%d active)", n)
				}
				return ""
			}),
		),
	)
	return progress, bar
}

// newDownloadProgress returns the multi-file byte-progress renderer for model
// downloads: an mpb multi-bar when stderr is a real terminal, else the plain
// line logger (CI, pipes) so output stays readable. logf receives the line
// logger's messages in the non-terminal case.
func newDownloadProgress(logf func(format string, args ...any)) downloadProgress {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return logProgress{logf: logf}
	}
	return &mpbDownload{p: mpb.New(mpb.WithOutput(os.Stderr))}
}

// mpbDownload renders one byte bar per concurrent file via mpb.
type mpbDownload struct{ p *mpb.Progress }

func (m *mpbDownload) file(name string, total int64) downloadFile {
	if total <= 0 {
		// Unknown size: a spinner with a running byte counter instead of a bar.
		bar := m.p.New(0, mpb.SpinnerStyle(),
			mpb.PrependDecorators(decor.Name(name+"  ")),
			mpb.AppendDecorators(decor.CurrentKibiByte("% .1f")),
		)
		return &mpbFile{bar: bar}
	}
	bar := m.p.New(total,
		mpb.BarStyle().Lbound("[").Filler("=").Tip(">").Padding(" ").Rbound("]"),
		mpb.PrependDecorators(
			decor.Name(name+"  "),
			decor.Percentage(decor.WC{W: 5}),
		),
		mpb.AppendDecorators(
			decor.CountersKibiByte("% .1f / % .1f"),
			decor.Name("  "),
			decor.AverageSpeed(decor.SizeB1024(0), "% .1f"),
		),
	)
	return &mpbFile{bar: bar}
}

func (m *mpbDownload) wait() { m.p.Wait() }

type mpbFile struct{ bar *mpb.Bar }

func (f *mpbFile) wrap(r io.Reader) io.Reader { return f.bar.ProxyReader(r) }
func (f *mpbFile) done(n int64) {
	// Settle the bar at the actual byte count so an over/under-estimated total
	// still completes cleanly.
	f.bar.SetTotal(n, true)
}
func (f *mpbFile) abort() { f.bar.Abort(true) }
