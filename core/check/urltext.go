package check

import (
	"regexp"
	"strings"
)

// URLSpans returns the byte range of every URL written out in text: a scheme
// and "://" followed by the address, or a host that starts with "www.". A URL
// is an address, never prose, so a word rule, a voice pattern and a fix stay
// out of it, wherever a format reader left it as text.
//
// The span follows the extended autolink reading of GitHub Flavored Markdown,
// which is how a reader of the file sees where the address ends. It stops at
// whitespace, a quote, a backtick or an angle bracket. Trailing punctuation
// (. , : ; ! ? * _ ~) ends a sentence rather than the address and stays
// outside the span, and so does a closing parenthesis or bracket the address
// does not open.
//
// A markdown link or image keeps its destination in the code that closes it,
// and an autolink in angle brackets is a placeholder, so neither reaches this
// scan through a block's text. A URL written as a link's visible text, or
// written bare, does.
func URLSpans(text string) [][2]int {
	if !strings.Contains(text, "://") && !strings.Contains(text, "www.") {
		return nil
	}
	var out [][2]int
	for _, m := range urlPattern.FindAllStringSubmatchIndex(text, -1) {
		start := m[2]
		if end := start + urlLength(text[start:m[3]]); end > start {
			out = append(out, [2]int{start, end})
		}
	}
	return out
}

// urlPattern matches a URL, the submatch, after a character that must not be
// a letter, a digit or a character a URL could continue from.
var urlPattern = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_@./:+-])((?:[A-Za-z][A-Za-z0-9+.-]*://|[Ww][Ww][Ww]\.)[^\s<>"'` + "`" + `\x{201C}\x{201D}\x{2018}\x{2019}]+)`)

// urlLength is the length of the address at the start of s, with the
// trailing punctuation and unbalanced closing brackets removed.
func urlLength(s string) int {
	end := len(s)
	for end > 0 {
		switch c := s[end-1]; c {
		case '.', ',', ':', ';', '!', '?', '*', '_', '~':
			end--
			continue
		case ')', ']':
			open := byte('(')
			if c == ']' {
				open = '['
			}
			if strings.Count(s[:end], string(open)) < strings.Count(s[:end], string(c)) {
				end--
				continue
			}
		}
		break
	}
	return end
}

// InURL reports whether the byte range [start, end) of text touches a URL
// written in it (URLSpans).
func InURL(text string, start, end int) bool {
	for _, sp := range URLSpans(text) {
		if start < sp[1] && end > sp[0] {
			return true
		}
	}
	return false
}
