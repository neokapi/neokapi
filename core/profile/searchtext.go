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
// SearchText is the text a term is looked for in: each character reference
// stands for the character it spells, and every other code keeps its zero
// width. A hit is mapped back to offsets of the block's text, so its anchor
// and fix span come from the block's own runs.

// entityCodeType is the vocabulary type of an inline code that is a character
// reference.
const entityCodeType = "code:entity"

// SearchText is the text a term is looked for in, for a block's text and the
// runs it flattens from.
type SearchText struct {
	// Text is the text searched.
	Text string
	// back maps each byte offset of Text, its end included, to the offset in
	// the block's text it stands for; nil when Text is the block's text.
	back []int
	// ref marks the bytes of Text a character reference spells.
	ref []bool
}

// NewSearchText returns the search text for text, which runs flatten to. When
// the runs hold no character reference, or do not flatten to text, the search
// text is text itself.
func NewSearchText(text string, runs []model.Run) SearchText {
	if !hasEntityCode(runs) {
		return SearchText{Text: text}
	}
	s := SearchText{}
	var b strings.Builder
	b.Grow(len(text) + 8)
	s.write(&b, runs)
	if model.RunsText(runs) != text {
		return SearchText{Text: text}
	}
	s.back = append(s.back, len(text))
	s.Text = b.String()
	return s
}

// ToText maps an offset of the search text to the offset in the block's text
// it stands for.
func (s SearchText) ToText(i int) int {
	if s.back == nil {
		return i
	}
	if i < 0 {
		return 0
	}
	if i >= len(s.back) {
		return s.back[len(s.back)-1]
	}
	return s.back[i]
}

// SpelledAtEdge reports whether a character reference spells the first or the
// last byte of the hit [start, end) of the search text. Such a hit has no span
// of the block's text that holds all of it: the block's text holds the words
// beside the reference, and the reference is a code.
func (s SearchText) SpelledAtEdge(start, end int) bool {
	if s.ref == nil || end <= start {
		return false
	}
	return s.ref[start] || s.ref[end-1]
}

// write writes the search text of runs to b, recording for each byte written
// the offset in the block's text it stands for and whether a reference spells
// it. A plural or a select is read in its "other" branch, as model.RunsText
// reads it.
func (s *SearchText) write(b *strings.Builder, runs []model.Run) {
	textAt := 0
	var walk func(runs []model.Run)
	walk = func(runs []model.Run) {
		for _, r := range runs {
			switch r.Kind() {
			case model.RunKindText:
				for i := range len(r.Text.Text) {
					s.back = append(s.back, textAt+i)
					s.ref = append(s.ref, false)
				}
				b.WriteString(r.Text.Text)
				textAt += len(r.Text.Text)
			case model.RunKindPh:
				if r.Ph.Type != entityCodeType {
					continue
				}
				spelled := spellReference(r.Ph.Data)
				for range len(spelled) {
					s.back = append(s.back, textAt)
					s.ref = append(s.ref, true)
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

// MatchTermRulesInRuns returns every hit under the rule sets in a block's text
// and the runs it flattens from. A term is looked for in the SearchText, so a
// character reference beside a word ends it as the page shows, and each hit's
// offsets index text. A hit whose first or last character a reference spells
// carries Spelled, the words as the page shows them.
//
// It and [MatchTermRulesInText] are the one definition of what it means for
// content to use a term rule. Every surface that matches term rules reaches
// one of them: `kapi check`, the overlays, the scoring and rewrite tools, the
// blast radius, so a word is a hit for every such gate or for none.
func MatchTermRulesInRuns(sets []TermRuleSet, text string, runs []model.Run) []VocabHit {
	return matchSearchText(sets, NewSearchText(text, runs))
}

// MatchTermRulesInText is [MatchTermRulesInRuns] for a caller that holds text
// and no runs: a text a person or an agent submits, a model's output. The text
// is read as `kapi check` reads a block of one text run, so `&amp;` written in
// it is five characters and no character reference spells a word.
func MatchTermRulesInText(sets []TermRuleSet, text string) []VocabHit {
	return matchSearchText(sets, SearchText{Text: text})
}

// matchSearchText matches the rule sets in the search text and maps each hit
// back to the text it was built from.
func matchSearchText(sets []TermRuleSet, search SearchText) []VocabHit {
	hits := matchTermRules(sets, search.Text)
	if search.back == nil {
		return hits
	}
	for i, h := range hits {
		if search.SpelledAtEdge(h.Start, h.End) {
			hits[i].Spelled = search.Text[h.Start:h.End]
		}
		hits[i].Start, hits[i].End = search.ToText(h.Start), search.ToText(h.End)
	}
	return hits
}

// spellReference returns the character a reference spells. A reference the
// decoder does not know still ends a word on the page, so it spells a space.
func spellReference(ref string) string {
	spelled := html.UnescapeString(ref)
	if spelled == ref {
		return " "
	}
	return spelled
}
