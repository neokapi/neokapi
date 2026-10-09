// Package jsonscan is a whitespace-preserving JSON tokenizer shared by the
// formats that rewrite a document in place (arb, xcstrings). Each token carries
// its preceding whitespace as a prefix, so the token stream re-serializes
// byte-for-byte and a writer can replace one string value while leaving every
// other byte untouched.
//
// Two scanners produce the same token sequence: Scanner indexes a whole
// document held in memory, and StreamScanner reads incrementally from a
// bufio.Reader for bounded-memory walks. Both take an error prefix that names
// the calling format, so a message reads "arb scanner: ..." rather than
// pointing at this package.
package jsonscan

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// TokenType classifies a token.
type TokenType int

const (
	ObjectStart TokenType = iota // {
	ObjectEnd                    // }
	ArrayStart                   // [
	ArrayEnd                     // ]
	Colon                        // :
	Comma                        // ,
	String                       // "..."
	Number                       // 123, 1.5
	True                         // true
	False                        // false
	Null                         // null
	EOF
)

// Token is one lexical item together with the whitespace that preceded it.
type Token struct {
	Type   TokenType
	Raw    string // exact source bytes of the token (strings keep their quotes/escapes)
	Value  string // decoded value for strings
	Prefix string // whitespace preceding the token
}

// Scanner tokenizes a document held in memory.
type Scanner struct {
	prefix string
	input  []byte
	pos    int
}

// New returns a Scanner over input. Errors are prefixed with prefix followed
// by a colon, for example "arb scanner".
func New(input []byte, prefix string) *Scanner {
	return &Scanner{prefix: prefix, input: input}
}

// Scan tokenizes the whole input. The last token is always EOF, whose Prefix
// holds any trailing whitespace.
func (s *Scanner) Scan() ([]Token, error) {
	tokens := make([]Token, 0, 256)
	for {
		tok, err := s.Next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return tokens, nil
}

// Next returns the next token.
func (s *Scanner) Next() (Token, error) {
	prefix := s.skipWhitespace()
	if s.pos >= len(s.input) {
		return Token{Type: EOF, Prefix: prefix}, nil
	}
	ch := s.input[s.pos]
	switch ch {
	case '{':
		s.pos++
		return Token{Type: ObjectStart, Raw: "{", Prefix: prefix}, nil
	case '}':
		s.pos++
		return Token{Type: ObjectEnd, Raw: "}", Prefix: prefix}, nil
	case '[':
		s.pos++
		return Token{Type: ArrayStart, Raw: "[", Prefix: prefix}, nil
	case ']':
		s.pos++
		return Token{Type: ArrayEnd, Raw: "]", Prefix: prefix}, nil
	case ':':
		s.pos++
		return Token{Type: Colon, Raw: ":", Prefix: prefix}, nil
	case ',':
		s.pos++
		return Token{Type: Comma, Raw: ",", Prefix: prefix}, nil
	case '"':
		return s.scanString(prefix)
	case 't':
		return s.scanLiteral("true", True, prefix)
	case 'f':
		return s.scanLiteral("false", False, prefix)
	case 'n':
		return s.scanLiteral("null", Null, prefix)
	default:
		if ch == '-' || (ch >= '0' && ch <= '9') {
			return s.scanNumber(prefix)
		}
		return Token{}, fmt.Errorf("%s: unexpected character %q at position %d", s.prefix, ch, s.pos)
	}
}

func (s *Scanner) skipWhitespace() string {
	start := s.pos
	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			s.pos++
			continue
		}
		if ch == 0xEF && s.pos+2 < len(s.input) && s.input[s.pos+1] == 0xBB && s.input[s.pos+2] == 0xBF {
			// A UTF-8 BOM stays in the prefix so it round-trips.
			s.pos += 3
			continue
		}
		break
	}
	return string(s.input[start:s.pos])
}

func (s *Scanner) scanString(prefix string) (Token, error) {
	start := s.pos
	s.pos++ // opening quote
	var decoded strings.Builder
	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		if ch == '"' {
			s.pos++
			raw := string(s.input[start:s.pos])
			return Token{Type: String, Raw: raw, Value: decoded.String(), Prefix: prefix}, nil
		}
		if ch == '\\' {
			s.pos++
			if s.pos >= len(s.input) {
				return Token{}, fmt.Errorf("%s: unterminated escape at %d", s.prefix, s.pos)
			}
			esc := s.input[s.pos]
			switch esc {
			case '"':
				decoded.WriteByte('"')
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
					return Token{}, err
				}
				decoded.WriteRune(r)
				s.pos += size - 1
			default:
				decoded.WriteByte('\\')
				decoded.WriteByte(esc)
			}
			s.pos++
			continue
		}
		r, size := utf8.DecodeRune(s.input[s.pos:])
		decoded.WriteRune(r)
		s.pos += size
	}
	return Token{}, fmt.Errorf("%s: unterminated string at %d", s.prefix, start)
}

func (s *Scanner) scanUnicodeEscape() (rune, int, error) {
	s.pos++ // past 'u'
	if s.pos+4 > len(s.input) {
		return 0, 0, fmt.Errorf("%s: incomplete unicode escape at %d", s.prefix, s.pos)
	}
	hex1 := string(s.input[s.pos : s.pos+4])
	r1, err := strconv.ParseUint(hex1, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: invalid unicode escape \\u%s", s.prefix, hex1)
	}
	if r1 >= 0xD800 && r1 <= 0xDBFF {
		if s.pos+10 <= len(s.input) && s.input[s.pos+4] == '\\' && s.input[s.pos+5] == 'u' {
			hex2 := string(s.input[s.pos+6 : s.pos+10])
			r2, err := strconv.ParseUint(hex2, 16, 32)
			if err == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
				combined := 0x10000 + (rune(r1)-0xD800)*0x400 + (rune(r2) - 0xDC00)
				return combined, 10, nil
			}
		}
	}
	return rune(r1), 4, nil
}

func (s *Scanner) scanNumber(prefix string) (Token, error) {
	start := s.pos
	if s.input[s.pos] == '-' {
		s.pos++
	}
	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-' {
			s.pos++
			continue
		}
		break
	}
	raw := string(s.input[start:s.pos])
	return Token{Type: Number, Raw: raw, Value: raw, Prefix: prefix}, nil
}

func (s *Scanner) scanLiteral(expected string, typ TokenType, prefix string) (Token, error) {
	if s.pos+len(expected) > len(s.input) || string(s.input[s.pos:s.pos+len(expected)]) != expected {
		return Token{}, fmt.Errorf("%s: expected %q at %d", s.prefix, expected, s.pos)
	}
	s.pos += len(expected)
	return Token{Type: typ, Raw: expected, Value: expected, Prefix: prefix}, nil
}
