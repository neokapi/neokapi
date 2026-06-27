package json

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// streamScanner is the bounded-memory twin of scanner: it tokenises JSON from an
// io.Reader through a small bufio window instead of a fully-buffered []byte, so
// peak memory is O(bufio buffer + current token), not O(document). It emits the
// exact same token sequence (type, raw, value, prefix) as scanner.next() for
// well-formed input, which is what lets a streaming JSON read stay byte-exact and
// drive the same forward, ancestor-only walk the buffered reader uses.
//
// Spike scope (see notes-internal/streaming-tree-formats.md): the streaming
// scanner targets parity on well-formed JSON/JSON5 (objects, arrays, strings with
// escapes/surrogates, numbers, literals, bare identifiers, and //, /* */, #,
// <!-- --> comments). Malformed-input error byte-offsets and the validation-mode
// snippet window are out of scope — those callers keep the buffered scanner.
//
// All lookahead is bounded (<= 10 bytes, well within bufio's 4 KiB window), so a
// bufio.Reader.Peek drives it without holding the document.
type streamScanner struct {
	br  *bufio.Reader
	pos int // running byte offset, mirrors scanner's error-offset positions

	errOffset   int
	errCategory string
}

func newStreamScanner(r io.Reader) *streamScanner {
	return &streamScanner{br: bufio.NewReader(r)}
}

func (s *streamScanner) syntaxErr(offset int, format string, args ...any) error {
	s.errOffset = offset
	s.errCategory = "structure.json-syntax"
	return fmt.Errorf(format, args...)
}

func (s *streamScanner) escapeErr(offset int, format string, args ...any) error {
	s.errOffset = offset
	s.errCategory = "structure.json-unicode-escape"
	return fmt.Errorf(format, args...)
}

// peekByte returns the next byte without consuming it.
func (s *streamScanner) peekByte() (byte, bool) {
	b, err := s.br.Peek(1)
	if err != nil || len(b) == 0 {
		return 0, false
	}
	return b[0], true
}

// peekN returns up to n upcoming bytes without consuming them.
func (s *streamScanner) peekN(n int) []byte {
	b, _ := s.br.Peek(n)
	return b
}

// take consumes the next byte and appends it to out.
func (s *streamScanner) take(out *strings.Builder) (byte, bool) {
	b, err := s.br.ReadByte()
	if err != nil {
		return 0, false
	}
	s.pos++
	out.WriteByte(b)
	return b, true
}

// drop consumes the next byte without recording it (used for single-char tokens).
func (s *streamScanner) drop() {
	if _, err := s.br.ReadByte(); err == nil {
		s.pos++
	}
}

// next returns the next token, mirroring scanner.next().
func (s *streamScanner) next() (token, error) {
	prefix := s.skipWhitespaceAndComments()

	ch, ok := s.peekByte()
	if !ok {
		return token{typ: tokenEOF, prefix: prefix}, nil
	}
	switch ch {
	case '{':
		s.drop()
		return token{typ: tokenObjectStart, raw: "{", prefix: prefix}, nil
	case '}':
		s.drop()
		return token{typ: tokenObjectEnd, raw: "}", prefix: prefix}, nil
	case '[':
		s.drop()
		return token{typ: tokenArrayStart, raw: "[", prefix: prefix}, nil
	case ']':
		s.drop()
		return token{typ: tokenArrayEnd, raw: "]", prefix: prefix}, nil
	case ':':
		s.drop()
		return token{typ: tokenColon, raw: ":", prefix: prefix}, nil
	case ',':
		s.drop()
		return token{typ: tokenComma, raw: ",", prefix: prefix}, nil
	case '"':
		return s.scanString(prefix, '"')
	case '\'':
		return s.scanString(prefix, '\'')
	case 't':
		if s.matchLiteral("true") {
			return s.scanLiteral("true", tokenTrue, prefix)
		}
		return s.scanBareIdentifier(prefix)
	case 'f':
		if s.matchLiteral("false") {
			return s.scanLiteral("false", tokenFalse, prefix)
		}
		return s.scanBareIdentifier(prefix)
	case 'n':
		if s.matchLiteral("null") {
			return s.scanLiteral("null", tokenNull, prefix)
		}
		return s.scanBareIdentifier(prefix)
	default:
		if ch == '-' || (ch >= '0' && ch <= '9') {
			return s.scanNumber(prefix)
		}
		if isIdentStart(ch) {
			return s.scanBareIdentifier(prefix)
		}
		return token{}, s.syntaxErr(s.pos, "json scanner: unexpected character %q at position %d", ch, s.pos)
	}
}

// matchLiteral reports whether `expected` is at the cursor and not followed by an
// identifier-continuation byte (so `true` vs a `trueish` bare identifier).
func (s *streamScanner) matchLiteral(expected string) bool {
	b := s.peekN(len(expected) + 1)
	if len(b) < len(expected) || string(b[:len(expected)]) != expected {
		return false
	}
	if len(b) > len(expected) && isIdentCont(b[len(expected)]) {
		return false
	}
	return true
}

func (s *streamScanner) scanLiteral(expected string, typ tokenType, prefix string) (token, error) {
	b := s.peekN(len(expected))
	if len(b) < len(expected) || string(b) != expected {
		return token{}, s.syntaxErr(s.pos, "json scanner: expected %q at position %d", expected, s.pos)
	}
	_, _ = s.br.Discard(len(expected))
	s.pos += len(expected)
	return token{typ: typ, raw: expected, value: expected, prefix: prefix}, nil
}

func (s *streamScanner) scanBareIdentifier(prefix string) (token, error) {
	var raw strings.Builder
	for {
		ch, ok := s.peekByte()
		if !ok || !isIdentCont(ch) {
			break
		}
		s.take(&raw)
	}
	r := raw.String()
	return token{typ: tokenString, raw: r, value: r, prefix: prefix}, nil
}

// skipWhitespaceAndComments consumes whitespace and comments, returning the bytes
// for skeleton preservation. Mirrors scanner.skipWhitespaceAndComments for
// well-formed input.
func (s *streamScanner) skipWhitespaceAndComments() string {
	var out strings.Builder
	for {
		ch, ok := s.peekByte()
		if !ok {
			break
		}
		// ASCII whitespace.
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			s.take(&out)
			continue
		}
		// Multi-byte Unicode whitespace (NBSP, etc.) + U+FEFF.
		if ch >= 0x80 {
			b := s.peekN(4)
			r, size := utf8.DecodeRune(b)
			if r != utf8.RuneError && (unicode.IsSpace(r) || r == 0xFEFF) {
				for range size {
					s.take(&out)
				}
				continue
			}
			break
		}
		two := s.peekN(2)
		// // line comment.
		if ch == '/' && len(two) == 2 && two[1] == '/' {
			s.take(&out)
			s.take(&out)
			for {
				c, ok := s.peekByte()
				if !ok || c == '\n' {
					break
				}
				s.take(&out)
			}
			continue
		}
		// /* block comment */ (supports nesting).
		if ch == '/' && len(two) == 2 && two[1] == '*' {
			s.take(&out)
			s.take(&out)
			depth := 1
			for depth > 0 {
				pair := s.peekN(2)
				if len(pair) < 2 {
					// Unterminated at EOF: consume the remainder.
					for {
						if _, ok := s.take(&out); !ok {
							break
						}
					}
					break
				}
				switch {
				case pair[0] == '/' && pair[1] == '*':
					s.take(&out)
					s.take(&out)
					depth++
				case pair[0] == '*' && pair[1] == '/':
					s.take(&out)
					s.take(&out)
					depth--
				default:
					s.take(&out)
				}
			}
			continue
		}
		// # hash line comment.
		if ch == '#' {
			s.take(&out)
			for {
				c, ok := s.peekByte()
				if !ok || c == '\n' {
					break
				}
				s.take(&out)
			}
			continue
		}
		// <!-- html comment -->.
		if ch == '<' {
			four := s.peekN(4)
			if len(four) == 4 && four[1] == '!' && four[2] == '-' && four[3] == '-' {
				s.take(&out)
				s.take(&out)
				s.take(&out)
				s.take(&out)
				for {
					three := s.peekN(3)
					if len(three) == 3 && three[0] == '-' && three[1] == '-' && three[2] == '>' {
						s.take(&out)
						s.take(&out)
						s.take(&out)
						break
					}
					if _, ok := s.take(&out); !ok {
						break
					}
				}
				continue
			}
		}
		break
	}
	return out.String()
}

func (s *streamScanner) scanString(prefix string, quote byte) (token, error) {
	start := s.pos
	var raw, decoded strings.Builder
	s.take(&raw) // opening quote

	for {
		ch, ok := s.peekByte()
		if !ok {
			return token{}, s.syntaxErr(start, "json scanner: unterminated string at position %d", start)
		}
		if ch == quote {
			s.take(&raw) // closing quote
			return token{typ: tokenString, raw: raw.String(), value: decoded.String(), prefix: prefix}, nil
		}
		if ch == '\\' {
			s.take(&raw) // backslash
			esc, ok := s.peekByte()
			if !ok {
				return token{}, s.syntaxErr(s.pos, "json scanner: unexpected end of string escape at position %d", s.pos)
			}
			switch esc {
			case '"':
				s.take(&raw)
				decoded.WriteByte('"')
			case '\'':
				s.take(&raw)
				decoded.WriteByte('\'')
			case '\\':
				s.take(&raw)
				decoded.WriteByte('\\')
			case '/':
				s.take(&raw)
				decoded.WriteByte('/')
			case 'b':
				s.take(&raw)
				decoded.WriteByte('\b')
			case 'f':
				s.take(&raw)
				decoded.WriteByte('\f')
			case 'n':
				s.take(&raw)
				decoded.WriteByte('\n')
			case 'r':
				s.take(&raw)
				decoded.WriteByte('\r')
			case 't':
				s.take(&raw)
				decoded.WriteByte('\t')
			case 'u':
				s.take(&raw) // the 'u'
				r, err := s.scanUnicodeEscape(&raw)
				if err != nil {
					return token{}, err
				}
				decoded.WriteRune(r)
			default:
				s.take(&raw)
				decoded.WriteByte('\\')
				decoded.WriteByte(esc)
			}
			continue
		}
		// Regular character (possibly multi-byte UTF-8).
		b := s.peekN(4)
		r, size := utf8.DecodeRune(b)
		for range size {
			s.take(&raw)
		}
		decoded.WriteRune(r)
	}
}

// scanUnicodeEscape reads a \uXXXX (and optional surrogate pair) escape; the
// cursor is at the first hex digit after \u. It appends the consumed hex bytes to
// raw and returns the decoded rune.
func (s *streamScanner) scanUnicodeEscape(raw *strings.Builder) (rune, error) {
	h := s.peekN(4)
	if len(h) < 4 {
		return 0, s.escapeErr(s.pos, "json scanner: incomplete unicode escape at position %d", s.pos)
	}
	r1, err := strconv.ParseUint(string(h), 16, 32)
	if err != nil {
		return 0, s.escapeErr(s.pos, "json scanner: invalid unicode escape \\u%s at position %d", string(h), s.pos)
	}
	for range 4 {
		s.take(raw)
	}
	// Surrogate pair: high surrogate followed by \uXXXX low surrogate.
	if r1 >= 0xD800 && r1 <= 0xDBFF {
		nb := s.peekN(6) // \ u X X X X
		if len(nb) == 6 && nb[0] == '\\' && nb[1] == 'u' {
			r2, err := strconv.ParseUint(string(nb[2:6]), 16, 32)
			if err == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
				for range 6 {
					s.take(raw)
				}
				return 0x10000 + (rune(r1)-0xD800)*0x400 + (rune(r2) - 0xDC00), nil
			}
		}
	}
	return rune(r1), nil
}

func (s *streamScanner) scanNumber(prefix string) (token, error) {
	var raw strings.Builder
	digits := func() {
		for {
			c, ok := s.peekByte()
			if !ok || c < '0' || c > '9' {
				return
			}
			s.take(&raw)
		}
	}
	if c, ok := s.peekByte(); ok && c == '-' {
		s.take(&raw)
	}
	// Integer part.
	if c, ok := s.peekByte(); ok && c == '0' {
		s.take(&raw)
	} else {
		digits()
	}
	// Fraction.
	if c, ok := s.peekByte(); ok && c == '.' {
		s.take(&raw)
		digits()
	}
	// Exponent.
	if c, ok := s.peekByte(); ok && (c == 'e' || c == 'E') {
		s.take(&raw)
		if c, ok := s.peekByte(); ok && (c == '+' || c == '-') {
			s.take(&raw)
		}
		digits()
	}
	r := raw.String()
	return token{typ: tokenNumber, raw: r, value: r, prefix: prefix}, nil
}
