package applestrings

import (
	"fmt"
	"strconv"
	"strings"
)

// This file provides a lossless XML tokenizer for the .stringsdict plist-XML
// format. Like the RESX tokenizer it preserves every byte of the source: each
// token carries its exact source slice in raw, so concatenating raw across the
// whole stream reproduces the input byte-for-byte. This is what makes the
// .stringsdict round-trip faithful (the DOCTYPE, attribute quoting, whitespace,
// and entity encoding all survive untouched). The reader walks the token tree
// to model plural values as blocks; the writer rewrites only changed <string>
// element text and copies all other tokens verbatim.
//
// A dedicated plist library is intentionally not used — .stringsdict is XML
// plist (never the binary or NeXTSTEP variant in practice for this purpose),
// and a token-level approach is the only way to guarantee byte-faithful output.

type plistTokKind int

const (
	plTokText      plistTokKind = iota // character data between tags
	plTokStartTag                      // <name ...>
	plTokEndTag                        // </name>
	plTokSelfClose                     // <name ... />
	plTokComment                       // <!-- ... -->
	plTokCDATA                         // <![CDATA[ ... ]]>
	plTokPI                            // <?...?>
	plTokDoctype                       // <!DOCTYPE ...>
	plTokEOF
)

// plistToken is one lossless lexical unit. raw is the exact source slice; name
// is the element local name for tag tokens.
type plistToken struct {
	kind plistTokKind
	raw  string
	name string
}

type plistTokenizer struct {
	input string
	pos   int
}

func newPlistTokenizer(input string) *plistTokenizer { return &plistTokenizer{input: input} }

// tokenize scans the whole input into a token slice. The concatenation of all
// token raw fields equals the input exactly.
func (t *plistTokenizer) tokenize() ([]plistToken, error) {
	var toks []plistToken
	for {
		tok, err := t.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.kind == plTokEOF {
			break
		}
	}
	return toks, nil
}

func (t *plistTokenizer) next() (plistToken, error) {
	if t.pos >= len(t.input) {
		return plistToken{kind: plTokEOF}, nil
	}
	if t.input[t.pos] != '<' {
		return t.scanText(), nil
	}
	rest := t.input[t.pos:]
	switch {
	case strings.HasPrefix(rest, "<!--"):
		return t.scanDelimited(plTokComment, "-->")
	case strings.HasPrefix(rest, "<![CDATA["):
		return t.scanDelimited(plTokCDATA, "]]>")
	case strings.HasPrefix(rest, "<!"):
		return t.scanDoctype()
	case strings.HasPrefix(rest, "<?"):
		return t.scanDelimited(plTokPI, "?>")
	case strings.HasPrefix(rest, "</"):
		return t.scanEndTag()
	default:
		return t.scanStartTag()
	}
}

func (t *plistTokenizer) scanText() plistToken {
	start := t.pos
	idx := strings.IndexByte(t.input[t.pos:], '<')
	if idx < 0 {
		t.pos = len(t.input)
	} else {
		t.pos += idx
	}
	return plistToken{kind: plTokText, raw: t.input[start:t.pos]}
}

func (t *plistTokenizer) scanDelimited(kind plistTokKind, closer string) (plistToken, error) {
	start := t.pos
	idx := strings.Index(t.input[t.pos:], closer)
	if idx < 0 {
		return plistToken{}, fmt.Errorf("applestrings plist: unterminated %q at offset %d", closer, start)
	}
	t.pos += idx + len(closer)
	return plistToken{kind: kind, raw: t.input[start:t.pos]}, nil
}

func (t *plistTokenizer) scanDoctype() (plistToken, error) {
	start := t.pos
	depth := 0
	i := t.pos
	for i < len(t.input) {
		switch t.input[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '>':
			if depth == 0 {
				t.pos = i + 1
				return plistToken{kind: plTokDoctype, raw: t.input[start:t.pos]}, nil
			}
		}
		i++
	}
	return plistToken{}, fmt.Errorf("applestrings plist: unterminated declaration at offset %d", start)
}

func (t *plistTokenizer) scanEndTag() (plistToken, error) {
	start := t.pos
	idx := strings.IndexByte(t.input[t.pos:], '>')
	if idx < 0 {
		return plistToken{}, fmt.Errorf("applestrings plist: unterminated end tag at offset %d", start)
	}
	t.pos += idx + 1
	raw := t.input[start:t.pos]
	name := strings.TrimSpace(raw[2 : len(raw)-1])
	return plistToken{kind: plTokEndTag, raw: raw, name: name}, nil
}

func (t *plistTokenizer) scanStartTag() (plistToken, error) {
	start := t.pos
	i := t.pos + 1
	var inQuote byte
	for i < len(t.input) {
		c := t.input[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			i++
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '>':
			t.pos = i + 1
			raw := t.input[start:t.pos]
			kind := plTokStartTag
			inner := raw[1 : len(raw)-1]
			if strings.HasSuffix(inner, "/") {
				kind = plTokSelfClose
				inner = inner[:len(inner)-1]
			}
			name := plistTagName(inner)
			return plistToken{kind: kind, raw: raw, name: name}, nil
		}
		i++
	}
	return plistToken{}, fmt.Errorf("applestrings plist: unterminated start tag at offset %d", start)
}

// plistTagName extracts the element name from a start-tag interior.
func plistTagName(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	if idx := strings.IndexAny(inner, " \t\r\n"); idx >= 0 {
		return inner[:idx]
	}
	return inner
}

// decodePlistText resolves the five XML predefined entities and numeric
// character references in element character data. Unknown named references are
// left verbatim.
func decodePlistText(s string) string {
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
				b.WriteString(s[i : i+semi+1])
			}
		}
		i += semi + 1
	}
	return b.String()
}

// encodePlistText escapes a decoded string for use as XML element character
// data: '&', '<', and '>' are entity-encoded. '"' and '\” are legal in element
// content and left bare. Used only when a translation changes a <string>;
// unchanged values round-trip via their original bytes.
func encodePlistText(s string) string {
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
