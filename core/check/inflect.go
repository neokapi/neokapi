package check

import (
	"strings"
	"unicode/utf8"
)

// An English source term is found with its regular inflections unless it
// declares forms of its own. A rule about "alert" is a rule about "Two alerts",
// and a check that read only the bare spelling would never ask for the rule's
// rendering there.
//
// The default stops at inflection. It accepts the endings -s, -es, -ed and -ing
// appended to the term, and -d after a final e. A stemmer or a substring match
// reaches derivations, which are other words with other terms: "translation" is
// not a use of "translate", "extraction" is not a use of "extract", and
// "settings" is not a use of "set". Irregular forms, the -ies plural and an e
// dropped before -ing are out of reach too, and a term that needs them declares
// them as forms.
//
// A term joined to another word by a hyphen is part of a compound, so
// "pseudo-translate" is not a use of "translate".

// englishInflections are the endings the English default appends to a term.
var englishInflections = []string{"s", "es", "ed", "ing"}

// EnglishInflections is the term followed by each inflected spelling the
// English default accepts for it. A blank term has none.
func EnglishInflections(term string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	out := make([]string, 0, len(englishInflections)+2)
	out = append(out, term)
	for _, suffix := range englishInflections {
		out = append(out, term+suffix)
	}
	if last, _ := utf8.DecodeLastRuneInString(term); last == 'e' || last == 'E' {
		out = append(out, term+"d")
	}
	return out
}

// FindEnglishInflectionsIn returns the byte ranges where term or one of its
// regular English inflections occurs in p as a word of its own, outside any
// hyphenated compound. cased matches in the term's own casing.
func FindEnglishInflectionsIn(p *PreparedText, term string, cased bool) [][2]int {
	if p == nil {
		return nil
	}
	hits := FindTermFormsIn(p, EnglishInflections(term), cased)
	kept := hits[:0]
	for _, h := range hits {
		if !inHyphenCompound(p.text, h[0], h[1]) {
			kept = append(kept, h)
		}
	}
	return kept
}

// inHyphenCompound reports whether the span [start,end) of text is joined to a
// word on either side by a hyphen.
func inHyphenCompound(text string, start, end int) bool {
	if start > 0 && text[start-1] == '-' {
		if before, _ := utf8.DecodeLastRuneInString(text[:start-1]); wordRune(before) {
			return true
		}
	}
	if end < len(text) && text[end] == '-' {
		if after, _ := utf8.DecodeRuneInString(text[end+1:]); wordRune(after) {
			return true
		}
	}
	return false
}
