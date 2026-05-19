package odf

import "github.com/neokapi/neokapi/core/model"

// runBuilder accumulates a []model.Run while walking ODF XML elements.
// Adjacent text chunks coalesce into a single TextRun, mirroring the
// behaviour of the html/markdown/openxml run builders.
type runBuilder struct {
	runs []model.Run
}

func newRunBuilder() *runBuilder {
	return &runBuilder{}
}

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

func (b *runBuilder) AddPcOpen(id, semType, data string) {
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID:   id,
		Type: semType,
		Data: data,
	}})
}

// AddPh adds a placeholder run carrying the original self-closing
// markup as Data (e.g. `<text:line-break/>`). Mirrors upstream Okapi's
// TagType.PLACEHOLDER for self-closing inline elements like line-break.
func (b *runBuilder) AddPh(id, semType, data string) {
	b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
		ID:   id,
		Type: semType,
		Data: data,
	}})
}

func (b *runBuilder) AddPcClose(id, semType string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:   id,
		Type: semType,
	}})
}

// AddPcCloseData is like AddPcClose but also captures the original
// closing-tag markup in Data so the writer can splice it back into the
// reconstructed XML verbatim. Used by the generic inline-element path
// to mirror upstream Okapi's TagType.CLOSING code emission.
func (b *runBuilder) AddPcCloseData(id, semType, data string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:   id,
		Type: semType,
		Data: data,
	}})
}

func (b *runBuilder) Runs() []model.Run {
	return b.runs
}

// PlainText returns the concatenated TextRun content (mirrors trim
// semantics of the previous textBuf-based check).
func (b *runBuilder) PlainText() string {
	var n int
	for _, r := range b.runs {
		if r.Text != nil {
			n += len(r.Text.Text)
		}
	}
	if n == 0 {
		return ""
	}
	buf := make([]byte, 0, n)
	for _, r := range b.runs {
		if r.Text != nil {
			buf = append(buf, r.Text.Text...)
		}
	}
	return string(buf)
}

// hasInlineCodeRuns reports whether the run sequence contains any
// non-text run (Ph / PcOpen / PcClose / Sub / Plural / Select).
func hasInlineCodeRuns(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}
