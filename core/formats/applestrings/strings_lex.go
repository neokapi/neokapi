package applestrings

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file provides a lossless lexer + parser for the legacy Apple .strings
// text format (NeXTSTEP-style "old plist" property lists used for string
// tables). It preserves every byte of the source: the parsed model records the
// exact source spans (offsets) of each entry's key and value strings, so the
// writer can splice a changed translation into the value while copying every
// other byte (comments, whitespace, escaping, BOM, line endings) verbatim.
//
// Grammar (per entry):
//
//	[comment]* "key" = "value" ;
//
// where:
//   - comments are /* ... */ (C block) or // ... (line) sequences
//   - "key" and "value" are double-quoted strings with C-style escapes
//     (\", \n, \t, \\, \Uxxxx, etc.)
//   - the key may also be a bareword in some hand-written files; we accept
//     bareword keys but the common Xcode-generated form is always quoted.
//
// The format is UTF-8 here. UTF-16 inputs are transcoded to UTF-8 before
// lexing (see strings_reader.go); the recorded HasBOM / UTF-16 flag lets the
// writer reproduce the original encoding faithfully.

// stringsEntry is one parsed "key" = "value"; entry with its source spans.
type stringsEntry struct {
	key   string // decoded key text
	value string // decoded value text

	// Raw source spans (byte offsets into the lexer input). The value span
	// covers the bytes strictly inside the value's surrounding quotes (the
	// content the writer rewrites). For barewords it covers the whole token.
	valStart, valEnd int

	comment    string // decoded comment text (preceding the entry), "" if none
	hasComment bool
}

// stringsDoc is the lossless parse of a whole .strings file.
type stringsDoc struct {
	entries []stringsEntry

	// orphanComments holds comment text that is not attached to any entry: a
	// trailing/orphan comment at EOF with no following entry, and the earlier of
	// two stacked comments before a single entry (last-wins supersedes it). These
	// bytes still round-trip verbatim in the skeleton; the reader surfaces the
	// text as layer-scoped NoteAnnotations so the developer note stays reachable
	// as structured metadata instead of being silently dropped.
	orphanComments []string
}

// parseStringsFile lexes and parses the .strings source losslessly.
func parseStringsFile(input string) (*stringsDoc, error) {
	l := &stringsLexer{input: input}
	doc := &stringsDoc{}

	var pendingComment string
	havePending := false

	for {
		l.skipWS()
		if l.eof() {
			// A trailing/orphan comment with no following entry to own it is not
			// dropped: its text is recorded so the reader can surface it as a
			// layer-scoped note (the bytes still round-trip in the skeleton).
			if havePending {
				doc.orphanComments = append(doc.orphanComments, pendingComment)
			}
			break
		}
		// Comments accumulate as the note for the next entry. Only the
		// comment immediately preceding the entry is exposed; if multiple
		// comments stack, the last one wins (matching Xcode/genstrings, which
		// emits a single /* */ note per key). A superseded earlier comment is
		// captured in orphanComments rather than silently lost.
		if l.peekHas("/*") {
			text, err := l.scanBlockComment()
			if err != nil {
				return nil, err
			}
			if havePending {
				doc.orphanComments = append(doc.orphanComments, pendingComment)
			}
			pendingComment = text
			havePending = true
			continue
		}
		if l.peekHas("//") {
			if havePending {
				doc.orphanComments = append(doc.orphanComments, pendingComment)
			}
			pendingComment = l.scanLineComment()
			havePending = true
			continue
		}

		// Parse: key = value ;
		var e stringsEntry
		key, _, err := l.scanStringOrBareword()
		if err != nil {
			return nil, err
		}
		e.key = key
		if havePending {
			e.comment = pendingComment
			e.hasComment = true
			havePending = false
			pendingComment = ""
		}

		l.skipWS()
		if !l.consume('=') {
			return nil, fmt.Errorf("applestrings: expected '=' after key %q at offset %d", key, l.pos)
		}
		l.skipWS()

		valStart := l.pos
		val, vquoted, err := l.scanStringOrBareword()
		if err != nil {
			return nil, err
		}
		valEnd := l.pos
		e.value = val
		// Record the inner span (between the surrounding quotes) for quoted
		// values so the writer rewrites only the content. For barewords the
		// span covers the whole token.
		if vquoted {
			e.valStart = valStart + 1
			e.valEnd = valEnd - 1
		} else {
			e.valStart = valStart
			e.valEnd = valEnd
		}

		l.skipWS()
		if !l.consume(';') {
			return nil, fmt.Errorf("applestrings: expected ';' after value for key %q at offset %d", key, l.pos)
		}

		doc.entries = append(doc.entries, e)
	}

	return doc, nil
}

// stringsLexer is a minimal byte-offset lexer over the .strings source.
type stringsLexer struct {
	input string
	pos   int
}

func (l *stringsLexer) eof() bool { return l.pos >= len(l.input) }

func (l *stringsLexer) peekHas(prefix string) bool {
	return strings.HasPrefix(l.input[l.pos:], prefix)
}

func (l *stringsLexer) consume(b byte) bool {
	if l.pos < len(l.input) && l.input[l.pos] == b {
		l.pos++
		return true
	}
	return false
}

// skipWS advances over whitespace and an optional leading UTF-8 BOM (kept by
// the caller's offset bookkeeping; the writer copies it verbatim).
func (l *stringsLexer) skipWS() {
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == '\v' {
			l.pos++
			continue
		}
		if c == 0xEF && l.pos+2 < len(l.input) && l.input[l.pos+1] == 0xBB && l.input[l.pos+2] == 0xBF {
			l.pos += 3
			continue
		}
		break
	}
}

func (l *stringsLexer) scanBlockComment() (string, error) {
	start := l.pos
	end := strings.Index(l.input[l.pos:], "*/")
	if end < 0 {
		return "", fmt.Errorf("applestrings: unterminated block comment at offset %d", start)
	}
	inner := l.input[l.pos+2 : l.pos+end]
	l.pos += end + 2
	return strings.TrimSpace(inner), nil
}

func (l *stringsLexer) scanLineComment() string {
	start := l.pos + 2
	idx := strings.IndexByte(l.input[start:], '\n')
	if idx < 0 {
		text := strings.TrimSpace(l.input[start:])
		l.pos = len(l.input)
		return text
	}
	text := strings.TrimSpace(l.input[start : start+idx])
	l.pos = start + idx
	return text
}

// scanStringOrBareword scans either a double-quoted string (with C escapes) or
// a bareword token. Returns the decoded text and whether it was quoted.
func (l *stringsLexer) scanStringOrBareword() (string, bool, error) {
	if l.pos >= len(l.input) {
		return "", false, fmt.Errorf("applestrings: unexpected end of input at offset %d", l.pos)
	}
	if l.input[l.pos] == '"' {
		s, err := l.scanQuotedString()
		return s, true, err
	}
	return l.scanBareword(), false, nil
}

// scanBareword scans an unquoted token (letters, digits, '_', '.', '/', '-').
// Hand-written .strings files occasionally use barewords for keys.
func (l *stringsLexer) scanBareword() string {
	start := l.pos
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == '=' || c == ';' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			break
		}
		l.pos++
	}
	return l.input[start:l.pos]
}

// scanQuotedString scans a double-quoted string starting at the opening quote
// and returns the decoded content. The lexer position is advanced past the
// closing quote.
func (l *stringsLexer) scanQuotedString() (string, error) {
	start := l.pos
	l.pos++ // opening quote
	var b strings.Builder
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == '"' {
			l.pos++ // closing quote
			return b.String(), nil
		}
		if c == '\\' {
			l.pos++
			if l.pos >= len(l.input) {
				return "", fmt.Errorf("applestrings: unterminated escape at offset %d", l.pos)
			}
			esc := l.input[l.pos]
			switch esc {
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case '/':
				b.WriteByte('/')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'a':
				b.WriteByte('\a')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'v':
				b.WriteByte('\v')
			case '0':
				b.WriteByte(0)
			case 'U', 'u':
				r, n, err := l.scanUnicodeEscape()
				if err != nil {
					return "", err
				}
				b.WriteRune(r)
				l.pos += n - 1
			default:
				// Unknown escape — keep the backslash and the char verbatim
				// (decoded form) so re-encoding reproduces it.
				b.WriteByte('\\')
				b.WriteByte(esc)
			}
			l.pos++
			continue
		}
		r, size := utf8.DecodeRuneInString(l.input[l.pos:])
		b.WriteRune(r)
		l.pos += size
	}
	return "", fmt.Errorf("applestrings: unterminated string at offset %d", start)
}

// scanUnicodeEscape parses a \Uxxxx (4 hex digits) escape (the position is on
// the 'U'/'u'). Returns the rune and the number of bytes consumed including the
// 'U' (so caller advances accordingly). Apple .strings use \U with exactly 4
// hex digits.
func (l *stringsLexer) scanUnicodeEscape() (rune, int, error) {
	if l.pos+5 > len(l.input) {
		return 0, 0, fmt.Errorf("applestrings: incomplete unicode escape at offset %d", l.pos)
	}
	hex := l.input[l.pos+1 : l.pos+5]
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("applestrings: invalid unicode escape \\U%s", hex)
	}
	return rune(v), 5, nil
}

// encodeStringsValue escapes a decoded value back into the .strings quoted
// content form. Only the characters Apple's plist writer escapes are escaped:
// backslash, double quote, and the control whitespace \n, \r, \t. Other
// characters (including non-ASCII) are emitted as UTF-8 verbatim — modern
// Xcode .strings are UTF-8 and do not \U-escape printable Unicode.
func encodeStringsValue(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
