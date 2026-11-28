package format

import (
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// ContentKeeper is implemented by a writer whose format does not keep every
// character of a block's content. A Markdown paragraph holds no white space
// at its ends: written with a space after its last word, it reads back
// without one.
//
// The change service asks it about an edit to content a file of the format
// holds. When the edit, as the format keeps it, is the content the file
// holds, the edit is unchanged. A writer that does not implement
// ContentKeeper declares that its format keeps content whole, and an edit its
// file does not take is refused as dropped. The operations matrix
// (core/formats/opsmatrix_kept_test.go) holds every built-in format to its
// declaration.
type ContentKeeper interface {
	// KeptRuns returns the part of runs the format keeps when they are the
	// content of b, the block as the format's reader read it. Content the
	// reader gave is kept whole, so KeptRuns returns it as it is, and
	// KeptRuns of its own result is that result. It never changes runs.
	KeptRuns(b *model.Block, runs []model.Run) []model.Run
}

// TrimRunEdges returns a copy of runs without the characters of cutset at
// the start of the content and at its end, for a ContentKeeper whose format
// keeps none there. Only text at an end is trimmed: an inline code, a plural
// or a select there ends the trim. A text run the trim empties is left out.
func TrimRunEdges(runs []model.Run, cutset string) []model.Run {
	out := slices.Clone(runs)
	for len(out) > 0 && out[0].Text != nil {
		if text := strings.TrimLeft(out[0].Text.Text, cutset); text != "" {
			out[0] = withText(out[0], text)
			break
		}
		out = out[1:]
	}
	for n := len(out); n > 0 && out[n-1].Text != nil; n = len(out) {
		if text := strings.TrimRight(out[n-1].Text.Text, cutset); text != "" {
			out[n-1] = withText(out[n-1], text)
			break
		}
		out = out[:n-1]
	}
	return out
}

// withText returns the text run r holding text, as a copy when the text
// differs.
func withText(r model.Run, text string) model.Run {
	if r.Text.Text == text {
		return r
	}
	t := *r.Text
	t.Text = text
	r.Text = &t
	return r
}
