package sievepen

import "github.com/neokapi/neokapi/core/model"

// runBuilder accumulates a []model.Run while parsing TMX inline content.
// It coalesces adjacent TextRuns so consecutive xml.CharData chunks produce
// a single text run; mirrors the runBuilder pattern used by other format readers.
//
// The builder is intentionally unexported — it exists only to let the TMX
// parser emit the Runs shape directly, avoiding a Fragment round-trip at
// import time.
type runBuilder struct {
	runs []model.Run
}

// AppendText adds plain text. If the previous run is a TextRun, the new
// text is appended to it rather than emitting a second adjacent TextRun.
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

// AppendPh emits a PlaceholderRun with the default zero-valued constraints
// (Deletable/Cloneable/Reorderable all false).
func (b *runBuilder) AddPh(id, semType, subType, data string) {
	b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
		ID:          id,
		Type:        semType,
		SubType:     subType,
		Data:        data,
		Constraints: &model.RunConstraints{},
	}})
}

// AppendPcOpen emits the opening half of a paired code.
func (b *runBuilder) AddPcOpen(id, semType, subType, data string) {
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID:          id,
		Type:        semType,
		SubType:     subType,
		Data:        data,
		Constraints: &model.RunConstraints{},
	}})
}

// AppendPcClose emits the closing half of a paired code. PcCloseRun has no
// Constraints field — the closing half inherits behavior from the opening.
func (b *runBuilder) AddPcClose(id, semType, subType, data string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
	}})
}

// AppendSub emits a sub-block reference run.
func (b *runBuilder) AppendSub(id, ref string) {
	b.runs = append(b.runs, model.Run{Sub: &model.SubRun{
		ID:  id,
		Ref: ref,
	}})
}

// Runs returns the accumulated run slice. Always returns a non-nil slice,
// even when empty, so callers can distinguish "empty content" from "no
// content at all" via a nil check on the returned value.
func (b *runBuilder) Runs() []model.Run {
	if b.runs == nil {
		return []model.Run{}
	}
	return b.runs
}
