// Package icu reads ICU MessageFormat: the argument, plural, select and
// selectordinal syntax that carries program semantics inside an otherwise
// translatable string.
//
// It is the framework's single reader of that syntax. The formats that protect
// an ICU message from a translator, the check that verifies a translation kept
// what the program needs, and the providers that must not translate through a
// picker all parse here, so a brace that contains braces means the same thing
// everywhere.
package icu

import (
	"strings"
	"unicode/utf8"
)

// quoteOpeners are the syntax characters an apostrophe must precede to open a
// span of quoted literal text.
//
// This is ICU's own DOUBLE_OPTIONAL apostrophe mode: an apostrophe opens quoted
// literal text only when the next character is one of these, a doubled
// apostrophe stands for a literal apostrophe, and any other apostrophe stands
// for itself. An apostrophe in prose ("Don't") is therefore prose, and does not
// open a span that swallows the rest of the message.
const quoteOpeners = "{}#|"

// SkipQuoted advances *i past the apostrophe span that starts at s[*i], which
// the caller has established is an apostrophe. The span is one of: a lone
// apostrophe standing for itself, a doubled apostrophe, or quoted literal text
// running to the next lone apostrophe (or to the end of s, which ICU accepts).
func SkipQuoted(s string, i *int) {
	n := len(s)
	*i++ // past the opening apostrophe
	if *i >= n {
		return // a trailing apostrophe stands for itself
	}
	if s[*i] == '\'' {
		*i++ // a doubled apostrophe stands for one apostrophe
		return
	}
	if strings.IndexByte(quoteOpeners, s[*i]) < 0 {
		return // an apostrophe in prose opens nothing
	}
	for *i < n {
		if s[*i] == '\'' {
			*i++
			if *i < n && s[*i] == '\'' {
				*i++ // a doubled apostrophe inside the span
				continue
			}
			return
		}
		_, size := utf8.DecodeRuneInString(s[*i:])
		*i += size
	}
}

// Unquote returns the literal text an apostrophe span stands for. The span is
// the slice SkipQuoted consumed: a lone apostrophe, a doubled apostrophe, or
// quoted literal text with its delimiters.
func Unquote(span string) string {
	if len(span) == 0 || span[0] != '\'' {
		return span
	}
	if span == "'" || span == "''" {
		return "'"
	}
	inner := strings.TrimSuffix(span[1:], "'")
	return strings.ReplaceAll(inner, "''", "'")
}

// MatchBrace returns the index of the '}' that closes the '{' at start,
// counting nested braces and stepping over quoted literal text. ok is false
// when s[start] is not '{', or when the group is never closed.
func MatchBrace(s string, start int) (end int, ok bool) {
	if start < 0 || start >= len(s) || s[start] != '{' {
		return 0, false
	}
	depth := 0
	i := start
	n := len(s)
	for i < n {
		switch s[i] {
		case '\'':
			SkipQuoted(s, &i)
		case '{':
			depth++
			i++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
			i++
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	return 0, false
}

// Span is the byte range of one balanced brace group, End exclusive.
type Span struct {
	Start int
	End   int
}

// Spans returns the balanced brace groups of s that are neither nested inside
// another group nor inside quoted literal text, in order. A simple {name}
// reference and a whole {count, plural, …} picker are both one span: what
// matters to a caller that must not write inside ICU syntax is where the syntax
// starts and ends, not which of the two it is. An unbalanced '{' opens no span.
func Spans(s string) []Span {
	var out []Span
	i := 0
	for i < len(s) {
		switch s[i] {
		case '\'':
			SkipQuoted(s, &i)
		case '{':
			if end, ok := MatchBrace(s, i); ok {
				out = append(out, Span{Start: i, End: end + 1})
				i = end + 1
				continue
			}
			i++
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
	}
	return out
}
