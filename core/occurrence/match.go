package occurrence

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/check"
)

// span is a byte range in the text searched.
type span struct{ start, end int }

// matcher finds one term in a text, delegating to the shared check.TermMatcher
// so the occurrence graph counts a term exactly where the gates report it.
type matcher struct{ tm *check.TermMatcher }

func newMatcher(term string) *matcher {
	return &matcher{tm: check.NewTermMatcher(term)}
}

// find returns every non-overlapping match of the term in text, left to right.
// Offsets are byte offsets into text as given.
func (m *matcher) find(text string) []span {
	hits := m.tm.Find(text)
	if len(hits) == 0 {
		return nil
	}
	out := make([]span, 0, len(hits))
	for _, h := range hits {
		out = append(out, span{start: h[0], end: h[1]})
	}
	return out
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

// Snippet renders the first use of term in text, in context: elided at both
// ends and collapsed to one line, exactly as an Occurrence carries it. It is
// the presentation half of a use whose fact, that the block uses the term, is
// recorded on a context-graph edge, so a surface that reads uses from the graph
// can still show the passage without re-running the join. Empty when the text
// holds no use of the term.
func Snippet(text, term string) string {
	if strings.TrimSpace(term) == "" {
		return ""
	}
	spans := newMatcher(term).find(text)
	if len(spans) == 0 {
		return ""
	}
	return snippet(text, spans[0].start, spans[0].end)
}
