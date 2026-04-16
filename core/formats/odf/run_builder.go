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

func (b *runBuilder) AppendPcOpen(id, semType, data string) {
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID:   id,
		Type: semType,
		Data: data,
	}})
}

func (b *runBuilder) AppendPcClose(id, semType string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:   id,
		Type: semType,
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
