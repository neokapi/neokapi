package openxml

import "github.com/neokapi/neokapi/core/model"

// runBuilder accumulates a []model.Run while parsing OpenXML inline
// content. It coalesces adjacent TextRuns so consecutive text chunks
// produce a single text run.
//
// breakNext, when true, forces the NEXT AddText call to start a fresh
// TextRun instead of coalescing into the previous one — even when the
// previous run is also a TextRun. Use Break() to set the flag from
// callers that need to preserve a heterogeneous-rPr boundary between
// adjacent source runs whose toggle props are identical (so no
// PcOpen/PcClose was emitted between them) but whose non-toggle rPr
// children differ. Mirrors upstream Okapi RunBuilder.java lines
// 73-188 + RunMerger.canRunPropertiesBeMerged (RunMerger.java lines
// 156-229), where heterogeneous RunProperties (toggle OR non-toggle)
// keep runs distinct on the way to the writer. Per ECMA-376-1
// §17.3.2, heterogeneous rPr means heterogeneous runs.
type runBuilder struct {
	runs      []model.Run
	breakNext bool
}

// AppendText adds plain text. If the previous run is a TextRun, the
// new text is appended to it rather than emitting a second adjacent
// TextRun — UNLESS Break() was called since the last AddText, in
// which case a fresh TextRun is started.
func (b *runBuilder) AddText(text string) {
	if text == "" {
		return
	}
	if !b.breakNext {
		if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
			b.runs[n-1].Text.Text += text
			return
		}
	}
	b.breakNext = false
	b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: text}})
}

// Break forces the next AddText to start a new TextRun rather than
// coalescing into the previous one. Used by buildBlock when adjacent
// source runs have identical toggles (so no PcOpen/PcClose break is
// emitted) but divergent non-toggle rPrChildren — the per-source-run
// rPr sidecar (Phase 1) carries one slot per source text-run, so the
// model.Run count must stay aligned with that population for the
// writer's per-run rPr emission (Phase 2) to fire.
//
// Calling Break() multiple times before the next AddText is a no-op
// beyond the first — the boundary is binary, not stacked.
func (b *runBuilder) Break() {
	b.breakNext = true
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
