package json

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
)

// An edited string value is written as a patch of the bytes it was read from.
// Each character the edit kept is copied as the document spelled it, so a \u
// escape, an escaped slash or a literal character stays as it was, and only
// the characters the edit added are encoded. Those follow the document's own
// spelling: valueSpelling counts how the whole document's strings spell
// non-ASCII characters and slashes, and an added character is spelled the way
// most of them are.

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


// observeText counts the spellings of the strings in skeleton text: the keys
// and the values the reader did not extract. A comment is skipped, so a quote
// or a slash in one counts for nothing. A string the text does not close is
// not counted.
func (s *valueSpelling) observeText(data []byte) {
	for i := 0; i < len(data); i++ {
		c := data[i]
		switch {
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			end := strings.Index(string(data[i+2:]), "*/")
			if end < 0 {
				return
			}
			i += end + 3
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(data) && data[j] != c {
				if data[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(data) {
				return
			}
			s.observe(data[i : j+1])
			i = j
		}
	}
}

// valuePlan is an edited value aligned with the bytes it was read from.
type valuePlan struct {
	quote  byte
	units  []valueUnit
	edited []rune
	match  []int
}

// planValue aligns text with raw, the quoted bytes the value was read from.
// It reports false when raw cannot be split into its characters.
func planValue(raw []byte, text string) (valuePlan, bool) {
	quote, units, ok := splitValue(raw)
	if !ok {
		return valuePlan{}, false
	}
	edited := []rune(text)
	match := format.MatchEdit(len(units), len(edited), func(i, j int) bool { return units[i].r == edited[j] })
	return valuePlan{quote: quote, units: units, edited: edited, match: match}, true
}

// waits reports whether the edit adds a character the document can spell two
// ways, a slash or a non-ASCII character, whose spelling waits on the counts
// of the whole document.
func (p valuePlan) waits() bool {
	added := make([]bool, len(p.edited))
	for i := range added {
		added[i] = true
	}
	for _, j := range p.match {
		if j >= 0 {
			added[j] = false
		}
	}
	for j, r := range p.edited {
		if added[j] && (r == '/' || r >= utf8.RuneSelf) {
			return true
		}
	}
	return false
}

// render writes the value: every character the edit kept is copied as the
// document spells it, and every character it adds is spelled by encodeRune.
func (p valuePlan) render(spelling valueSpelling, escapeSlashes bool) []byte {
	var b strings.Builder
	b.Grow(len(p.units) + len(p.edited)/4 + 2)
	b.WriteByte(p.quote)
	j := 0
	for i, u := range p.units {
		if p.match[i] < 0 {
			continue
		}
		for ; j < p.match[i]; j++ {
			b.WriteString(spelling.encodeRune(p.edited[j], p.quote, escapeSlashes))
		}
		b.WriteString(u.raw)
		j++
	}
	for ; j < len(p.edited); j++ {
		b.WriteString(spelling.encodeRune(p.edited[j], p.quote, escapeSlashes))
	}
	b.WriteByte(p.quote)
	return []byte(b.String())
}

// valueWriter writes a document from its skeleton entries and counts its
// spelling on the way. A value whose added characters wait on the counts of
// the whole document is held, with every byte written after it, until the
// last entry is counted (finish). A document no such edit touches is written
// as its entries arrive.
type valueWriter struct {
	out           io.Writer
	escapeSlashes bool
	spelling      valueSpelling
	holding       bool
	held          []heldPart
}

// heldPart is output held until the document's spelling is known: bytes, or a
// value still to render.
type heldPart struct {
	data []byte
	plan *valuePlan
}

// text writes skeleton text and counts the strings it holds.
func (v *valueWriter) text(data []byte) error {
	v.spelling.observeText(data)
	return v.write(data)
}

// original counts the spelling of a value's bytes as read.
func (v *valueWriter) original(raw []byte) {
	v.spelling.observe(raw)
}

// value writes a string value whose text is text. read is the pair the reader
// recorded for the value: its bytes are replayed while the text is the text
// read, and patched when an edit changed the source. A translation, or a value
// without a pair, is encoded whole with quote as its delimiter.
func (v *valueWriter) value(read valueAsRead, text string, quote byte) error {
	if raw, ok := read.replay(text); ok {
		return v.write(raw)
	}
	if read.ok && !read.target {
		if p, ok := planValue(read.raw, text); ok {
			if p.waits() {
				v.holding = true
				v.held = append(v.held, heldPart{plan: &p})
				return nil
			}
			return v.write(p.render(v.spelling, v.escapeSlashes))
		}
	}
	return v.write([]byte(escapeJSONStringQuoted(text, v.escapeSlashes, quote)))
}

// write writes data, or holds a copy of it behind a held value.
func (v *valueWriter) write(data []byte) error {
	if v.holding {
		v.held = append(v.held, heldPart{data: append([]byte(nil), data...)})
		return nil
	}
	_, err := v.out.Write(data)
	return err
}

// finish renders the held values under the whole document's spelling and
// writes everything held.
func (v *valueWriter) finish() error {
	for _, h := range v.held {
		data := h.data
		if h.plan != nil {
			data = h.plan.render(v.spelling, v.escapeSlashes)
		}
		if _, err := v.out.Write(data); err != nil {
			return err
		}
	}
	v.held, v.holding = nil, false
	return nil
}
