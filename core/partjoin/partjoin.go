// Package partjoin joins the parts of an identity into one string so that no
// two different lists of parts join to the same string.
//
// A plain join such as a + "\x00" + b gives ("a\x00b", "c") and ("a", "b\x00c")
// one result, and a hash of that result gives them one identity. Join escapes
// inside each part instead: the escape character becomes the escape followed
// by '1', and the separator becomes the escape followed by '0'. A part that
// holds neither character is written as it is, so for such parts Join returns
// exactly the bytes of the plain join, and a hash or id built from it does not
// move.
//
// Every identity in the repository that is joined from two or more parts and
// hashed, or kept as an id, goes through this package; scripts/keyguard
// reports one that does not.
package partjoin

import (
	"io"
	"strings"
)

// The NUL separator and its escape. Ordinary text never holds U+0001, so the
// escape changes nothing for real content.
const (
	NULSep    byte = 0x00
	NULEscape byte = 0x01
)

// Join joins parts with sep, escaping sep and esc inside each part.
func Join(sep, esc byte, parts ...string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(sep)
		}
		writeEscaped(&b, p, sep, esc, true)
	}
	return b.String()
}

// JoinTail is Join with the last part escaped for esc alone, so a last part
// that holds sep keeps its bytes. The result stays unambiguous: the parts
// before it hold no bare sep, so the first len(parts)-1 separators end them.
func JoinTail(sep, esc byte, parts ...string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(sep)
		}
		writeEscaped(&b, p, sep, esc, i < len(parts)-1)
	}
	return b.String()
}

// NUL joins parts with NUL, escaping with U+0001. It is the join for a hash
// input.
//
// The number of parts is part of the caller's contract: NUL() and NUL("")
// both return "". Lists of any one length, and lists of different lengths
// above zero, never share a result, so a caller whose list can be empty puts
// a fixed part first.
func NUL(parts ...string) string {
	return Join(NULSep, NULEscape, parts...)
}

// AppendNUL extends a NUL join with more parts: joined must come from NUL or
// AppendNUL, and the result is NUL of all the parts.
func AppendNUL(joined string, parts ...string) string {
	var b strings.Builder
	b.WriteString(joined)
	for _, p := range parts {
		b.WriteByte(NULSep)
		writeEscaped(&b, p, NULSep, NULEscape, true)
	}
	return b.String()
}

// WriteNUL writes each part to w, escaped as NUL escapes it and followed by
// NUL. A hash fed part by part this way reads the bytes NUL(parts...) + "\x00".
func WriteNUL(w io.Writer, parts ...string) {
	var b strings.Builder
	for _, p := range parts {
		writeEscaped(&b, p, NULSep, NULEscape, true)
		b.WriteByte(NULSep)
	}
	_, _ = io.WriteString(w, b.String())
}

// WriteNULJoined writes a string that is already a NUL join, as NUL or
// AppendNUL returned it, to w, followed by NUL. It is not escaped again, so
// the bytes are those of writing its parts with WriteNUL.
func WriteNULJoined(w io.Writer, joined string) {
	_, _ = io.WriteString(w, joined+"\x00")
}

// Escape returns s with sep and esc escaped, as Join writes a part.
func Escape(s string, sep, esc byte) string {
	var b strings.Builder
	writeEscaped(&b, s, sep, esc, true)
	return b.String()
}

func writeEscaped(b *strings.Builder, s string, sep, esc byte, escapeSep bool) {
	if strings.IndexByte(s, esc) < 0 && (!escapeSep || strings.IndexByte(s, sep) < 0) {
		b.WriteString(s)
		return
	}
	for i := range len(s) {
		switch c := s[i]; {
		case c == esc:
			b.WriteByte(esc)
			b.WriteByte('1')
		case c == sep && escapeSep:
			b.WriteByte(esc)
			b.WriteByte('0')
		default:
			b.WriteByte(c)
		}
	}
}
