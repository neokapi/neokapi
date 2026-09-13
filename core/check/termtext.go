package check

import (
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/icu"
)

// PlaceholderToken matches the common interpolation/placeholder styles seen in
// multilingual content, longest/most-specific forms first so "{{x}}" wins over
// "{x}". Covered: {{name}}, ${name}, %(name)s (Python), %1$s (positional),
// %s/%d/%@ (printf/ObjC), {name}/{0} (ICU/.NET), <0>…</0> (numbered tags).
var PlaceholderToken = regexp.MustCompile(
	`\{\{[^{}]+\}\}|\$\{[^{}]+\}|%\([^)]+\)[a-zA-Z]|%\d+\$[a-zA-Z]|%[sdifeEgGxXobpqv@%]|\{[^{}]+\}|</?[0-9]+>`)

// NonBracePlaceholderToken is PlaceholderToken without the brace forms: the
// styles that can sit inside an ICU message as ordinary text, where the ICU
// parse steps over them.
var NonBracePlaceholderToken = regexp.MustCompile(
	`%\([^)]+\)[a-zA-Z]|%\d+\$[a-zA-Z]|%[sdifeEgGxXobpqv@%]|</?[0-9]+>`)

// termTextFill overwrites a placeholder in TermText. It is neither a word
// character nor whitespace, so a term cannot match inside a placeholder, run
// into one from the adjacent text, or bridge one as the gap in a term of
// several words.
const termTextFill = '\x1f'

// TermText is text as a term is matched against it: every placeholder
// overwritten byte for byte, and every word a reader sees left where it was, so
// an offset into the result is an offset into s.
//
// It is the one projection term matching reads through. term-check reads both
// sides through it; profile.MatchTermRules, the declared-term matcher behind
// the voice vocabulary gate and the source terminology gate, and the terms
// store lookup in terms.Locate read the content through it; and
// profile.ScopeTermRules reads through it to decide which rules a translate
// prompt carries. A placeholder's name is program syntax, so
// "{vessel} is alongside" does not use the term "vessel".
//
// Which spans are placeholders follows the split the placeholder check makes. A
// text that parses as ICU MessageFormat and chooses between sub-messages is
// read structurally (icu.SyntaxSpans): arguments, picker heads, branch keywords
// and the # a branch formats are overwritten, and the text of every branch is
// kept, so "{count, plural, one {# berth} other {# berths}}" uses "berth". The
// non-brace interpolation styles inside such a message are overwritten too. Any
// other text, including text that does not parse as ICU, is read for the
// placeholder styles of PlaceholderToken, and the rest of it is kept as it is.
//
// The input is plain text: for a block, the text of its runs, where placeholder
// runs are already absent. TermText reaches the placeholders a format reader
// left inside text runs.
func TermText(s string) string {
	if !strings.ContainsAny(s, "{$%<") {
		return s
	}
	var spans [][]int
	if tokens, err := icu.PlaceholderTokens(s); err == nil && tokens.Picker {
		for _, sp := range icu.SyntaxSpans(s) {
			spans = append(spans, []int{sp.Start, sp.End})
		}
		spans = append(spans, NonBracePlaceholderToken.FindAllStringIndex(s, -1)...)
	} else {
		spans = PlaceholderToken.FindAllStringIndex(s, -1)
	}
	if len(spans) == 0 {
		return s
	}
	b := []byte(s)
	for _, sp := range spans {
		for i := sp[0]; i < sp[1]; i++ {
			b[i] = termTextFill
		}
	}
	return string(b)
}
