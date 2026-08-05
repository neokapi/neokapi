package occurrence

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// span is a byte range in the text searched.
type span struct{ start, end int }

// matcher finds one term in a text. It is built once per term and reused across
// every candidate block, since the expensive part — deciding what the term's
// words are and whether its edges take a boundary rule — depends only on the
// term.
type matcher struct {
	// words are the term's whitespace-separated parts, case-folded. A term is
	// found where its words appear in order, separated by any whitespace.
	words []string
	// boundaryStart and boundaryEnd record whether a word boundary is required
	// at each edge. It is not, where the term's edge character belongs to a
	// script written without word separators.
	boundaryStart bool
	boundaryEnd   bool
}

func newMatcher(term string) *matcher {
	m := &matcher{words: strings.Fields(strings.ToLower(strings.TrimSpace(term)))}
	if len(m.words) == 0 {
		return m
	}
	first, _ := utf8.DecodeRuneInString(m.words[0])
	last, _ := utf8.DecodeLastRuneInString(m.words[len(m.words)-1])
	m.boundaryStart = wordRune(first)
	m.boundaryEnd = wordRune(last)
	return m
}

// find returns every non-overlapping match of the term in text, left to right.
// Offsets are byte offsets into text as given.
func (m *matcher) find(text string) []span {
	if len(m.words) == 0 || text == "" {
		return nil
	}
	// Fold the haystack once, and only where folding preserves byte lengths.
	// A handful of runes lowercase to a different number of bytes — a dotted
	// capital I is the usual example — and there the offsets of the folded
	// copy no longer address the original. Matching case-sensitively is a
	// small loss on rare text; offsets that point at the wrong characters
	// would be a silent one.
	folded := strings.ToLower(text)
	if len(folded) != len(text) {
		folded = text
	}

	var out []span
	from := 0
	for from <= len(folded) {
		s, e, ok := m.next(folded, from)
		if !ok {
			break
		}
		out = append(out, span{start: s, end: e})
		if e == s {
			from = s + 1
		} else {
			from = e
		}
	}
	return out
}

// next finds the first match at or after `from`.
func (m *matcher) next(text string, from int) (int, int, bool) {
	for from <= len(text) {
		idx := strings.Index(text[from:], m.words[0])
		if idx < 0 {
			return 0, 0, false
		}
		start := from + idx
		end, ok := m.matchTail(text, start+len(m.words[0]))
		if ok && m.boundariesHold(text, start, end) {
			return start, end, true
		}
		from = start + 1
	}
	return 0, 0, false
}

// matchTail walks the term's remaining words, each separated from the last by
// one or more whitespace characters.
func (m *matcher) matchTail(text string, pos int) (int, bool) {
	for _, w := range m.words[1:] {
		gap := 0
		for pos+gap < len(text) {
			r, size := utf8.DecodeRuneInString(text[pos+gap:])
			if !unicode.IsSpace(r) {
				break
			}
			gap += size
		}
		if gap == 0 {
			return 0, false // words must be separated by whitespace
		}
		pos += gap
		if !strings.HasPrefix(text[pos:], w) {
			return 0, false
		}
		pos += len(w)
	}
	return pos, true
}

// boundariesHold applies the word-boundary rule at each edge of a candidate
// match. See Find's documentation for what the rule is and why each edge is
// judged separately.
func (m *matcher) boundariesHold(text string, start, end int) bool {
	if m.boundaryStart && start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if wordRune(before) {
			return false
		}
	}
	if m.boundaryEnd && end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if wordRune(after) {
			return false
		}
	}
	return true
}

// separatorless are the scripts that do not put spaces between words. A term in
// one of them is surrounded by its own script wherever it appears, so requiring
// a boundary would reject every real occurrence.
var separatorless = []*unicode.RangeTable{
	unicode.Han, unicode.Hiragana, unicode.Katakana,
	unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar, unicode.Tibetan,
}

// wordRune reports whether r is the kind of character that continues a word,
// and therefore blocks a boundary. Characters from separatorless scripts never
// do: there is no boundary for them to block.
func wordRune(r rune) bool {
	if unicode.In(r, separatorless...) {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// snippetRunes is how much context a snippet carries on each side of a match.
const snippetRunes = 48

// ellipsis marks a snippet that was cut. A single character rather than three
// dots, so a terminal column count stays predictable.
const ellipsis = "…"

// snippet returns the match in context, cut at rune boundaries and elided where
// it was cut. Whitespace is collapsed: a match spanning a line break reads as
// one line, which is what a list of occurrences needs it to be.
func snippet(text string, start, end int) string {
	if start < 0 || end > len(text) || start > end {
		return ""
	}
	left, cutLeft := backUp(text[:start], snippetRunes)
	right, cutRight := runForward(text[end:], snippetRunes)

	// Collapse the window as one string: doing it piecewise would drop the
	// space between the context and the match itself.
	body := collapse(left + text[start:end] + right)
	var b strings.Builder
	if cutLeft {
		b.WriteString(ellipsis)
	}
	b.WriteString(body)
	if cutRight {
		b.WriteString(ellipsis)
	}
	return b.String()
}

func backUp(s string, runes int) (string, bool) {
	count := 0
	for i := len(s); i > 0; {
		_, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
		count++
		if count == runes {
			return s[i:], i > 0
		}
	}
	return s, false
}

func runForward(s string, runes int) (string, bool) {
	count, i := 0, 0
	for i < len(s) {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		count++
		if count == runes {
			return s[:i], i < len(s)
		}
	}
	return s, false
}

// collapse turns every run of whitespace into a single space.
func collapse(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}
