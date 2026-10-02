package format

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Some formats spell markup in the same characters as text: Markdown, MDX and
// AsciiDoc readers turn the constructs they model into inline codes and leave
// the rest of the document's syntax in text runs, so a text run there holds
// the document's own markup (a backslash escape, a bracket that opens no
// link, an MDX expression). An edit's wording is text. Once an edit has
// rewritten a block, the writer cannot tell the two apart by the characters
// alone, so it compares the edited block with the block as it was read
// (model.Block.SourceAsRead): markup the block already held, spelled the
// same, is the document's own; anything else the edit spells is escaped so
// it reads back as text.

// AddedSyntax describes the markup a format's text can spell, for
// EscapeAddedSyntax.
type AddedSyntax struct {
	// Find reports whether s[i] begins markup, or triggers it (the '[' after
	// an AsciiDoc macro name), and the extent [start, end) the markup is
	// compared by, which may begin before i.
	Find func(s string, i int) (start, end int, ok bool)
	// Escape returns the bytes that replace s[i:i+n] so that the markup Find
	// reported at i reads as text.
	Escape func(s string, i int) (n int, repl string)
	// InCode reports whether a text run that follows the given inline code
	// is literal until the code that closes it, as text inside a Markdown
	// code span is. Such text is never markup and never escaped. It may be
	// nil.
	InCode func(r model.Run) (opens, closes bool)
}

// RenderEditedRuns renders an edited block's runs as model.RenderRunsWithData
// does, escaping the markup the edit added. read is the block's source as it
// was read (model.Block.SourceAsRead): markup Find reports in a text run of
// the edited runs is written as it is when the same markup, byte for byte,
// begins in a text run of read, and escaped otherwise. Each piece of markup in
// read can account for one in the edit, so an edit can keep, move or drop the
// block's own markup and cannot add any.
func RenderEditedRuns(runs, read []model.Run, syn AddedSyntax) string {
	out, spans := renderTextSpans(runs, syn)
	readOut, readSpans := renderTextSpans(read, syn)

	// The markup read holds, counted by spelling. Markup inside other markup
	// is part of it and is not counted on its own, on either side.
	kept := map[string]int{}
	readSkip := 0
	for _, sp := range readSpans {
		for i := max(sp[0], readSkip); i < sp[1]; i++ {
			if start, end, ok := syn.Find(readOut, i); ok {
				kept[readOut[start:end]]++
				readSkip = max(end, i+1)
				i = readSkip - 1
			}
		}
	}

	var b strings.Builder
	b.Grow(len(out) + 16)
	prev, skip := 0, 0
	for _, sp := range spans {
		for i := max(sp[0], skip); i < sp[1]; i++ {
			if i < skip {
				continue
			}
			// Markup the edit adds is escaped where it opens, and what it
			// encloses is scanned again: escaping an expression's '{' leaves
			// its '}' to be judged on its own.
			start, end, ok := syn.Find(out, i)
			if !ok {
				continue
			}
			if c := out[start:end]; kept[c] > 0 {
				kept[c]--
				skip = end
				continue
			}
			n, repl := syn.Escape(out, i)
			b.WriteString(out[prev:i])
			b.WriteString(repl)
			prev = i + n
			skip = i + max(n, 1)
		}
	}
	b.WriteString(out[prev:])
	return b.String()
}

// renderTextSpans renders runs as model.RenderRunsWithData does and returns
// the byte spans of the output that text runs wrote, leaving out text that
// syn.InCode says is literal.
func renderTextSpans(runs []model.Run, syn AddedSyntax) (string, [][2]int) {
	var b strings.Builder
	var spans [][2]int
	depth := 0
	model.RenderRunsWith(&b, runs, &model.RunRenderer{
		Text: func(b *strings.Builder, text string) {
			if depth == 0 && text != "" {
				spans = append(spans, [2]int{b.Len(), b.Len() + len(text)})
			}
			b.WriteString(text)
		},
		Code: func(r model.Run) {
			if syn.InCode == nil {
				return
			}
			opens, closes := syn.InCode(r)
			switch {
			case opens:
				depth++
			case closes && depth > 0:
				depth--
			}
		},
	})
	return b.String(), spans
}
