package check

import (
	"regexp"
	"slices"
	"strings"
)

// URLSpans returns the byte range of every URL written out in text: a scheme
// and "://" followed by the address, a host that starts with "www.", or a bare
// host name such as example.com (hostSpans). A URL is an address, never prose,
// so word rules, voice patterns and fixes stay out of it wherever a format
// reader left it as text.
//
// The span follows the extended autolink reading of GitHub Flavored Markdown.
// It stops at whitespace, a quote, a backtick or an angle bracket. Trailing
// punctuation (. , : ; ! ? * _ ~) stays outside the span, and so does a closing
// parenthesis or bracket the address does not open.
//
// A markdown link or image keeps its destination in the code that closes it,
// and an autolink in angle brackets is a placeholder, so neither reaches a
// block's text. A bare URL, or one written as a link's text, does.
func URLSpans(text string) [][2]int {
	var out [][2]int
	if strings.Contains(text, "://") || strings.Contains(text, "www.") {
		for _, m := range urlPattern.FindAllStringSubmatchIndex(text, -1) {
			start := m[2]
			if end := start + urlLength(text[start:m[3]]); end > start {
				out = append(out, [2]int{start, end})
			}
		}
	}
	for _, sp := range hostSpans(text) {
		if !inAnySpan(out, sp[0]) {
			out = append(out, sp)
		}
	}
	return out
}

// hostSpans returns the byte range of every bare host name in text, with the
// port and path written after it.
//
// A host name is two or more labels of letters, digits and hyphens joined by
// dots, whose last label is one of hostTopLabels in lower case. It starts
// where a URL may start (urlPattern), and a letter, digit, hyphen or
// underscore straight after it makes the word something else. A path after
// it ends as a URL's address ends (urlLength).
//
// The last label tells a host from other dotted words:
//
//   - a version number, 1.3.0, ends in digits;
//   - a file name, kapi.yaml or example.com.json, ends in an extension, and
//     no extension is on the list;
//   - a sentence boundary with no space, "kapi.The", ends in a capital;
//   - a name in code, cfg.Name or log.info, ends in a capital or a word off
//     the list.
func hostSpans(text string) [][2]int {
	if !containsTopLabel(text) {
		return nil
	}
	var out [][2]int
	for _, m := range hostPattern.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[2], m[3]
		if !slices.Contains(hostTopLabels, text[m[4]:m[5]]) {
			continue
		}
		if end < len(text) && isLabelByte(text[end]) {
			continue
		}
		if m[6] >= 0 {
			end += urlLength(text[end:m[7]])
		}
		out = append(out, [2]int{start, end})
	}
	return out
}

// hostTopLabels are the top-level labels a bare host name may end in: the
// generic ones text writes most, and none that is also a common file
// extension. The list leaves out app for macOS bundle names (Kapi.app), sh for
// shell scripts and md for Markdown files. A host under a label off the list,
// such as example.no, is read as prose; written with its scheme, it is a URL.
var hostTopLabels = []string{"com", "org", "net", "edu", "gov", "io", "dev", "ai", "cloud"}

// hostPattern matches a host name, the first submatch, after a character a
// URL may start after (urlPattern). The second submatch is the last label, and
// the third the port and path written after the host.
var hostPattern = regexp.MustCompile(
	`(?:^|[^A-Za-z0-9_@./:+-])((?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)+([a-z]{2,}))` +
		`((?::[0-9]+)?/[^\s<>"'` + "`" + `\x{201C}\x{201D}\x{2018}\x{2019}]*|:[0-9]+)?`)

// containsTopLabel reports whether text holds a full stop followed by one of
// hostTopLabels, as every bare host name does.
func containsTopLabel(text string) bool {
	for _, label := range hostTopLabels {
		if strings.Contains(text, "."+label) {
			return true
		}
	}
	return false
}

// isLabelByte reports whether c continues a host label or a name in code.
func isLabelByte(c byte) bool {
	return c == '-' || c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
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
