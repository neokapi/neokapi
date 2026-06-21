package markdown

import "github.com/neokapi/neokapi/core/model"

// runBuilder accumulates a []model.Run while walking the Markdown
// AST. It coalesces adjacent TextRuns so consecutive text nodes
// produce a single text run.
type runBuilder struct {
	runs           []model.Run
	hasInlineCodes bool
}

// newRunBuilder returns a fresh runBuilder.
func newRunBuilder() *runBuilder {
	return &runBuilder{}
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

// AddPh emits a self-closing placeholder run with the given metadata
// and editing constraints.
func (b *runBuilder) AddPh(id, semType, subType, data, disp, equiv string, deletable, cloneable, reorderable bool) {
	b.hasInlineCodes = true
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

// AddPcOpen emits the opening half of a paired code.
func (b *runBuilder) AddPcOpen(id, semType, subType, data, disp, equiv string, deletable, cloneable, reorderable bool) {
	b.hasInlineCodes = true
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

// AddPcClose emits the closing half of a paired code. PcCloseRun has
// no Disp or Constraints fields — the closing half inherits behaviour
// from the opening.
func (b *runBuilder) AddPcClose(id, semType, subType, data, equiv string) {
	b.hasInlineCodes = true
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Equiv:   equiv,
	}})
}

// SetLastAttrs attaches format-neutral attributes (href/src/alt/title) to the
// most recently appended PcOpen or Ph run, so a writer for a different format
// can re-synthesize a link/image natively on the cross-format path. No-op when
// attrs is empty or the last run is neither a PcOpen nor a Ph.
func (b *runBuilder) SetLastAttrs(attrs map[string]string) {
	if len(attrs) == 0 || len(b.runs) == 0 {
		return
	}
	last := &b.runs[len(b.runs)-1]
	switch {
	case last.PcOpen != nil:
		last.PcOpen.Attrs = attrs
	case last.Ph != nil:
		last.Ph.Attrs = attrs
	}
}

// HasInlineCodes reports whether any non-text run (placeholder, paired
// code open/close) has been accumulated. Used by the reader to decide
// whether to emit a runs-backed segment.
func (b *runBuilder) HasInlineCodes() bool {
	return b.hasInlineCodes
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
