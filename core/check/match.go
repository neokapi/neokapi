package check

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// TermMatcher finds one term in a text. It is the single definition of what it
// means for a text to *use* a term, shared by the voice-profile vocabulary
// rules, the terms-store lookup, the do-not-translate check and the occurrence
// graph — so the same word in the same sentence is a hit for all of them or for
// none. A matcher is built once per term and reused across texts, because the
// expensive part (deciding what the term's words are and whether its edges take
// a boundary rule) depends only on the term.
//
// The rule, precisely:
//
//   - Matching is case-insensitive unless the matcher was built case-sensitive.
//   - A multi-word term matches across any run of whitespace, so "content
//     memory" is found in "content  memory" and across a line break.
//   - A match must sit on a word boundary: "use" is not found inside "user",
//     and "mooring" is not found inside "mooring_id" — an underscore continues
//     a word, so an identifier is one token. The rule is applied per edge and
//     only where the term's own edge character is a word character, so "C++"
//     and "{count}" still match against adjacent text.
//   - Scripts written without word separators (Han, Kana, Thai, Lao, Khmer,
//     Myanmar, Tibetan) take no boundary rule: a term in one of them is
//     surrounded by its own script wherever it appears.
//   - Matches of one term never overlap: the scan is left to right and resumes
//     after each match.
type TermMatcher struct {
	// words are the term's whitespace-separated parts, folded to the case the
	// scan runs in. A term is found where its words appear in order, separated
	// by any whitespace.
	words []string
	// boundaryStart and boundaryEnd record whether a word boundary is required
	// at each edge. It is not, where the term's edge character is not itself a
	// word character.
	boundaryStart bool
	boundaryEnd   bool
	caseSensitive bool
}

// NewTermMatcher builds a case-insensitive matcher for term.
func NewTermMatcher(term string) *TermMatcher {
	return newTermMatcher(strings.ToLower(term), false)
}

// NewCaseSensitiveTermMatcher builds a matcher that requires the term's own
// casing — for a vocabulary that distinguishes "Compass" the product from
// "compass" the instrument.
func NewCaseSensitiveTermMatcher(term string) *TermMatcher {
	return newTermMatcher(term, true)
}

func newTermMatcher(term string, caseSensitive bool) *TermMatcher {
	m := &TermMatcher{words: strings.Fields(term), caseSensitive: caseSensitive}
	if len(m.words) == 0 {
		return m
	}
	first, _ := utf8.DecodeRuneInString(m.words[0])
	last, _ := utf8.DecodeLastRuneInString(m.words[len(m.words)-1])
	m.boundaryStart = wordRune(first)
	m.boundaryEnd = wordRune(last)
	return m
}

// Empty reports whether the matcher was built from a term with no words, and
// therefore matches nothing.
func (m *TermMatcher) Empty() bool { return len(m.words) == 0 }

// Find returns the byte ranges [start,end) where the term occurs in text.
// Reported ranges index the bytes of text as given.
func (m *TermMatcher) Find(text string) [][2]int {
	return m.FindIn(PrepareText(text))
}

// Matches reports whether the term occurs in text at all.
func (m *TermMatcher) Matches(text string) bool { return len(m.Find(text)) > 0 }

// FindIn is Find against a text prepared once. A caller scanning one text for
// many terms — the terms-store lookup does exactly that, for every term
// declared in the language — prepares the text once and pays the case fold
// once rather than once per term.
func (m *TermMatcher) FindIn(p *PreparedText) [][2]int {
	if p == nil || len(m.words) == 0 {
		return nil
	}
	hay, offset := p.folded, p.offset
	if m.caseSensitive {
		hay, offset = p.text, nil
	}
	if hay == "" {
		return nil
	}

	var out [][2]int
	from := 0
	for from <= len(hay) {
		s, e, ok := m.next(hay, from)
		if !ok {
			break
		}
		out = append(out, [2]int{mapOffset(offset, s), mapOffset(offset, e)})
		if e == s {
			from = s + 1
		} else {
			from = e
		}
	}
	return out
}

// next finds the first match at or after from.
func (m *TermMatcher) next(hay string, from int) (int, int, bool) {
	for from <= len(hay) {
		idx := strings.Index(hay[from:], m.words[0])
		if idx < 0 {
			return 0, 0, false
		}
		start := from + idx
		end, ok := m.matchTail(hay, start+len(m.words[0]))
		if ok && m.boundariesHold(hay, start, end) {
			return start, end, true
		}
		from = start + 1
	}
	return 0, 0, false
}

// matchTail walks the term's remaining words, each separated from the last by
// one or more whitespace characters.
func (m *TermMatcher) matchTail(hay string, pos int) (int, bool) {
	for _, w := range m.words[1:] {
		gap := 0
		for pos+gap < len(hay) {
			r, size := utf8.DecodeRuneInString(hay[pos+gap:])
			if !unicode.IsSpace(r) {
				break
			}
			gap += size
		}
		if gap == 0 {
			return 0, false // words must be separated by whitespace
		}
		pos += gap
		if !strings.HasPrefix(hay[pos:], w) {
			return 0, false
		}
		pos += len(w)
	}
	return pos, true
}

// boundariesHold applies the word-boundary rule at each edge of a candidate
// match, judging each edge separately.
func (m *TermMatcher) boundariesHold(hay string, start, end int) bool {
	if m.boundaryStart && start > 0 {
		before, _ := utf8.DecodeLastRuneInString(hay[:start])
		if wordRune(before) {
			return false
		}
	}
	if m.boundaryEnd && end < len(hay) {
		after, _ := utf8.DecodeRuneInString(hay[end:])
		if wordRune(after) {
			return false
		}
	}
	return true
}

// PreparedText is a text folded once so many matchers can scan it. Matched
// ranges always index the original bytes.
type PreparedText struct {
	// text is the text as given.
	text string
	// folded is the case-folded copy the case-insensitive scan runs over.
	folded string
	// offset maps a byte offset in folded to the corresponding offset in text.
	// It is nil in the common case, where folding leaves every rune the same
	// width and the two coincide. A handful of runes lowercase to a different
	// number of bytes — a dotted capital I is the usual example — and there an
	// unmapped offset would address the wrong characters.
	offset []int
}

// PrepareText folds text once for matching.
func PrepareText(text string) *PreparedText {
	folded := strings.ToLower(text)
	if len(folded) == len(text) {
		return &PreparedText{text: text, folded: folded}
	}

	var b strings.Builder
	b.Grow(len(folded))
	offset := make([]int, 0, len(folded)+1)
	for i, r := range text {
		before := b.Len()
		b.WriteRune(unicode.ToLower(r))
		for k := before; k < b.Len(); k++ {
			offset = append(offset, i)
		}
	}
	offset = append(offset, len(text))
	return &PreparedText{text: text, folded: b.String(), offset: offset}
}

// Text returns the text as given.
func (p *PreparedText) Text() string { return p.text }

func mapOffset(offset []int, i int) int {
	if offset == nil {
		return i
	}
	if i >= len(offset) {
		return offset[len(offset)-1]
	}
	return offset[i]
}

// separatorless are the scripts that do not put spaces between words. A term in
// one of them is surrounded by its own script wherever it appears, so requiring
// a boundary would reject every real occurrence.
var separatorless = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana,
	unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar, unicode.Tibetan,
}

// wordRune reports whether r is the kind of character that continues a word,
// and therefore blocks a boundary. An underscore does: `mooring_id` is one
// token, not a use of `mooring`. Characters from separatorless scripts never
// do — there is no boundary for them to block.
func wordRune(r rune) bool {
	if unicode.In(r, separatorless...) {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// FindTerm returns the byte ranges [start,end) where term occurs in text, under
// the shared TermMatcher rule. An empty term returns nil.
func FindTerm(text, term string) [][2]int {
	return NewTermMatcher(term).Find(text)
}

// FindTermCased is FindTerm, requiring the term's own casing.
//
// For a rule whose whole content is capitalisation — a product name written one
// way and one way only — where folding case makes the rule fire on every
// correct use.
func FindTermCased(text, term string) [][2]int {
	return NewCaseSensitiveTermMatcher(term).Find(text)
}

// ContainsTerm reports whether term occurs in text (see FindTerm).
func ContainsTerm(text, term string) bool {
	return len(FindTerm(text, term)) > 0
}
