package arb

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// streamScanner is the bounded-memory twin of scanner: it produces the identical
// token sequence (typ/raw/value/prefix) but reads incrementally from an
// io.Reader through a bufio window instead of indexing a whole-document byte
// slice. Each token owns its bytes, so a streaming walk can emit skeleton and
// build blocks without materialising the document. Used only by the reader's
// streaming path; the slice scanner still backs the buffered reader and the
// writer.
type streamScanner struct {
	r   *bufio.Reader
	buf []byte // scratch reused across tokens
}

func newStreamScanner(r *bufio.Reader) *streamScanner { return &streamScanner{r: r} }

func (s *streamScanner) next() (token, error) {
	prefix, err := s.skipWhitespace()
	if err != nil {
		return token{}, err
	}
	b, err := s.r.ReadByte()
	if errors.Is(err, io.EOF) {
		return token{typ: tokEOF, prefix: prefix}, nil
	}
	if err != nil {
		return token{}, err
	}
	switch b {
	case '{':
		return token{typ: tokObjectStart, raw: "{", prefix: prefix}, nil
	case '}':
		return token{typ: tokObjectEnd, raw: "}", prefix: prefix}, nil
	case '[':
		return token{typ: tokArrayStart, raw: "[", prefix: prefix}, nil
	case ']':
		return token{typ: tokArrayEnd, raw: "]", prefix: prefix}, nil
	case ':':
		return token{typ: tokColon, raw: ":", prefix: prefix}, nil
	case ',':
		return token{typ: tokComma, raw: ",", prefix: prefix}, nil
	case '"':
		return s.scanString(prefix)
	case 't':
		return s.scanLiteral(b, "true", tokTrue, prefix)
	case 'f':
		return s.scanLiteral(b, "false", tokFalse, prefix)
	case 'n':
		return s.scanLiteral(b, "null", tokNull, prefix)
	default:
		if b == '-' || (b >= '0' && b <= '9') {
			return s.scanNumber(b, prefix)
		}
		return token{}, fmt.Errorf("arb scanner: unexpected character %q", b)
	}
}

// skipWhitespace consumes leading whitespace (and a UTF-8 BOM) into the prefix.
func (s *streamScanner) skipWhitespace() (string, error) {
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
			// Possible UTF-8 BOM — peek the next two bytes.
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

func (s *streamScanner) scanString(prefix string) (token, error) {
	raw := []byte{'"'}
	var decoded strings.Builder
	for {
		b, err := s.r.ReadByte()
		if err != nil {
			return token{}, errors.New("arb scanner: unterminated string")
		}
		raw = append(raw, b)
		if b == '"' {
			return token{typ: tokString, raw: string(raw), value: decoded.String(), prefix: prefix}, nil
		}
		if b == '\\' {
			esc, err := s.r.ReadByte()
			if err != nil {
				return token{}, errors.New("arb scanner: unterminated escape")
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
				r, consumed, hexRaw, err := s.scanUnicodeEscape()
				if err != nil {
					return token{}, err
				}
				decoded.WriteRune(r)
				raw = append(raw, hexRaw...)
				_ = consumed
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
				return token{}, errors.New("arb scanner: truncated UTF-8")
			}
			raw = append(raw, cb)
			mb = append(mb, cb)
		}
		r, _ := utf8.DecodeRune(mb)
		decoded.WriteRune(r)
	}
}

// scanUnicodeEscape reads the hex digits after a \u (already consumed), handling
// a surrogate pair. It returns the rune, the number of source bytes after the
// 'u' consumed, and those raw bytes (for the token's raw field).
func (s *streamScanner) scanUnicodeEscape() (rune, int, []byte, error) {
	hex := make([]byte, 4)
	if _, err := io.ReadFull(s.r, hex); err != nil {
		return 0, 0, nil, errors.New("arb scanner: incomplete unicode escape")
	}
	r1, err := strconv.ParseUint(string(hex), 16, 32)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("arb scanner: invalid unicode escape \\u%s", hex)
	}
	raw := append([]byte(nil), hex...)
	if r1 >= 0xD800 && r1 <= 0xDBFF {
		if pk, _ := s.r.Peek(6); len(pk) == 6 && pk[0] == '\\' && pk[1] == 'u' {
			r2, err := strconv.ParseUint(string(pk[2:6]), 16, 32)
			if err == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
				_, _ = s.r.Discard(6)
				raw = append(raw, pk...)
				combined := 0x10000 + (rune(r1)-0xD800)*0x400 + (rune(r2) - 0xDC00)
				return combined, 10, raw, nil
			}
		}
	}
	return rune(r1), 4, raw, nil
}

func (s *streamScanner) scanNumber(first byte, prefix string) (token, error) {
	raw := []byte{first}
	for {
		b, err := s.r.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return token{}, err
		}
		if (b >= '0' && b <= '9') || b == '.' || b == 'e' || b == 'E' || b == '+' || b == '-' {
			raw = append(raw, b)
			continue
		}
		_ = s.r.UnreadByte()
		break
	}
	return token{typ: tokNumber, raw: string(raw), value: string(raw), prefix: prefix}, nil
}

func (s *streamScanner) scanLiteral(first byte, expected string, typ tokenType, prefix string) (token, error) {
	rest := make([]byte, len(expected)-1)
	if _, err := io.ReadFull(s.r, rest); err != nil {
		return token{}, fmt.Errorf("arb scanner: expected %q", expected)
	}
	if string(first)+string(rest) != expected {
		return token{}, fmt.Errorf("arb scanner: expected %q", expected)
	}
	return token{typ: typ, raw: expected, value: expected, prefix: prefix}, nil
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
