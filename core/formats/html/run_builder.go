package html

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// runBuilder accumulates a []model.Run while walking the HTML AST or
// token stream. It coalesces adjacent TextRuns so consecutive text
// chunks produce a single text run.
type runBuilder struct {
	runs []model.Run
	// dropNextLeadingSpace, when true, causes the very next AddText call
	// to strip any leading HTML whitespace from its argument (and clears
	// the flag). Set by token-stream handlers that consume an unknown
	// orphan close tag (e.g. </pub> in simple_subscript.html) so the
	// trailing space okapi's HtmlFilter swallows around such placeholders
	// is dropped on the native side too. Internal whitespace is untouched.
	dropNextLeadingSpace bool
}

// newRunBuilder returns a zero-valued runBuilder ready for appends.
func newRunBuilder() *runBuilder {
	return &runBuilder{}
}

// MarkDropNextLeadingSpace flags the builder so the next AddText / addTextWithEntities
// call peels any leading HTML whitespace from its input. The flag clears as
// soon as it is consumed, so it never crosses more than one text token.
func (b *runBuilder) MarkDropNextLeadingSpace() {
	b.dropNextLeadingSpace = true
}

// AppendText adds plain text. If the previous run is a TextRun, the new
// text is appended to it rather than emitting a second adjacent TextRun.
func (b *runBuilder) AddText(text string) {
	if b.dropNextLeadingSpace {
		b.dropNextLeadingSpace = false
		text = trimLeadingHTMLWhitespace(text)
	}
	if text == "" {
		return
	}
	if n := len(b.runs); n > 0 && b.runs[n-1].Text != nil {
		b.runs[n-1].Text.Text += text
		return
	}
	b.runs = append(b.runs, model.Run{Text: &model.TextRun{Text: text}})
}

// AppendPh emits a PlaceholderRun. The display/equiv/constraints arguments
// match the HTML reader's vocabulary-driven span metadata; pass zero-values
// for spans that were never looked up in the vocabulary (e.g. comments, raw
// markup, non-translatable placeholders).
func (b *runBuilder) AddPh(id, semType, subType, data, disp, equiv string, constraints model.RunConstraints) {
	b.runs = append(b.runs, model.Run{Ph: &model.PlaceholderRun{
		ID:          id,
		Type:        semType,
		SubType:     subType,
		Data:        data,
		Disp:        disp,
		Equiv:       equiv,
		Constraints: &constraints,
	}})
}

// AppendPcOpen emits the opening half of a paired code.
func (b *runBuilder) AddPcOpen(id, semType, subType, data, disp, equiv string, constraints model.RunConstraints) {
	b.runs = append(b.runs, model.Run{PcOpen: &model.PcOpenRun{
		ID:          id,
		Type:        semType,
		SubType:     subType,
		Data:        data,
		Disp:        disp,
		Equiv:       equiv,
		Constraints: &constraints,
	}})
}

// AppendPcClose emits the closing half of a paired code. PcCloseRun carries no
// Constraints or Disp — the closing half inherits behavior and display from its
// opener (matched by shared id).
func (b *runBuilder) AddPcClose(id, semType, subType, data, equiv string) {
	b.runs = append(b.runs, model.Run{PcClose: &model.PcCloseRun{
		ID:      id,
		Type:    semType,
		SubType: subType,
		Data:    data,
		Equiv:   equiv,
	}})
}

// Runs returns the accumulated run slice. Returns a non-nil empty slice if
// nothing was appended, so callers can distinguish "empty content" from
// "no builder at all" by nil-checking the builder itself.
func (b *runBuilder) Runs() []model.Run {
	if b.runs == nil {
		return []model.Run{}
	}
	return b.runs
}

// IsEmpty reports whether no runs have been appended.
func (b *runBuilder) IsEmpty() bool {
	return len(b.runs) == 0
}

// HasNonText reports whether any non-text run (Ph, PcOpen, PcClose)
// has been appended.
func (b *runBuilder) HasNonText() bool {
	for _, r := range b.runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// PlainText returns the concatenated TextRun content, dropping any
// non-text runs.
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

// collapseWhitespaceRuns collapses runs of HTML whitespace inside TextRuns to
// a single space, with non-text runs acting as transparent boundaries — i.e.
// consecutive whitespace split by a span marker collapses across the marker,
// matching the behavior of collapseWhitespaceCoded on a PUA-coded string.
//
// The input slice is not modified; a new slice is returned.
func collapseWhitespaceRuns(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	out := make([]model.Run, 0, len(runs))
	inSpace := false
	for _, r := range runs {
		if r.Text == nil {
			// Non-text runs pass through unchanged and do NOT reset the
			// inSpace flag — a marker between two whitespace regions
			// behaves as if the whitespace were contiguous.
			out = append(out, r)
			continue
		}
		var buf strings.Builder
		buf.Grow(len(r.Text.Text))
		for _, ch := range r.Text.Text {
			if isHTMLWhitespace(ch) {
				if !inSpace {
					buf.WriteByte(' ')
					inSpace = true
				}
			} else {
				buf.WriteRune(ch)
				inSpace = false
			}
		}
		if buf.Len() > 0 {
			out = append(out, model.Run{Text: &model.TextRun{Text: buf.String()}})
		}
	}
	return out
}

// trimWhitespaceRuns trims leading and trailing HTML whitespace from the Run
// sequence, stopping at the first non-text run (matches trimCoded, which
// stops at a PUA marker).
//
// The input slice is not modified; a new slice is returned when trimming
// changes anything. Empty TextRuns produced by trimming are dropped.
func trimWhitespaceRuns(runs []model.Run) []model.Run {
	if len(runs) == 0 {
		return runs
	}
	// Leading trim: walk forward while we see TextRuns, peeling leading
	// whitespace from each. Stop at the first non-text run OR the first
	// text run with non-whitespace content.
	start := 0
	var leadingText string
	for ; start < len(runs); start++ {
		r := runs[start]
		if r.Text == nil {
			break
		}
		trimmed := trimLeadingHTMLWhitespace(r.Text.Text)
		if trimmed != "" {
			leadingText = trimmed
			break
		}
		// Entire TextRun was whitespace — drop it and continue peeling.
	}
	// Trailing trim: symmetric walk from the end.
	end := len(runs)
	var trailingText string
	for ; end > start; end-- {
		r := runs[end-1]
		if r.Text == nil {
			break
		}
		// At the boundary run between start and end, we've already
		// trimmed leading whitespace; don't double-trim.
		src := r.Text.Text
		if end-1 == start {
			src = leadingText
		}
		trimmed := trimTrailingHTMLWhitespace(src)
		if trimmed != "" {
			trailingText = trimmed
			break
		}
	}
	if start >= end {
		return []model.Run{}
	}
	singleton := start == end-1
	out := make([]model.Run, 0, end-start)
	for i := start; i < end; i++ {
		r := runs[i]
		switch {
		case singleton && r.Text != nil:
			// Both trims collapsed onto the same TextRun; trailingText
			// already reflects the leading trim (see src = leadingText).
			out = append(out, model.Run{Text: &model.TextRun{Text: trailingText}})
		case i == start && r.Text != nil:
			out = append(out, model.Run{Text: &model.TextRun{Text: leadingText}})
		case i == end-1 && r.Text != nil:
			out = append(out, model.Run{Text: &model.TextRun{Text: trailingText}})
		default:
			out = append(out, r)
		}
	}
	return out
}

// trimLeadingHTMLWhitespace peels HTML whitespace (space, tab, CR, LF, FF)
// from the start of s. Preserves non-breaking space (\u00A0).
func trimLeadingHTMLWhitespace(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			i++
			continue
		}
		break
	}
	return s[i:]
}

// trimTrailingHTMLWhitespace peels HTML whitespace from the end of s.
func trimTrailingHTMLWhitespace(s string) string {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			i--
			continue
		}
		break
	}
	return s[:i]
}
