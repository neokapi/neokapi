package json

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// tokenType identifies the kind of JSON token.
type tokenType int

const (
	tokenObjectStart tokenType = iota // {
	tokenObjectEnd                    // }
	tokenArrayStart                   // [
	tokenArrayEnd                     // ]
	tokenColon                        // :
	tokenComma                        // ,
	tokenString                       // "..."
	tokenNumber                       // 123, 1.5, -3e10
	tokenTrue                         // true
	tokenFalse                        // false
	tokenNull                         // null
	tokenEOF                          // end of input
)

// token represents a single JSON token with its surrounding whitespace/comments.
type token struct {
	typ    tokenType
	raw    string // raw bytes of the token (for non-strings: exactly as in input)
	value  string // decoded value (for strings: unescaped content)
	prefix string // whitespace and comments preceding this token
}

// scanner tokenizes JSON input, preserving whitespace and handling comments.
// Supported comment styles: // line, /* block */, # hash line, <!-- html -->.
type scanner struct {
	input []byte
	pos   int
}

func newScanner(input []byte) *scanner {
	return &scanner{input: input}
}

// scan returns all tokens from the input.
func (s *scanner) scan() ([]token, error) {
	tokens := make([]token, 0, 64)
	for {
		tok, err := s.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.typ == tokenEOF {
			break
		}
	}
	return tokens, nil
}

// next returns the next token.
func (s *scanner) next() (token, error) {
	prefix := s.skipWhitespaceAndComments()

	if s.pos >= len(s.input) {
		return token{typ: tokenEOF, prefix: prefix}, nil
	}

	ch := s.input[s.pos]
	switch ch {
	case '{':
		s.pos++
		return token{typ: tokenObjectStart, raw: "{", prefix: prefix}, nil
	case '}':
		s.pos++
		return token{typ: tokenObjectEnd, raw: "}", prefix: prefix}, nil
	case '[':
		s.pos++
		return token{typ: tokenArrayStart, raw: "[", prefix: prefix}, nil
	case ']':
		s.pos++
		return token{typ: tokenArrayEnd, raw: "]", prefix: prefix}, nil
	case ':':
		s.pos++
		return token{typ: tokenColon, raw: ":", prefix: prefix}, nil
	case ',':
		s.pos++
		return token{typ: tokenComma, raw: ",", prefix: prefix}, nil
	case '"':
		return s.scanString(prefix, '"')
	case '\'':
		// JSON5 single-quoted string. Okapi's okf_json accepts these;
		// preserved as a tokenString with single-quote raw form so the
		// writer can round-trip the same quote style.
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
		// JSON5 bare identifier — accepted in object-key position by
		// okapi's okf_json. Emit as tokenString; the parser/writer
		// preserve the raw form so the bare identifier round-trips.
		if isIdentStart(ch) {
			return s.scanBareIdentifier(prefix)
		}
		return token{}, fmt.Errorf("json scanner: unexpected character %q at position %d", ch, s.pos)
	}
}

// matchLiteral returns true when the literal `expected` is present at
// s.pos and the following byte is not an identifier continuation
// character. This disambiguates `true`/`false`/`null` from JSON5 bare
// identifiers that happen to share a prefix (`foo` vs `false`).
func (s *scanner) matchLiteral(expected string) bool {
	end := s.pos + len(expected)
	if end > len(s.input) {
		return false
	}
	if string(s.input[s.pos:end]) != expected {
		return false
	}
	if end < len(s.input) && isIdentCont(s.input[end]) {
		return false
	}
	return true
}

// isIdentStart reports whether b can begin a JSON5 bare identifier.
// Mirrors JS identifier rules conservatively (ASCII letters, _, $).
func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || b == '$'
}

// isIdentCont reports whether b can continue a JSON5 bare identifier.
func isIdentCont(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

// scanBareIdentifier consumes a JSON5 bare identifier and emits it as
// a tokenString with raw == value (no surrounding quotes). The writer
// preserves the bare form by outputting tok.raw verbatim for keys.
func (s *scanner) scanBareIdentifier(prefix string) (token, error) {
	start := s.pos
	for s.pos < len(s.input) && isIdentCont(s.input[s.pos]) {
		s.pos++
	}
	raw := string(s.input[start:s.pos])
	return token{typ: tokenString, raw: raw, value: raw, prefix: prefix}, nil
}

// skipWhitespaceAndComments consumes whitespace and comment blocks, returning
// the consumed bytes as a string (for skeleton preservation).
//
// Whitespace handling is permissive: ASCII whitespace plus any Unicode
// whitespace rune (e.g. U+00A0 NO-BREAK SPACE, often introduced by
// indentation copy-pasted from word processors). RFC 8259 only allows
// ASCII whitespace, but every real-world JSON parser this reader has
// to interop with — including okapi's okf_json — accepts the broader
// Unicode set, and several of our parity fixtures rely on it.
func (s *scanner) skipWhitespaceAndComments() string {
	start := s.pos
	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		// Fast path: ASCII whitespace.
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			s.pos++
			continue
		}
		// Multi-byte Unicode whitespace (NBSP, etc.) plus U+FEFF
		// (BOM/ZWNBSP) which Unicode doesn't classify as IsSpace but
		// every JSON parser tolerates as a leading marker. Treating it
		// as prefix whitespace keeps the byte in the skeleton so the
		// writer round-trips it verbatim.
		if ch >= 0x80 {
			r, size := utf8.DecodeRune(s.input[s.pos:])
			if r != utf8.RuneError && (unicode.IsSpace(r) || r == 0xFEFF) {
				s.pos += size
				continue
			}
		}
		// // line comment
		if ch == '/' && s.pos+1 < len(s.input) && s.input[s.pos+1] == '/' {
			s.pos += 2
			for s.pos < len(s.input) && s.input[s.pos] != '\n' {
				s.pos++
			}
			continue
		}
		// /* block comment */ (supports nesting)
		if ch == '/' && s.pos+1 < len(s.input) && s.input[s.pos+1] == '*' {
			s.pos += 2
			depth := 1
			for s.pos+1 < len(s.input) && depth > 0 {
				if s.input[s.pos] == '/' && s.input[s.pos+1] == '*' {
					depth++
					s.pos += 2
				} else if s.input[s.pos] == '*' && s.input[s.pos+1] == '/' {
					depth--
					s.pos += 2
				} else {
					s.pos++
				}
			}
			continue
		}
		// # hash line comment
		if ch == '#' {
			s.pos++
			for s.pos < len(s.input) && s.input[s.pos] != '\n' {
				s.pos++
			}
			continue
		}
		// <!-- html comment -->
		if ch == '<' && s.pos+3 < len(s.input) &&
			s.input[s.pos+1] == '!' && s.input[s.pos+2] == '-' && s.input[s.pos+3] == '-' {
			s.pos += 4
			for s.pos+2 < len(s.input) {
				if s.input[s.pos] == '-' && s.input[s.pos+1] == '-' && s.input[s.pos+2] == '>' {
					s.pos += 3
					break
				}
				s.pos++
			}
			continue
		}
		break
	}
	return string(s.input[start:s.pos])
}

// scanString scans a JSON string token, handling all escape sequences.
// The `quote` byte is the opening delimiter ('"' for standard JSON,
// '\” for JSON5 single-quoted strings); the same character closes the
// string. The escape `\<quote>` is always recognised regardless of
// which quote opened the string.
func (s *scanner) scanString(prefix string, quote byte) (token, error) {
	start := s.pos
	s.pos++ // skip opening quote
	var decoded strings.Builder

	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		if ch == quote {
			s.pos++ // skip closing quote
			raw := string(s.input[start:s.pos])
			return token{typ: tokenString, raw: raw, value: decoded.String(), prefix: prefix}, nil
		}
		if ch == '\\' {
			s.pos++
			if s.pos >= len(s.input) {
				return token{}, fmt.Errorf("json scanner: unexpected end of string escape at position %d", s.pos)
			}
			esc := s.input[s.pos]
			switch esc {
			case '"':
				decoded.WriteByte('"')
			case '\'':
				decoded.WriteByte('\'')
			case '\\':
				decoded.WriteByte('\\')
			case '/':
				decoded.WriteByte('/')
			case 'b':
				decoded.WriteByte('\b')
			case 'f':
				decoded.WriteByte('\f')
			case 'n':
				decoded.WriteByte('\n')
			case 'r':
				decoded.WriteByte('\r')
			case 't':
				decoded.WriteByte('\t')
			case 'u':
				r, size, err := s.scanUnicodeEscape()
				if err != nil {
					return token{}, err
				}
				decoded.WriteRune(r)
				s.pos += size - 1 // -1 because loop will increment
			default:
				// Unknown escape — preserve as-is
				decoded.WriteByte('\\')
				decoded.WriteByte(esc)
			}
			s.pos++
			continue
		}
		// Regular character (possibly multi-byte UTF-8)
		r, size := utf8.DecodeRune(s.input[s.pos:])
		decoded.WriteRune(r)
		s.pos += size
	}
	return token{}, fmt.Errorf("json scanner: unterminated string at position %d", start)
}

// scanUnicodeEscape reads a \uXXXX (and optional surrogate pair) escape.
// s.pos points to the first hex digit after \u.
func (s *scanner) scanUnicodeEscape() (rune, int, error) {
	s.pos++ // skip past 'u'
	if s.pos+4 > len(s.input) {
		return 0, 0, fmt.Errorf("json scanner: incomplete unicode escape at position %d", s.pos)
	}
	hex1 := string(s.input[s.pos : s.pos+4])
	r1, err := strconv.ParseUint(hex1, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("json scanner: invalid unicode escape \\u%s at position %d", hex1, s.pos)
	}
	size := 4

	// Check for surrogate pair
	if r1 >= 0xD800 && r1 <= 0xDBFF {
		// High surrogate — expect \uXXXX low surrogate
		if s.pos+4+2+4 <= len(s.input) && s.input[s.pos+4] == '\\' && s.input[s.pos+5] == 'u' {
			hex2 := string(s.input[s.pos+6 : s.pos+10])
			r2, err := strconv.ParseUint(hex2, 16, 32)
			if err == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
				combined := 0x10000 + (rune(r1)-0xD800)*0x400 + (rune(r2) - 0xDC00)
				return combined, 10, nil // 4 + 2 + 4
			}
		}
	}

	return rune(r1), size, nil
}

// scanNumber scans a JSON number token.
func (s *scanner) scanNumber(prefix string) (token, error) {
	start := s.pos
	if s.input[s.pos] == '-' {
		s.pos++
	}
	// Integer part
	if s.pos < len(s.input) && s.input[s.pos] == '0' {
		s.pos++
	} else {
		for s.pos < len(s.input) && s.input[s.pos] >= '0' && s.input[s.pos] <= '9' {
			s.pos++
		}
	}
	// Fraction
	if s.pos < len(s.input) && s.input[s.pos] == '.' {
		s.pos++
		for s.pos < len(s.input) && s.input[s.pos] >= '0' && s.input[s.pos] <= '9' {
			s.pos++
		}
	}
	// Exponent
	if s.pos < len(s.input) && (s.input[s.pos] == 'e' || s.input[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.input) && (s.input[s.pos] == '+' || s.input[s.pos] == '-') {
			s.pos++
		}
		for s.pos < len(s.input) && s.input[s.pos] >= '0' && s.input[s.pos] <= '9' {
			s.pos++
		}
	}
	raw := string(s.input[start:s.pos])
	return token{typ: tokenNumber, raw: raw, value: raw, prefix: prefix}, nil
}

// scanLiteral scans a JSON keyword (true, false, null).
func (s *scanner) scanLiteral(expected string, typ tokenType, prefix string) (token, error) {
	if s.pos+len(expected) > len(s.input) || string(s.input[s.pos:s.pos+len(expected)]) != expected {
		return token{}, fmt.Errorf("json scanner: expected %q at position %d", expected, s.pos)
	}
	s.pos += len(expected)
	return token{typ: typ, raw: expected, value: expected, prefix: prefix}, nil
}

// escapeJSONString escapes a string for JSON output using double quotes.
// If escapeSlashes is true, forward slashes are escaped as \/.
func escapeJSONString(s string, escapeSlashes bool) string {
	return escapeJSONStringQuoted(s, escapeSlashes, '"')
}

// escapeJSONStringQuoted escapes a string for JSON output using the
// given delimiter. Use '\” for JSON5 single-quoted output so that
// translatable values inside a single-quoted source like
// `'foo': 'bar'` round-trip with their original quote style. Inside a
// single-quoted string the literal `"` doesn't need escaping (and
// vice-versa), so the escape logic depends on the delimiter.
func escapeJSONStringQuoted(s string, escapeSlashes bool, quote byte) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '"':
			if quote == '"' {
				b.WriteString(`\"`)
			} else {
				b.WriteByte('"')
			}
		case '\'':
			if quote == '\'' {
				b.WriteString(`\'`)
			} else {
				b.WriteByte('\'')
			}
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '/':
			if escapeSlashes {
				b.WriteString(`\/`)
			} else {
				b.WriteByte('/')
			}
		default:
			if r < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// quoteOf inspects a token's raw form and returns the delimiter byte
// used to wrap it (or 0 for bare identifiers). Used by the writer to
// preserve JSON5 single-quoted strings on round-trip.
func quoteOf(raw string) byte {
	if len(raw) == 0 {
		return 0
	}
	switch raw[0] {
	case '"', '\'':
		return raw[0]
	}
	return 0
}
