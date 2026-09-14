package comments

import (
	"bytes"
)

// cDialect is how a lexical scan of C or C++ reads the constructs the dialects
// of the two languages read differently.
type cDialect struct {
	// rawStrings reads `R"delim( ... )delim"` as one literal, as C++ and GNU C
	// do. ISO C reads the `R` as an identifier and the rest as an ordinary
	// string.
	rawStrings bool
	// trigraphs reads `??/` as a backslash, as ISO C before C23 does.
	trigraphs bool
}

// cComments returns the spans of the comments in a C or C++ file, read from the
// file's characters alone. A line comment's span runs from its `//` to the line
// break that ends it, past every line splice, and a block comment's span holds
// both delimiters.
//
// A file holding a construct the dialects read differently, a raw string prefix
// in C or the trigraph `??/` in either language, is scanned again the other
// way, and when the comments differ the file cannot be read either way.
func cComments(src []byte, d cDialect) ([][2]int, error) {
	spans, rawPrefix, err := lexC(src, d)
	if err != nil {
		return nil, err
	}
	type reading struct {
		dialect cDialect
		what    string
	}
	var others []reading
	if rawPrefix {
		others = append(others, reading{cDialect{rawStrings: true, trigraphs: d.trigraphs}, "raw string literals"})
	}
	if bytes.Contains(src, []byte("??/")) {
		others = append(others, reading{cDialect{rawStrings: d.rawStrings, trigraphs: !d.trigraphs}, "trigraphs"})
	}
	for _, r := range others {
		other, _, err := lexC(src, r.dialect)
		if err != nil {
			return nil, &lexError{offset: err.offset, reason: "the file reads differently with and without " + r.what}
		}
		if at, differ := firstDifference(spans, other); differ {
			return nil, &lexError{offset: at, reason: "the comments read differently with and without " + r.what}
		}
	}
	return spans, nil
}

// firstDifference returns where two sorted span lists first differ.
func firstDifference(a, b [][2]int) (int, bool) {
	for i := 0; i < len(a) || i < len(b); i++ {
		switch {
		case i >= len(a):
			return b[i][0], true
		case i >= len(b):
			return a[i][0], true
		case a[i] != b[i]:
			return min(a[i][0], b[i][0]), true
		}
	}
	return 0, false
}

// directiveState tracks where a header name may follow.
type directiveState int

const (
	// outside is any place a header name cannot follow.
	outside directiveState = iota
	// directiveName follows the `#` that opens a directive.
	directiveName
	// hasInclude follows `__has_include`, before its parenthesis.
	hasInclude
	// headerName follows `#include`, `#include_next`, `#import` or
	// `__has_include(`, where `<` opens a header name.
	headerName
)

// cLexer reads C or C++ characters once line splices are removed.
type cLexer struct {
	src []byte
	d   cDialect
}

// lexC returns the comment spans in src, and whether it holds a raw string
// prefix that d reads as an identifier.
func lexC(src []byte, d cDialect) ([][2]int, bool, *lexError) {
	l := &cLexer{src: src, d: d}
	var spans [][2]int
	rawPrefix := false
	// lineStart reports that nothing but whitespace and comments precedes the
	// current place on its line, so a `#` there opens a directive.
	lineStart := true
	state := outside
	for i := 0; ; {
		at, next, c := l.char(i)
		if at >= len(src) {
			return spans, rawPrefix, nil
		}
		switch {
		case c == '\n' || c == '\r':
			lineStart, state = true, outside
			i = next
			continue
		case c == ' ' || c == '\t' || c == '\f' || c == '\v':
			i = next
			continue
		case c == '/':
			_, after, c2 := l.char(next)
			switch c2 {
			case '/':
				end := l.lineEnd(after)
				for end > at && (src[end-1] == '\n' || src[end-1] == '\r') {
					end--
				}
				spans = append(spans, [2]int{at, end})
				i = end
				continue
			case '*':
				end, ok := l.blockEnd(after)
				if !ok {
					return nil, rawPrefix, &lexError{offset: at, reason: "a block comment is never closed"}
				}
				spans = append(spans, [2]int{at, end})
				i = end
				continue
			}
		case c == '"' || c == '\'':
			i = l.literal(next, c)
			lineStart, state = false, outside
			continue
		case isDigit(c) || c == '.' && isDigit(l.peek(next)):
			i = l.number(next)
			lineStart, state = false, outside
			continue
		case isIdentifier(c):
			word, end := l.identifier(at)
			_, afterQuote, q := l.char(end)
			switch word {
			case "L", "u", "U", "u8":
				if q == '"' || q == '\'' {
					i = l.literal(afterQuote, q)
					lineStart, state = false, outside
					continue
				}
			case "R", "LR", "uR", "UR", "u8R":
				if q == '"' {
					if !d.rawStrings {
						rawPrefix = true
						break
					}
					end, err := l.rawString(at, afterQuote)
					if err != nil {
						return nil, rawPrefix, err
					}
					i = end
					lineStart, state = false, outside
					continue
				}
			}
			switch {
			case state == directiveName && (word == "include" || word == "include_next" || word == "import"):
				state = headerName
			case word == "__has_include" || word == "__has_include_next":
				state = hasInclude
			default:
				state = outside
			}
			lineStart = false
			i = end
			continue
		case c == '#' || c == '%' && l.peek(next) == ':':
			if c == '%' {
				_, next, _ = l.char(next)
			}
			if lineStart {
				state = directiveName
			} else {
				state = outside
			}
			lineStart = false
			i = next
			continue
		case c == '(' && state == hasInclude:
			state = headerName
			i = next
			continue
		case c == '<' && state == headerName:
			if end, ok := l.headerName(next); ok {
				state = outside
				i = end
				continue
			}
		}
		lineStart, state = false, outside
		i = next
	}
}

// splice returns the length of the line splice at i, a backslash followed by
// any spaces or tabs and a line break, or 0 when none starts there.
func (l *cLexer) splice(i int) int {
	j := i
	switch {
	case j < len(l.src) && l.src[j] == '\\':
		j++
	case l.d.trigraphs && bytes.HasPrefix(l.src[j:], []byte("??/")):
		j += 3
	default:
		return 0
	}
	for j < len(l.src) && (l.src[j] == ' ' || l.src[j] == '\t' || l.src[j] == '\f' || l.src[j] == '\v') {
		j++
	}
	switch {
	case bytes.HasPrefix(l.src[j:], []byte("\r\n")):
		return j + 2 - i
	case j < len(l.src) && (l.src[j] == '\n' || l.src[j] == '\r'):
		return j + 1 - i
	}
	return 0
}

// char returns the character at i once any line splices there are skipped:
// where it sits, the offset after it, and the character itself. At the end of
// the file at is len(src) and the character is 0.
func (l *cLexer) char(i int) (at, next int, c byte) {
	for n := l.splice(i); n > 0; n = l.splice(i) {
		i += n
	}
	if i >= len(l.src) {
		return len(l.src), len(l.src), 0
	}
	if l.d.trigraphs && bytes.HasPrefix(l.src[i:], []byte("??/")) {
		return i, i + 3, '\\'
	}
	return i, i + 1, l.src[i]
}

// peek returns the character at i, or 0 at the end of the file.
func (l *cLexer) peek(i int) byte {
	_, _, c := l.char(i)
	return c
}

// lineEnd returns where the line holding i ends: the offset of the line break
// no splice removes, or the end of the file.
func (l *cLexer) lineEnd(i int) int {
	for {
		at, next, c := l.char(i)
		if at >= len(l.src) || c == '\n' || c == '\r' {
			return at
		}
		i = next
	}
}

// blockEnd returns the offset after the `*/` that closes a block comment whose
// opener ends before i.
func (l *cLexer) blockEnd(i int) (int, bool) {
	for {
		at, next, c := l.char(i)
		if at >= len(l.src) {
			return 0, false
		}
		if c == '*' {
			if _, after, c2 := l.char(next); c2 == '/' {
				return after, true
			}
		}
		i = next
	}
}

// literal returns the offset after a string or character literal whose opening
// quote ends before i: after its closing quote, or at the line break that
// leaves it unterminated, as a compiler reads one.
func (l *cLexer) literal(i int, quote byte) int {
	for {
		at, next, c := l.char(i)
		switch {
		case at >= len(l.src), c == '\n', c == '\r':
			return at
		case c == quote:
			return next
		case c == '\\':
			escaped, after, e := l.char(next)
			if escaped >= len(l.src) || e == '\n' || e == '\r' {
				return escaped
			}
			next = after
		}
		i = next
	}
}

// rawString returns the offset after a raw string literal that opens at start,
// whose quote ends before i. Its delimiter and content are read as written,
// splices included.
func (l *cLexer) rawString(start, i int) (int, *lexError) {
	j := i
	for j < len(l.src) && l.src[j] != '(' {
		if c := l.src[j]; j-i == 16 || c <= ' ' || c == ')' || c == '\\' || c >= 0x7f {
			return 0, &lexError{offset: start, reason: "a raw string's delimiter is malformed"}
		}
		j++
	}
	if j >= len(l.src) {
		return 0, &lexError{offset: start, reason: "a raw string's delimiter is malformed"}
	}
	closing := make([]byte, 0, j-i+2)
	closing = append(append(append(closing, ')'), l.src[i:j]...), '"')
	k := bytes.Index(l.src[j+1:], closing)
	if k < 0 {
		return 0, &lexError{offset: start, reason: "a raw string is never closed"}
	}
	return j + 1 + k + len(closing), nil
}

// number returns the offset after a preprocessing number whose first character
// ends before i. A quote between two characters of the number, as in `1'000`,
// separates its digits and opens no character literal.
func (l *cLexer) number(i int) int {
	prev := byte(0)
	for {
		at, next, c := l.char(i)
		switch {
		case at >= len(l.src):
			return at
		case isIdentifier(c) || isDigit(c) || c == '.':
		case (c == '+' || c == '-') && (prev == 'e' || prev == 'E' || prev == 'p' || prev == 'P'):
		case c == '\'':
			_, after, d := l.char(next)
			if !isDigit(d) && !isASCIILetter(d) && d != '_' {
				return at
			}
			c, next = d, after
		default:
			return at
		}
		prev = c
		i = next
	}
}

// identifier returns the identifier that starts at i, spelled without line
// splices when it is short enough to be a literal prefix or a directive name,
// and the offset after it.
func (l *cLexer) identifier(i int) (string, int) {
	var word [18]byte
	n := 0
	for {
		at, next, c := l.char(i)
		if at >= len(l.src) || !isIdentifier(c) && !isDigit(c) {
			if n > len(word) {
				return "", at
			}
			return string(word[:n]), at
		}
		if n < len(word) {
			word[n] = c
		}
		n++
		i = next
	}
}

// headerName returns the offset after a header name whose `<` ends before i,
// and false when the line ends before its `>`, so the `<` is an operator.
func (l *cLexer) headerName(i int) (int, bool) {
	for {
		at, next, c := l.char(i)
		switch {
		case at >= len(l.src), c == '\n', c == '\r':
			return 0, false
		case c == '>':
			return next, true
		}
		i = next
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// isIdentifier reports whether c may start an identifier: a letter, `_`, `$`,
// or a byte of a UTF-8 sequence.
func isIdentifier(c byte) bool { return isASCIILetter(c) || c == '_' || c == '$' || c >= 0x80 }
