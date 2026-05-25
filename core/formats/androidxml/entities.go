package androidxml

import (
	"strconv"
	"strings"
)

// This file handles the XML entity layer of an Android string resource: the five
// predefined entities and numeric character references inside element character
// data (the same surface as any XML document).
//
// The reader decodes XML entities (so callers see real characters) but
// deliberately KEEPS Android backslash escapes (\' \" \n \t \@ \? \\ \uXXXX)
// verbatim in the Block text. That keeps the surface a translator edits
// identical to what lives in the file (they keep typing \n for a newline, \' for
// an apostrophe) and makes the writer's job a byte-faithful splice rather than a
// re-escape guess. Backslash escapes therefore round-trip untouched.

// decodeEntities resolves the five XML predefined entities and numeric
// character references in element character data. Unknown named references are
// left verbatim (Android resource files do not declare custom entities). This is
// the inverse of encodeText.
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

// encodeText escapes a string for XML element character data. Per the XML 1.0
// spec (§2.4), only '&' and '<' must be escaped in character data; a bare '>'
// is legal and Android resource files routinely carry one (e.g. "Do NOT check
// ->"). To stay byte-faithful with such genuine source content, '>' is left bare
// EXCEPT where it would close a CDATA section ("]]>"), the one place §2.4
// requires escaping it. Double and single quotes are legal bare in element
// content (Android's backslash-escape layer governs their string-literal meaning
// separately) and are left as-is. This runs only when a translation changes a
// value; unchanged values round-trip via their original bytes.
func encodeText(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := range len(s) {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			// Escape '>' only when it terminates a "]]>" sequence.
			if i >= 2 && s[i-1] == ']' && s[i-2] == ']' {
				b.WriteString("&gt;")
			} else {
				b.WriteByte('>')
			}
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// encodeAttr escapes a string for use as a double-quoted XML attribute value.
func encodeAttr(s string) string {
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
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
