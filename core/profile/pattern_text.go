package profile

import (
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// PatternText is the text a pattern rule reads, with where code sits in it.
//
// Pattern rules read the same projection whether the text arrived as a plain
// string or as a block's runs, so a rule scoped to prose reaches the same words
// in both. In a plain string an inline code span is spelled with backticks; in a
// block a reader holds it as a [model.TypeInlineCode] pair. The projection of a
// block writes that pair back as a backtick span around the code's text and
// records it as code, so a match cannot start in the prose beside a code span
// and read on into it, as it cannot in the plain string.
//
// Offsets into Text map back to [model.RunsText] coordinates, the space every
// run anchor is in, through RealOffset.
type PatternText struct {
	text string
	// code holds the byte ranges of the code spans the runs declare. Spans
	// spelled with backticks inside a text run are found from the text itself
	// (check.CodeSpans) when a scoped rule needs them.
	code []span
	// real maps each byte offset of text, and its end, to the matching offset
	// in RunsText. Nil for a plain string, where the two are the same.
	real []int
}

// PlainPatternText is the projection of a plain string: the string itself.
func PlainPatternText(text string) PatternText {
	return PatternText{text: text}
}

// RunsPatternText is the projection of a block's runs that pattern rules read.
//
// It is built on the hygiene view (model.NewHygieneView), which collapses each
// inline code to one sentinel and each [model.TypeInlineCode] span, with its
// contents, to one sentinel. Every other run kind is settled there. This
// projection then writes the sentinels back:
//
//   - a collapsed inline code span becomes a backtick, the span's text, and a
//     backtick, and its range is recorded as code;
//   - every other inline code (a placeholder, a link or emphasis marker, a sub
//     run) contributes no text, as in RunsText;
//   - a literal U+FFFC that stands for no code is text.
func RunsPatternText(runs []model.Run) PatternText {
	view := model.NewHygieneView(runs)
	hygiene := view.Text()
	realText := model.RunsText(runs)

	var buf strings.Builder
	var code []span
	real := make([]int, 0, len(hygiene)+1)
	// Between two sentinels the hygiene text and RunsText advance together.
	copyText := func(from, to int) {
		base := view.RealOffset(from)
		for i := from; i < to; i++ {
			real = append(real, base+i-from)
		}
		buf.WriteString(hygiene[from:to])
	}

	start := 0
	for i := 0; i < len(hygiene); {
		r, size := utf8.DecodeRuneInString(hygiene[i:])
		if r != model.ObjectReplacement {
			i += size
			continue
		}
		run, ok := view.Code(i)
		if !ok {
			i += size
			continue
		}
		copyText(start, i)
		if run.PcOpen != nil && run.PcOpen.Type == model.TypeInlineCode {
			innerStart, innerEnd := view.RealOffset(i), view.RealOffset(i+size)
			spanStart := buf.Len()
			real = append(real, innerStart)
			buf.WriteByte('`')
			for j := innerStart; j < innerEnd; j++ {
				real = append(real, j)
			}
			buf.WriteString(realText[innerStart:innerEnd])
			real = append(real, innerEnd)
			buf.WriteByte('`')
			code = append(code, span{spanStart, buf.Len()})
		}
		i += size
		start = i
	}
	copyText(start, len(hygiene))
	real = append(real, view.RealOffset(len(hygiene)))

	return PatternText{text: buf.String(), code: code, real: real}
}

// Text returns the projected text the pattern rules are matched against.
func (t PatternText) Text() string { return t.text }

// RealOffset maps a byte offset into Text to the matching byte offset into
// [model.RunsText] of the runs the projection was built from. A backtick written
// for a code span maps to the edge of the span's text. For a plain string the
// offset is unchanged.
func (t PatternText) RealOffset(off int) int {
	if t.real == nil {
		return off
	}
	if off < 0 {
		return 0
	}
	if off >= len(t.real) {
		return t.real[len(t.real)-1]
	}
	return t.real[off]
}

// codeSpans returns where code sits in the projection: the spans the runs
// declare and the backtick spans and fences spelled in the text.
func (t PatternText) codeSpans() []span {
	return append(append([]span(nil), t.code...), codeSpans(t.text)...)
}

// codeSpans finds fenced blocks and inline code spans (check.CodeSpans).
func codeSpans(text string) []span {
	found := check.CodeSpans(text)
	out := make([]span, 0, len(found))
	for _, sp := range found {
		out = append(out, span{sp[0], sp[1]})
	}
	return out
}
