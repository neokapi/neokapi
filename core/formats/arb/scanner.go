package arb

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file provides a whitespace-preserving JSON tokenizer used by the writer
// to rewrite message value strings inside the original document while leaving
// every other byte untouched. Each token carries its preceding whitespace as a
// prefix so the stream re-serializes byte-for-byte. It mirrors the approach
// used by the native JSON and xcstrings formats.

type tokenType int

const (
	tokObjectStart tokenType = iota // {
	tokObjectEnd                    // }
	tokArrayStart                   // [
	tokArrayEnd                     // ]
	tokColon                        // :
	tokComma                        // ,
	tokString                       // "..."
	tokNumber                       // 123, 1.5
	tokTrue                         // true
	tokFalse                        // false
	tokNull                         // null
	tokEOF
)

type token struct {
	typ    tokenType
	raw    string // exact source bytes of the token (strings keep their quotes/escapes)
	value  string // decoded value for strings
	prefix string // whitespace preceding the token
}

type scanner struct {
	input []byte
	pos   int
}

func newScanner(input []byte) *scanner { return &scanner{input: input} }

func (s *scanner) scan() ([]token, error) {
	tokens := make([]token, 0, 256)
	for {
		tok, err := s.next()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.typ == tokEOF {
			break
		}
	}
	return tokens, nil
}

func (s *scanner) next() (token, error) {
	prefix := s.skipWhitespace()
	if s.pos >= len(s.input) {
		return token{typ: tokEOF, prefix: prefix}, nil
	}
	ch := s.input[s.pos]
	switch ch {
	case '{':
		s.pos++
		return token{typ: tokObjectStart, raw: "{", prefix: prefix}, nil
	case '}':
		s.pos++
		return token{typ: tokObjectEnd, raw: "}", prefix: prefix}, nil
	case '[':
		s.pos++
		return token{typ: tokArrayStart, raw: "[", prefix: prefix}, nil
	case ']':
		s.pos++
		return token{typ: tokArrayEnd, raw: "]", prefix: prefix}, nil
	case ':':
		s.pos++
		return token{typ: tokColon, raw: ":", prefix: prefix}, nil
	case ',':
		s.pos++
		return token{typ: tokComma, raw: ",", prefix: prefix}, nil
	case '"':
		return s.scanString(prefix)
	case 't':
		return s.scanLiteral("true", tokTrue, prefix)
	case 'f':
		return s.scanLiteral("false", tokFalse, prefix)
	case 'n':
		return s.scanLiteral("null", tokNull, prefix)
	default:
		if ch == '-' || (ch >= '0' && ch <= '9') {
			return s.scanNumber(prefix)
		}
		return token{}, fmt.Errorf("arb scanner: unexpected character %q at position %d", ch, s.pos)
	}
}

func (s *scanner) skipWhitespace() string {
	start := s.pos
	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			s.pos++
			continue
		}
		if ch == 0xEF && s.pos+2 < len(s.input) && s.input[s.pos+1] == 0xBB && s.input[s.pos+2] == 0xBF {
			// UTF-8 BOM — keep in prefix so it round-trips.
			s.pos += 3
			continue
		}
		break
	}
	return string(s.input[start:s.pos])
}

func (s *scanner) scanString(prefix string) (token, error) {
	start := s.pos
	s.pos++ // opening quote
	var decoded strings.Builder
	for s.pos < len(s.input) {
		ch := s.input[s.pos]
		if ch == '"' {
			s.pos++
			raw := string(s.input[start:s.pos])
			return token{typ: tokString, raw: raw, value: decoded.String(), prefix: prefix}, nil
		}
		if ch == '\\' {
			s.pos++
			if s.pos >= len(s.input) {
				return token{}, fmt.Errorf("arb scanner: unterminated escape at %d", s.pos)
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
					return token{}, err
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
	return token{}, fmt.Errorf("arb scanner: unterminated string at %d", start)
}

func (s *scanner) scanUnicodeEscape() (rune, int, error) {
	s.pos++ // past 'u'
	if s.pos+4 > len(s.input) {
		return 0, 0, fmt.Errorf("arb scanner: incomplete unicode escape at %d", s.pos)
	}
	hex1 := string(s.input[s.pos : s.pos+4])
	r1, err := strconv.ParseUint(hex1, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("arb scanner: invalid unicode escape \\u%s", hex1)
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

func (s *scanner) scanNumber(prefix string) (token, error) {
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
	return token{typ: tokNumber, raw: raw, value: raw, prefix: prefix}, nil
}

func (s *scanner) scanLiteral(expected string, typ tokenType, prefix string) (token, error) {
	if s.pos+len(expected) > len(s.input) || string(s.input[s.pos:s.pos+len(expected)]) != expected {
		return token{}, fmt.Errorf("arb scanner: expected %q at %d", expected, s.pos)
	}
	s.pos += len(expected)
	return token{typ: typ, raw: expected, value: expected, prefix: prefix}, nil
}
