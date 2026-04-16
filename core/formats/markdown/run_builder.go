package markdown

import "github.com/neokapi/neokapi/core/model"

// runBuilder accumulates a []model.Run while walking the Markdown AST.
// It coalesces adjacent TextRuns so consecutive text nodes produce a
// single text run, matching the behaviour of model.Fragment.AppendText
// followed by model.FragmentToRuns.
//
// The builder is intentionally unexported — it exists only to let the
// markdown reader emit the Runs shape directly, avoiding a Fragment
// round-trip on every parse.
type runBuilder struct {
	runs     []model.Run
	hasSpans bool
}

// newRunBuilder returns a fresh runBuilder.
func newRunBuilder() *runBuilder {
	return &runBuilder{}
}

// AppendText adds plain text. If the previous run is a TextRun, the new
// text is appended to it rather than emitting a second adjacent TextRun.
func (b *runBuilder) AppendText(text string) {
	if text == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += text
		return
	}
	b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: text}})
}

// AppendPh emits a self-closing placeholder run with the given metadata
// and editing constraints. Mirrors the shape FragmentToRuns would
// produce for a model.Span with SpanType == SpanPlaceholder.
func (b *runBuilder) AppendPh(id, semType, subType, data, disp, equiv string, deletable, cloneable, reorderable bool) {
	b.hasSpans = true
	b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Disp:    disp,
		Equiv:   equiv,
		Constraints: &model.RunConstraints{
			Deletable:   deletable,
			Cloneable:   cloneable,
			Reorderable: reorderable,
		},
	}})
}

// AppendPcOpen emits the opening half of a paired code. Mirrors the
// shape FragmentToRuns would produce for a model.Span with
// SpanType == SpanOpening.
func (b *runBuilder) AppendPcOpen(id, semType, subType, data, disp, equiv string, deletable, cloneable, reorderable bool) {
	b.hasSpans = true
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Disp:    disp,
		Equiv:   equiv,
		Constraints: &model.RunConstraints{
			Deletable:   deletable,
			Cloneable:   cloneable,
			Reorderable: reorderable,
		},
	}})
}

// AppendPcClose emits the closing half of a paired code. PcCloseRun has
// no Disp or Constraints fields — the closing half inherits behaviour
// from the opening and mirrors FragmentToRuns for SpanClosing spans.
func (b *runBuilder) AppendPcClose(id, semType, subType, data, equiv string) {
	b.hasSpans = true
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Equiv:   equiv,
	}})
}

// HasSpans reports whether any non-text run (placeholder, paired code
// open/close) has been accumulated. Mirrors Fragment.HasSpans — used
// by the reader to decide whether to emit a runs-backed segment.
func (b *runBuilder) HasSpans() bool {
	return b.hasSpans
}

// Runs returns the accumulated run slice. Always returns a non-nil
// slice, even when empty, so callers can distinguish "empty content"
// from "no content at all" via a nil check on the returned value.
func (b *runBuilder) Runs() []model.Run {
	if b.runs == nil {
		return []model.Run{}
	}
	return b.runs
}
