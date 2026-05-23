package resx

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// decodeEntities resolves the five XML predefined entities and numeric
// character references in a text run. Unknown named references are left
// verbatim (RESX does not declare custom entities). This is the inverse of
// encodeText for the cases RESX actually exercises.
func decodeEntities(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		semi := strings.IndexByte(s[i:], ';')
		if semi < 0 {
			b.WriteByte('&')
			i++
			continue
		}
		ref := s[i+1 : i+semi]
		switch ref {
		case "amp":
			b.WriteByte('&')
		case "lt":
			b.WriteByte('<')
		case "gt":
			b.WriteByte('>')
		case "quot":
			b.WriteByte('"')
		case "apos":
			b.WriteByte('\'')
		default:
			switch {
			case strings.HasPrefix(ref, "#x") || strings.HasPrefix(ref, "#X"):
				if n, err := strconv.ParseInt(ref[2:], 16, 32); err == nil && n > 0 {
					b.WriteRune(rune(n))
				} else {
					b.WriteString(s[i : i+semi+1])
				}
			case strings.HasPrefix(ref, "#"):
				if n, err := strconv.ParseInt(ref[1:], 10, 32); err == nil && n > 0 {
					b.WriteRune(rune(n))
				} else {
					b.WriteString(s[i : i+semi+1])
				}
			default:
				// Unknown named entity — leave verbatim.
				b.WriteString(s[i : i+semi+1])
			}
		}
		i += semi + 1
	}
	return b.String()
}

// encodeText escapes a string for use as XML element character data, matching
// the .NET ResXResourceWriter conventions: '&', '<', and '>' are entity-encoded
// ('>' is encoded for symmetry with the writer's escaping of ']]>' edge cases
// and to match how .NET serialises text). '"' and ”' are left bare — they are
// legal in element content. This is used only when a translation changes a
// <value>; unchanged values round-trip via their original bytes.
func encodeText(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstRune returns the first rune of s (used for cheap prefix checks).
func firstRune(s string) (rune, bool) {
	if s == "" {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r, true
}
