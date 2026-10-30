package json

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
)

// An edited string value is written as a patch of the bytes it was read from.
// Each character the edit kept is copied as the document spelled it, so an
// escape (`é`, `\/`) or a literal (`é`, `/`) stays as it was, and only the
// characters the edit added are encoded. Those follow the document's own
// spelling: valueSpelling counts how the values written so far spell non-ASCII
// characters and slashes, and an added character is spelled the way most of
// them are.

// valueSpelling counts how the document's string values spell the characters
// a writer can spell two ways.
type valueSpelling struct {
	// escapedNonASCII and literalNonASCII count non-ASCII characters written
	// as a \u escape and as UTF-8.
	escapedNonASCII, literalNonASCII int
	// upperHex and lowerHex count the hex letters of \u escapes by case.
	upperHex, lowerHex int
	// escapedSlash and literalSlash count slashes written as \/ and as /.
	escapedSlash, literalSlash int
}

// observe counts the spellings in a value's bytes as read, quotes included.
func (s *valueSpelling) observe(raw []byte) {
	_, units, ok := splitValue(raw)
	if !ok {
		return
	}
	for _, u := range units {
		switch {
		case u.raw == `\/`:
			s.escapedSlash++
		case u.raw == "/":
			s.literalSlash++
		case u.r >= utf8.RuneSelf && strings.HasPrefix(u.raw, `\u`):
			s.escapedNonASCII++
			for _, c := range u.raw[2:] {
				switch {
				case c >= 'A' && c <= 'F':
					s.upperHex++
				case c >= 'a' && c <= 'f':
					s.lowerHex++
				}
			}
		case u.r >= utf8.RuneSelf:
			s.literalNonASCII++
		}
	}
}

// encodeRune spells r for a value delimited by quote. A slash is escaped as
// most of the document's slashes are, or as escapeSlashes says when the
// document has none; a non-ASCII character is a \u escape when most of the
// document's non-ASCII characters are.
func (s valueSpelling) encodeRune(r rune, quote byte, escapeSlashes bool) string {
	if r == '/' && s.escapedSlash != s.literalSlash {
		escapeSlashes = s.escapedSlash > s.literalSlash
	}
	if r >= utf8.RuneSelf && s.escapedNonASCII > s.literalNonASCII {
		hex := "%04x"
		if s.upperHex > s.lowerHex {
			hex = "%04X"
		}
		if r > 0xFFFF {
			r -= 0x10000
			return fmt.Sprintf(`\u`+hex+`\u`+hex, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
		}
		return fmt.Sprintf(`\u`+hex, r)
	}
	enc := escapeJSONStringQuoted(string(r), escapeSlashes, quote)
	return enc[1 : len(enc)-1]
}

// valueUnit is one character of a string value: the character and the bytes
// that spell it in the document.
type valueUnit struct {
	r   rune
	raw string
}

// splitValue splits a quoted string value's bytes into its characters. It
// reports false for a value it cannot split one character per unit (an unknown
// escape, which the scanner keeps as two characters, or bytes that are not
// UTF-8), and the caller then encodes the value whole.
func splitValue(raw []byte) (quote byte, units []valueUnit, ok bool) {
	if len(raw) < 2 || (raw[0] != '"' && raw[0] != '\'') || raw[len(raw)-1] != raw[0] {
		return 0, nil, false
	}
	quote = raw[0]
	body := string(raw[1 : len(raw)-1])
	units = make([]valueUnit, 0, len(body))
	for i := 0; i < len(body); {
		if body[i] != '\\' {
			r, n := utf8.DecodeRuneInString(body[i:])
			if r == utf8.RuneError && n <= 1 {
				return 0, nil, false
			}
			units = append(units, valueUnit{r: r, raw: body[i : i+n]})
			i += n
			continue
		}
		if i+1 >= len(body) {
			return 0, nil, false
		}
		var r rune
		n := 2
		switch body[i+1] {
		case '"', '\'', '\\', '/':
			r = rune(body[i+1])
		case 'b':
			r = '\b'
		case 'f':
			r = '\f'
		case 'n':
			r = '\n'
		case 'r':
			r = '\r'
		case 't':
			r = '\t'
		case 'u':
			var size int
			r, size, ok = unicodeEscape(body[i+2:])
			if !ok {
				return 0, nil, false
			}
			n = 2 + size
		default:
			return 0, nil, false
		}
		units = append(units, valueUnit{r: r, raw: body[i : i+n]})
		i += n
	}
	return quote, units, true
}

// unicodeEscape decodes the hex digits after a \u, and the low half of a
// surrogate pair after them, as the scanner does (scanUnicodeEscape). size is
// the number of bytes it read.
func unicodeEscape(s string) (r rune, size int, ok bool) {
	if len(s) < 4 {
		return 0, 0, false
	}
	hi, err := strconv.ParseUint(s[:4], 16, 32)
	if err != nil {
		return 0, 0, false
	}
	if hi >= 0xD800 && hi <= 0xDBFF && len(s) >= 10 && s[4] == '\\' && s[5] == 'u' {
		if lo, err := strconv.ParseUint(s[6:10], 16, 32); err == nil && lo >= 0xDC00 && lo <= 0xDFFF {
			return 0x10000 + (rune(hi)-0xD800)*0x400 + (rune(lo) - 0xDC00), 10, true
		}
	}
	return rune(hi), 4, true
}

// patchValue writes text as a patch of raw, the quoted bytes the value was read
// from: every character text keeps from raw is copied as raw spells it, and
// every character it adds is spelled by encodeRune. It reports false when raw
// cannot be split into its characters.
func patchValue(raw []byte, text string, spelling valueSpelling, escapeSlashes bool) (string, bool) {
	quote, units, ok := splitValue(raw)
	if !ok {
		return "", false
	}
	edited := []rune(text)
	match := format.MatchEdit(len(units), len(edited), func(i, j int) bool { return units[i].r == edited[j] })
	var b strings.Builder
	b.Grow(len(raw) + len(text)/4)
	b.WriteByte(quote)
	j := 0
	for i, u := range units {
		if match[i] < 0 {
			continue
		}
		for ; j < match[i]; j++ {
			b.WriteString(spelling.encodeRune(edited[j], quote, escapeSlashes))
		}
		b.WriteString(u.raw)
		j++
	}
	for ; j < len(edited); j++ {
		b.WriteString(spelling.encodeRune(edited[j], quote, escapeSlashes))
	}
	b.WriteByte(quote)
	return b.String(), true
}

// writeValue returns the bytes for a string value whose text is text. read is
// the pair the reader recorded for the value: its bytes are replayed while the
// text is the text read, and patched when an edit changed the source. A
// translation, or a value without a pair, is encoded whole with quote as its
// delimiter.
func (w *Writer) writeValue(read valueAsRead, text string, quote byte, spelling valueSpelling) []byte {
	if raw, ok := read.replay(text); ok {
		return raw
	}
	if read.ok && !read.target {
		if patched, ok := patchValue(read.raw, text, spelling, w.cfg.EscapeForwardSlashes); ok {
			return []byte(patched)
		}
	}
	return []byte(escapeJSONStringQuoted(text, w.cfg.EscapeForwardSlashes, quote))
}
