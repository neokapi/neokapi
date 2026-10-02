package mdx

import (
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
)

// editSyntax is the markup MDX can read from the text of an edited block, on
// top of Markdown's (markdown.EditSyntax). A '{' opens a JavaScript
// expression that runs when the page is built, and a '}' closes one. A '<'
// followed by anything but whitespace opens JSX or HTML. A line that begins
// with import or export is an ES module statement. The MDX reader leaves an
// expression in a text run, so an edit can keep the expressions its block
// held, spelled the same; one it adds is escaped and reads as text.
var editSyntax = format.AddedSyntax{
	Find:   findEditSyntax,
	Escape: escapeEditSyntax,
	InCode: markdown.EditSyntax.InCode,
}

// maxExpressionSpan bounds how far Find looks for the end of an expression or
// a tag.
const maxExpressionSpan = 4096

func findEditSyntax(s string, i int) (start, end int, ok bool) {
	if backslashEscaped(s, i) {
		return 0, 0, false
	}
	switch c := s[i]; {
	case c == '{':
		return i, braceEnd(s, i), true
	case c == '}':
		return i, i + 1, true
	case c == '<':
		if i+1 < len(s) && !isSpace(s[i+1]) {
			limit := min(len(s), i+maxExpressionSpan)
			if j := strings.IndexByte(s[i+1:limit], '>'); j >= 0 {
				return i, i + 1 + j + 1, true
			}
			return i, limit, true
		}
		return 0, 0, false
	case (c == 'i' || c == 'e') && (i == 0 || s[i-1] == '\n') && beginsModuleStatement(s[i:]):
		if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
			return i, i + j, true
		}
		return i, len(s), true
	}
	return markdown.EditSyntax.Find(s, i)
}

// escapeEditSyntax backslash-escapes a delimiter and writes the first letter
// of an import or export as a character reference, which MDX reads as the
// letter and never as a keyword.
func escapeEditSyntax(s string, i int) (int, string) {
	switch s[i] {
	case 'i':
		return 1, "&#105;"
	case 'e':
		return 1, "&#101;"
	}
	return markdown.EditSyntax.Escape(s, i)
}

// beginsModuleStatement reports whether s begins with the import or export
// keyword followed by what can continue a statement.
func beginsModuleStatement(s string) bool {
	for _, kw := range []string{"import", "export"} {
		if strings.HasPrefix(s, kw) && len(s) > len(kw) {
			switch s[len(kw)] {
			case ' ', '\t', '{', '*':
				return true
			}
		}
	}
	return false
}

// braceEnd returns the offset just past the '}' that closes the '{' at s[i],
// counting nested braces, or the end of the bounded span when none does: an
// unclosed expression runs to the end of the document.
func braceEnd(s string, i int) int {
	limit := min(len(s), i+maxExpressionSpan)
	depth := 0
	for j := i; j < limit; j++ {
		switch s[j] {
		case '\\':
			j++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return limit
}

// backslashEscaped reports whether s[i] follows an odd number of backslashes.
func backslashEscaped(s string, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
