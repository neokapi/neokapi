package openxml

import "github.com/neokapi/neokapi/core/model"

// runBuilder accumulates a []model.Run while parsing OpenXML inline
// content. It coalesces adjacent TextRuns so consecutive text chunks
// produce a single text run, matching the behavior of
// the runBuilder pattern used by other format readers.
//
// The builder is intentionally unexported — it exists only to let the
// WML / DML / SML parsers emit the Runs shape directly, avoiding a
// Fragment round-trip at import time. Each Append* method mirrors the
// Run that model.MarshalRuns would produce for the equivalent
// Fragment + Span pair.
type runBuilder struct {
	runs []model.Run
}

// AppendText adds plain text. If the previous run is a TextRun, the
// new text is appended to it rather than emitting a second adjacent
// TextRun.
func (b *runBuilder) AddText(text string) {
	if text == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += text
		return
	}
	b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: text}})
}

// AppendPh emits a PlaceholderRun mirroring a SpanPlaceholder. The
// constraint booleans map directly onto RunConstraints (Deletable,
// Cloneable, Reorderable), preserving the behavior of
// MarshalRuns on a matching Span.
func (b *runBuilder) AddPh(id, semType, subType, data, equiv, disp string, deletable, cloneable, reorderable bool) {
	b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Equiv:   equiv,
		Disp:    disp,
		Constraints: &model.RunConstraints{
			Deletable:   deletable,
			Cloneable:   cloneable,
			Reorderable: reorderable,
		},
	}})
}

// AppendPcOpen emits the opening half of a paired code mirroring a
// SpanOpening.
func (b *runBuilder) AddPcOpen(id, semType, subType, data, equiv, disp string, deletable, cloneable, reorderable bool) {
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Equiv:   equiv,
		Disp:    disp,
		Constraints: &model.RunConstraints{
			Deletable:   deletable,
			Cloneable:   cloneable,
			Reorderable: reorderable,
		},
	}})
}

// AppendPcClose emits the closing half of a paired code mirroring a
// SpanClosing. PcCloseRun has no Constraints field — the closing half
// inherits behavior from the opening.
func (b *runBuilder) AddPcClose(id, semType, subType, data, equiv string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Equiv:   equiv,
	}})
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
