package icu

import (
	"strings"
	"unicode/utf8"
)

// SyntaxSpans returns the byte ranges of msg that are MessageFormat syntax
// rather than text a reader sees.
//
// A simple or typed argument ({vessel}, {draught, number}) is one range. Of a
// plural, select or selectordinal, the head ({count, plural,), the offset, the
// branch keywords, the braces around each branch and the closing brace are
// ranges, and so is a # inside a branch, where it stands for the formatted
// number; the text of each branch is not. Quoted literal text is text.
// Everything outside the ranges is therefore the literal text of the message,
// at the offset it has in msg.
//
// The ranges are in order and do not overlap. msg is expected to parse (see
// Parse). A brace that never closes opens no range, so a message that does not
// parse reads as text rather than being lost.
func SyntaxSpans(msg string) []Span {
	var out []Span
	syntaxIn(msg, 0, len(msg), false, &out)
	return out
}

// syntaxIn collects the syntax ranges of the message text s[start:end].
// inBranch is true inside a picker's branch, where the parser reads # as the
// number (see parsePattern).
func syntaxIn(s string, start, end int, inBranch bool, out *[]Span) {
	i := start
	for i < end {
		switch s[i] {
		case '\'':
			SkipQuoted(s, &i)
		case '#':
			if inBranch {
				*out = append(*out, Span{Start: i, End: i + 1})
			}
			i++
		case '{':
			closing, ok := MatchBrace(s, i)
			if !ok || closing >= end {
				i++
				continue
			}
			argumentSyntax(s, i, closing, out)
			i = closing + 1
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
}

// argumentSyntax collects the syntax ranges of the argument s[open:closing+1].
// The branches of a picker are message text and are walked for their own
// syntax; every other argument is syntax whole.
func argumentSyntax(s string, open, closing int, out *[]Span) {
	whole := Span{Start: open, End: closing + 1}

	nameLen := strings.IndexAny(s[open+1:closing], ",{")
	if nameLen < 0 || s[open+1+nameLen] != ',' {
		*out = append(*out, whole)
		return
	}
	typeStart := open + 1 + nameLen + 1
	typeLen := strings.IndexAny(s[typeStart:closing], ",{")
	if typeLen < 0 || s[typeStart+typeLen] != ',' {
		*out = append(*out, whole)
		return
	}
	switch strings.ToLower(strings.TrimSpace(s[typeStart : typeStart+typeLen])) {
	case "plural", "select", "selectordinal":
	default:
		*out = append(*out, whole)
		return
	}

	// From the head to the first branch's opening brace is syntax, and so is
	// everything from a branch's closing brace to the next branch's opening
	// one: the offset, the keywords and the whitespace between them.
	from := open
	for k := typeStart + typeLen + 1; k < closing; k++ {
		if s[k] != '{' {
			continue
		}
		body, ok := MatchBrace(s, k)
		if !ok || body >= closing {
			break
		}
		*out = append(*out, Span{Start: from, End: k + 1})
		syntaxIn(s, k+1, body, true, out)
		from = body
		k = body
	}
	*out = append(*out, Span{Start: from, End: closing + 1})
}
