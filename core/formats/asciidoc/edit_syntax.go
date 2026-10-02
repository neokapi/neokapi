package asciidoc

import (
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// editSyntax is the markup AsciiDoc can read from the text of an edited
// block, as far as it reaches past the text: a raw passthrough (`+++…+++`,
// a `++++` block), an inline or block macro (`pass:[…]`, `link:…[…]`,
// `image:…[…]`, `include::…[]`, a URL with link text), a block attribute
// list at the start of a line (`[pass]`), an attribute reference (`{name}`)
// and a cross reference (`<<id>>`). The reader leaves the passthroughs and the
// macros it does not model in text runs, so an edit can keep the ones its
// block held, spelled the same; one it adds is escaped.
//
// Monospace text is processed like any other text, so the markup inside it
// counts too.
//
// The escape writes the opening character as a character reference
// (`&#91;` for '['): the processor extracts passthroughs and matches
// attribute references and macros before it restores references, so the
// character reads as text and the page shows it unchanged.
var editSyntax = format.AddedSyntax{
	Find:   findEditSyntax,
	Escape: escapeEditSyntax,
}

// maxSyntaxSpan bounds how far Find looks for the end of markup.
const maxSyntaxSpan = 4096

// attrRefRE matches an attribute reference, including the {set:…} and
// {counter:…} directives, at the start of a string.
var attrRefRE = regexp.MustCompile(`^\{[A-Za-z0-9_][^{}\n]*\}`)

func findEditSyntax(s string, i int) (start, end int, ok bool) {
	limit := min(len(s), i+maxSyntaxSpan)
	switch s[i] {
	case '+':
		if i > 0 && s[i-1] == '+' {
			return 0, 0, false
		}
		n := plusRun(s, i)
		if n < 3 {
			return 0, 0, false
		}
		// A raw passthrough runs to the next run of three or more; an
		// unclosed one, or a `++++` block delimiter, runs to the end.
		for j := i + n; j < limit; j++ {
			if s[j] == '+' && s[j-1] != '+' {
				if m := plusRun(s, j); m >= 3 {
					return i, j + m, true
				}
			}
		}
		return i, limit, true
	case '[':
		j := i
		for j > 0 && !isSpaceByte(s[j-1]) {
			j--
		}
		atLineStart := j == i && (i == 0 || s[i-1] == '\n')
		if !strings.Contains(s[j:i], ":") && !atLineStart && !strings.HasPrefix(s[i:], "[[") {
			return 0, 0, false
		}
		if e := closingBracket(s, i, limit); e >= 0 {
			return j, e + 1, true
		}
		return j, limit, true
	case '{':
		if m := attrRefRE.FindString(s[i:limit]); m != "" {
			return i, i + len(m), true
		}
	case '<':
		if strings.HasPrefix(s[i:], "<<") && (i == 0 || s[i-1] != '<') {
			if e := strings.Index(s[i+2:limit], ">>"); e >= 0 {
				return i, i + 2 + e + 2, true
			}
		}
	}
	return 0, 0, false
}

// escapeEditSyntax writes the character that opens markup as a character
// reference: every '+' of a passthrough's opening run, or the one '[', '{'
// or '<'.
func escapeEditSyntax(s string, i int) (int, string) {
	switch s[i] {
	case '+':
		n := plusRun(s, i)
		return n, strings.Repeat("&#43;", n)
	case '[':
		return 1, "&#91;"
	case '{':
		return 1, "&#123;"
	case '<':
		return 1, "&#60;"
	}
	return 1, s[i : i+1]
}

func plusRun(s string, i int) int {
	n := 0
	for i+n < len(s) && s[i+n] == '+' {
		n++
	}
	return n
}

// closingBracket returns the offset of the ']' that closes the '[' at s[i],
// counting nested brackets and skipping backslash-escaped ones, or -1.
func closingBracket(s string, i, limit int) int {
	depth := 0
	for j := i; j < limit; j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
