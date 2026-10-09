package jsonscan

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// StreamScanner is the bounded-memory twin of Scanner: it produces the
// identical token sequence (Type/Raw/Value/Prefix) but reads incrementally from
// an io.Reader through a bufio window instead of indexing a whole-document byte
// slice. Each token owns its bytes, so a streaming walk can emit skeleton and
// build blocks without materialising the document.
type StreamScanner struct {
	prefix string
	r      *bufio.Reader
	buf    []byte // scratch reused across tokens
}

// NewStream returns a StreamScanner over r. Errors are prefixed with prefix
// followed by a colon, for example "arb scanner".
func NewStream(r *bufio.Reader, prefix string) *StreamScanner {
	return &StreamScanner{prefix: prefix, r: r}
}

// Next returns the next token.
func (s *StreamScanner) Next() (Token, error) {
	prefix, err := s.skipWhitespace()
	if err != nil {
		return Token{}, err
	}
	b, err := s.r.ReadByte()
	if errors.Is(err, io.EOF) {
		return Token{Type: EOF, Prefix: prefix}, nil
	}
	if err != nil {
		return Token{}, err
	}
	switch b {
	case '{':
		return Token{Type: ObjectStart, Raw: "{", Prefix: prefix}, nil
	case '}':
		return Token{Type: ObjectEnd, Raw: "}", Prefix: prefix}, nil
	case '[':
		return Token{Type: ArrayStart, Raw: "[", Prefix: prefix}, nil
	case ']':
		return Token{Type: ArrayEnd, Raw: "]", Prefix: prefix}, nil
	case ':':
		return Token{Type: Colon, Raw: ":", Prefix: prefix}, nil
	case ',':
		return Token{Type: Comma, Raw: ",", Prefix: prefix}, nil
	case '"':
		return s.scanString(prefix)
	case 't':
		return s.scanLiteral(b, "true", True, prefix)
	case 'f':
		return s.scanLiteral(b, "false", False, prefix)
	case 'n':
		return s.scanLiteral(b, "null", Null, prefix)
	default:
		if b == '-' || (b >= '0' && b <= '9') {
			return s.scanNumber(b, prefix)
		}
		return Token{}, fmt.Errorf("%s: unexpected character %q", s.prefix, b)
	}
}

// skipWhitespace consumes leading whitespace (and a UTF-8 BOM) into the prefix.
func (s *StreamScanner) skipWhitespace() (string, error) {
	s.buf = s.buf[:0]
	for {
		b, err := s.r.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			s.buf = append(s.buf, b)
			continue
		}
		if b == 0xEF {
			// Possible UTF-8 BOM: peek the next two bytes.
			if pk, _ := s.r.Peek(2); len(pk) == 2 && pk[0] == 0xBB && pk[1] == 0xBF {
				_, _ = s.r.Discard(2)
				s.buf = append(s.buf, 0xEF, 0xBB, 0xBF)
				continue
			}
		}
		_ = s.r.UnreadByte()
		break
	}
	return string(s.buf), nil
}

func (s *StreamScanner) scanString(prefix string) (Token, error) {
	raw := []byte{'"'}
	var decoded strings.Builder
	for {
		b, err := s.r.ReadByte()
		if err != nil {
			return Token{}, errors.New(s.prefix + ": unterminated string")
		}
		raw = append(raw, b)
		if b == '"' {
			return Token{Type: String, Raw: string(raw), Value: decoded.String(), Prefix: prefix}, nil
		}
		if b == '\\' {
			esc, err := s.r.ReadByte()
			if err != nil {
				return Token{}, errors.New(s.prefix + ": unterminated escape")
			}
			raw = append(raw, esc)
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
				r, hexRaw, err := s.scanUnicodeEscape()
				if err != nil {
					return Token{}, err
				}
				decoded.WriteRune(r)
				raw = append(raw, hexRaw...)
			default:
				decoded.WriteByte('\\')
				decoded.WriteByte(esc)
			}
			continue
		}
		// Multi-byte UTF-8: read continuation bytes into raw+decoded.
		if b < 0x80 {
			decoded.WriteByte(b)
			continue
		}
		n := utf8ContinuationCount(b)
		mb := []byte{b}
		for range n {
			cb, err := s.r.ReadByte()
			if err != nil {
				return Token{}, errors.New(s.prefix + ": truncated UTF-8")
			}
			raw = append(raw, cb)
			mb = append(mb, cb)
		}
		r, _ := utf8.DecodeRune(mb)
		decoded.WriteRune(r)
	}
}

// scanUnicodeEscape reads the hex digits after a \u (already consumed), handling
// a surrogate pair. It returns the rune and the raw source bytes consumed after
// the 'u' (for the token's Raw field).
func (s *StreamScanner) scanUnicodeEscape() (rune, []byte, error) {
	hex := make([]byte, 4)
	if _, err := io.ReadFull(s.r, hex); err != nil {
		return 0, nil, errors.New(s.prefix + ": incomplete unicode escape")
	}
	r1, err := strconv.ParseUint(string(hex), 16, 32)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: invalid unicode escape \\u%s", s.prefix, hex)
	}
	raw := append([]byte(nil), hex...)
	if r1 >= 0xD800 && r1 <= 0xDBFF {
		if pk, _ := s.r.Peek(6); len(pk) == 6 && pk[0] == '\\' && pk[1] == 'u' {
			r2, err := strconv.ParseUint(string(pk[2:6]), 16, 32)
			if err == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
				_, _ = s.r.Discard(6)
				raw = append(raw, pk...)
				combined := 0x10000 + (rune(r1)-0xD800)*0x400 + (rune(r2) - 0xDC00)
				return combined, raw, nil
			}
		}
	}
	return rune(r1), raw, nil
}

func (s *StreamScanner) scanNumber(first byte, prefix string) (Token, error) {
	raw := []byte{first}
	for {
		b, err := s.r.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Token{}, err
		}
		if (b >= '0' && b <= '9') || b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-' {
			raw = append(raw, b)
			continue
		}
		_ = s.r.UnreadByte()
		break
	}
	return Token{Type: Number, Raw: string(raw), Value: string(raw), Prefix: prefix}, nil
}

func (s *StreamScanner) scanLiteral(first byte, expected string, typ TokenType, prefix string) (Token, error) {
	rest := make([]byte, len(expected)-1)
	if _, err := io.ReadFull(s.r, rest); err != nil {
		return Token{}, fmt.Errorf("%s: expected %q", s.prefix, expected)
	}
	if string(first)+string(rest) != expected {
		return Token{}, fmt.Errorf("%s: expected %q", s.prefix, expected)
	}
	return Token{Type: typ, Raw: expected, Value: expected, Prefix: prefix}, nil
}

// utf8ContinuationCount returns how many continuation bytes follow a UTF-8 lead
// byte (0 for invalid/ASCII).
func utf8ContinuationCount(b byte) int {
	switch {
	case b&0xE0 == 0xC0:
		return 1
	case b&0xF0 == 0xE0:
		return 2
	case b&0xF8 == 0xF0:
		return 3
	default:
		return 0
	}
}
