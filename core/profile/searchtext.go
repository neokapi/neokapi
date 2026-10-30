package profile

import (
	"html"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A block's text (model.RunsText) gives every inline code zero width, which is
// what a reader sees of `<b>`. The HTML reader also keeps a character
// reference (`&amp;`, `&nbsp;`) as an inline code, and the page shows it as a
// character. In the zero-width text `utilize&amp;more` reads as "utilizemore".
//
// TermSearchText is the text a term is looked for in: each character reference
// stands for the character it spells, and every other code keeps its zero
// width. A hit is mapped back to offsets of the block's text, so its anchor
// and fix span come from the block's own runs.

// entityCodeType is the vocabulary type of an inline code that is a character
// reference.
const entityCodeType = "code:entity"

// TermSearchText returns the text a term is looked for in, for text that the
// runs flatten to, and a function from an offset in that search text to the
// offset in text it stands for. When the runs hold no character reference, or
// do not flatten to text, it returns text itself and a nil function.
func TermSearchText(text string, runs []model.Run) (string, func(int) int) {
	if !hasEntityCode(runs) {
		return text, nil
	}
	var b strings.Builder
	b.Grow(len(text) + 8)
	var back []int
	writeSearchText(&b, &back, runs)
	if model.RunsText(runs) != text {
		return text, nil
	}
	back = append(back, len(text))
	return b.String(), func(i int) int {
		if i < 0 {
			return 0
		}
		if i >= len(back) {
			return back[len(back)-1]
		}
		return back[i]
	}
}

// writeSearchText writes the search text of runs to b and, for each byte
// written, the offset in the block's text it stands for to back. A plural or a
// select is read in its "other" branch, as model.RunsText reads it.
func writeSearchText(b *strings.Builder, back *[]int, runs []model.Run) {
	textAt := 0
	var walk func(runs []model.Run)
	walk = func(runs []model.Run) {
		for _, r := range runs {
			switch r.Kind() {
			case model.RunKindText:
				for i := range len(r.Text.Text) {
					*back = append(*back, textAt+i)
				}
				b.WriteString(r.Text.Text)
				textAt += len(r.Text.Text)
			case model.RunKindPh:
				if r.Ph.Type != entityCodeType {
					continue
				}
				spelled := html.UnescapeString(r.Ph.Data)
				if spelled == r.Ph.Data {
					// A reference the decoder does not know still ends a
					// word on the page.
					spelled = " "
				}
				for range len(spelled) {
					*back = append(*back, textAt)
				}
				b.WriteString(spelled)
			case model.RunKindPlural:
				if form, ok := r.Plural.Forms[model.PluralOther]; ok {
					walk(form)
				}
			case model.RunKindSelect:
				if form, ok := r.Select.Cases["other"]; ok {
					walk(form)
				}
			}
		}
	}
	walk(runs)
}

// hasEntityCode reports whether runs hold a character reference.
func hasEntityCode(runs []model.Run) bool {
	for _, r := range runs {
		switch r.Kind() {
		case model.RunKindPh:
			if r.Ph.Type == entityCodeType {
				return true
			}
		case model.RunKindPlural:
			for _, form := range r.Plural.Forms {
				if hasEntityCode(form) {
					return true
				}
			}
		case model.RunKindSelect:
			for _, form := range r.Select.Cases {
				if hasEntityCode(form) {
					return true
				}
			}
		}
	}
	return false
}

// MatchTermRulesInRuns is MatchTermRules over a block's text and the runs it
// flattens from: a term is looked for in TermSearchText, so a character
// reference beside a word ends it as the page shows, and each hit's offsets
// index text. A hit that lies inside one character reference has no text of
// its own to report or replace and is left out.
func MatchTermRulesInRuns(sets []TermRuleSet, text string, runs []model.Run) []VocabHit {
	search, toText := TermSearchText(text, runs)
	hits := MatchTermRules(sets, search)
	if toText == nil {
		return hits
	}
	out := hits[:0]
	for _, h := range hits {
		h.Start, h.End = toText(h.Start), toText(h.End)
		if h.End > h.Start {
			out = append(out, h)
		}
	}
	return out
}
