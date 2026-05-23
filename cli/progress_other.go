//go:build !js

package cli

import (
	"fmt"
	"os"
	"sync/atomic"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
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
