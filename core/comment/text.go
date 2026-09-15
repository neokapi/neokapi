package comment

import (
	"strings"
	"unicode/utf8"
)

// TextLines splits the text a rewrite writes into lines, refusing what no
// comment may hold and dropping blank lines at either edge. A comment is
// rewritten, never removed, so an empty text is refused too.
func TextLines(text string) ([]string, error) {
	if !utf8.ValidString(text) {
		return nil, refuse(RefusedText, "the text is not valid UTF-8")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, r := range text {
		if reason := unwritable(r); reason != "" {
			return nil, refuse(RefusedText, "the text holds %s (U+%04X)", reason, r)
		}
	}
	lines := strings.Split(text, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil, refuse(RefusedText, "the text is empty, and a comment is rewritten here, never removed")
	}
	return lines, nil
}

// unwritable names what r is when a comment must not hold it, and returns ""
// for a character it may hold.
func unwritable(r rune) string {
	switch {
	case r == '\n' || r == '\t':
		return ""
	case r == '\r':
		return "a carriage return that ends no line"
	case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
		return "a control character"
	case r == '\uFEFF':
		return "a byte order mark"
	case r == '\u2028' || r == '\u2029':
		return "a line or paragraph separator, which an editor may show as a line break"
	case (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069'):
		return "a bidirectional control, which can make the code around the comment display out of order"
	}
	return ""
}
