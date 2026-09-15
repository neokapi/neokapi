package check

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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

// termTextFill overwrites a placeholder or a span of program syntax in
// TermText. It is neither a word character nor whitespace, so a term cannot
// match inside an overwritten span, run into one from the adjacent text, or
// bridge one as the gap in a term of several words.
const termTextFill = '\x1f'

// TermText is text as a term is matched against it: every placeholder and
// every span of program syntax overwritten byte for byte, and every word a
// reader sees in prose left where it was, so an offset into the result is an
// offset into s.
//
// It is the projection term matching reads a source through. term-check reads
// the source through it to decide which rules a translation is held to; the
// terms store lookup in terms.Locate reads the content through it; and
// profile.ScopeTermRules reads through it to decide which rules a translate
// prompt carries. A placeholder's name is program syntax, so
// "{vessel} is alongside" does not use the term "vessel". A command a target
// keeps as written is program syntax too, so "Run `kapi check`" does not use
// the term "check", and "check" in the prose beside it does.
//
// The placeholders are the spans PlaceholderText overwrites. The program
// syntax is:
//
//   - inline code and fenced blocks (CodeSpans);
//   - a kapi command in single or double quotes, straight or curly, which may
//     wrap onto one more line, as CLI help writes 'kapi check --staged';
//   - an example line: an indented line, or a line after a "$ " prompt, whose
//     text starts with "kapi ", overwritten from the command to the line end
//     or to a shell comment, a # after a space, which stays prose;
//   - a flag name such as --diff-range that starts a word.
//
// A bare "kapi check" in a sentence stays prose: the text marks nothing that
// tells the command from a sentence whose subject is kapi.
//
// The input is plain text: for a block, the text of its runs, where placeholder
// runs are already absent. TermText reaches the placeholders and the code a
// format reader left inside text runs.
func TermText(s string) string {
	return overwrite(s, append(placeholderSpans(s), programSyntaxSpans(s)...))
}

// PlaceholderText is s with every placeholder overwritten byte for byte, as
// TermText overwrites it, and code, commands and flags left as they are.
//
// profile.MatchTermRules, the declared-term matcher behind the voice
// vocabulary gate and the source terminology gate, reads through it: a rule
// that names no scope matches inside code, so a name written wrongly in a code
// sample is still found, and a rule scoped to prose leaves code out itself.
//
// Which spans are placeholders follows the split the placeholder check makes. A
// text that parses as ICU MessageFormat and chooses between sub-messages is
// read structurally (icu.SyntaxSpans): arguments, picker heads, branch keywords
// and the # a branch formats are overwritten, and the text of every branch is
// kept, so "{count, plural, one {# berth} other {# berths}}" uses "berth". The
// non-brace interpolation styles inside such a message are overwritten too. Any
// other text, including text that does not parse as ICU, is read for the
// placeholder styles of PlaceholderToken, and the rest of it is kept as it is.
func PlaceholderText(s string) string {
	return overwrite(s, placeholderSpans(s))
}

// placeholderSpans returns the byte range of every placeholder in s.
func placeholderSpans(s string) [][2]int {
	if !strings.ContainsAny(s, "{$%<") {
		return nil
	}
	var found [][]int
	if tokens, err := icu.PlaceholderTokens(s); err == nil && tokens.Picker {
		for _, sp := range icu.SyntaxSpans(s) {
			found = append(found, []int{sp.Start, sp.End})
		}
		found = append(found, NonBracePlaceholderToken.FindAllStringIndex(s, -1)...)
	} else {
		found = PlaceholderToken.FindAllStringIndex(s, -1)
	}
	spans := make([][2]int, 0, len(found))
	for _, m := range found {
		spans = append(spans, [2]int{m[0], m[1]})
	}
	return spans
}

// CodeSpans returns the byte range of every fenced block and inline code span
// in text: a fence of ``` or ~~~ to the next fence or the end of the text, and
// a single-backtick span on one line outside any fence.
//
// Byte ranges rather than a parse: this runs over whatever text a caller has,
// which may be a Markdown document, a paragraph out of one, or a block's runs
// joined together. A tolerant scan gets the common shapes right and never
// refuses to answer.
func CodeSpans(text string) [][2]int {
	if !strings.ContainsAny(text, "`~") {
		return nil
	}
	var out [][2]int
	for _, m := range fencePattern.FindAllStringIndex(text, -1) {
		out = append(out, [2]int{m[0], m[1]})
	}
	for _, m := range inlineCodePattern.FindAllStringIndex(text, -1) {
		if !inAnySpan(out, m[0]) {
			out = append(out, [2]int{m[0], m[1]})
		}
	}
	return out
}

// fencePattern matches a fenced block including its fences. Non-greedy to the
// next fence, and tolerant of a block nobody closed.
var fencePattern = regexp.MustCompile("(?s)(?:^|\n)(?:```|~~~).*?(?:\n(?:```|~~~)|$)")

// inlineCodePattern matches a single-backtick span on one line.
var inlineCodePattern = regexp.MustCompile("`[^`\n]+`")

// quotedCommandPattern matches a kapi command in quotes, from the opening
// quote to the closing one, across at most one line break.
var quotedCommandPattern = regexp.MustCompile(
	`'kapi [^'\n]*(?:\n[^'\n]*)?'` +
		`|"kapi [^"\n]*(?:\n[^"\n]*)?"` +
		`|‘kapi [^’\n]*(?:\n[^’\n]*)?’` +
		`|“kapi [^”\n]*(?:\n[^”\n]*)?”`)

// exampleLinePattern matches an example command line. The submatch is the
// command, from "kapi" to the end of the line.
var exampleLinePattern = regexp.MustCompile(`(?m)^(?:[ \t]+(?:\$ )?|\$ )(kapi [^\n]*)`)

// flagNamePattern matches a long flag name that starts a word. The submatch
// is the flag.
var flagNamePattern = regexp.MustCompile(`(?:^|[\s(\[=,;'"‘“])(--[A-Za-z][A-Za-z0-9-]*)`)

// programSyntaxSpans returns the byte range of every span of program syntax in
// s that TermText overwrites beyond the placeholders.
func programSyntaxSpans(s string) [][2]int {
	if !strings.ContainsAny(s, "`~-") && !strings.Contains(s, "kapi ") {
		return nil
	}
	spans := CodeSpans(s)
	for _, m := range quotedCommandPattern.FindAllStringIndex(s, -1) {
		// A closing quote followed by a letter or digit is an apostrophe, as in
		// "'kapi won't", and the span it would close is prose.
		if r, _ := utf8.DecodeRuneInString(s[m[1]:]); unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		spans = append(spans, [2]int{m[0], m[1]})
	}
	for _, m := range exampleLinePattern.FindAllStringSubmatchIndex(s, -1) {
		spans = append(spans, [2]int{m[2], m[2] + commandLength(s[m[2]:m[3]])})
	}
	for _, m := range flagNamePattern.FindAllStringSubmatchIndex(s, -1) {
		spans = append(spans, [2]int{m[2], m[3]})
	}
	return spans
}

// commandLength is the length of an example command line up to its shell
// comment, a # after a space or tab. The comment is prose a translation
// renders, so it stays readable.
func commandLength(line string) int {
	for i := 1; i < len(line); i++ {
		if line[i] == '#' && (line[i-1] == ' ' || line[i-1] == '\t') {
			return i
		}
	}
	return len(line)
}

// overwrite returns s with every byte inside spans replaced by termTextFill.
func overwrite(s string, spans [][2]int) string {
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

// inAnySpan reports whether pos falls inside one of spans.
func inAnySpan(spans [][2]int, pos int) bool {
	for _, sp := range spans {
		if pos >= sp[0] && pos < sp[1] {
			return true
		}
	}
	return false
}
