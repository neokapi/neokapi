package resx

import (
	"fmt"
	"strings"
)

// This file provides a lossless XML tokenizer used by both the reader and the
// writer. Unlike encoding/xml, it preserves every byte of the source: each
// token carries its exact source bytes in Raw, so concatenating Raw across the
// whole token stream reproduces the input byte-for-byte. This is what makes the
// RESX round-trip faithful (entity encoding, attribute order/quoting,
// whitespace, the resheader boilerplate, and CDATA all survive untouched).
//
// The tokenizer is deliberately minimal — it recognises just enough XML to walk
// a ResX document: the prolog/PI, comments, CDATA, doctype, start tags, end
// tags, self-closing tags, and character data between tags. It does not resolve
// entities or namespaces; the reader handles the small amount of decoding RESX
// needs (resolving the five predefined entities and numeric refs in <value>
// text) itself.

type tokKind int

const (
	tokText      tokKind = iota // character data between tags (may be empty/whitespace)
	tokStartTag                 // <name ...>
	tokEndTag                   // </name>
	tokSelfClose                // <name ... />
	tokComment                  // <!-- ... -->
	tokCDATA                    // <![CDATA[ ... ]]>
	tokPI                       // <?...?> (XML declaration / processing instruction)
	tokDoctype                  // <!DOCTYPE ...>
	tokEOF
)

// token is one lossless lexical unit. Raw is the exact source slice; Name is
// the element local-or-qualified name for tag tokens; Attrs holds parsed
// attributes for start/self-close tokens (used by the reader for @name /
// @type / xml:space). Text tokens carry the raw character data in Raw.
type token struct {
	kind  tokKind
	raw   string
	name  string
	attrs []attr
}

type attr struct {
	name  string
	value string // decoded attribute value (entities resolved)
}

// attrValue returns the decoded value of the named attribute and whether it
// was present.
func (t token) attrValue(name string) (string, bool) {
	for _, a := range t.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
}

type tokenizer struct {
	input string
	pos   int
}

func newTokenizer(input string) *tokenizer { return &tokenizer{input: input} }

// tokenize scans the whole input into a token slice. The concatenation of all
// token Raw fields equals the input exactly.
func (t *tokenizer) tokenize() ([]token, error) {
	var toks []token
	for {
		tok, err := t.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.kind == tokEOF {
			break
		}
	}
	return toks, nil
}

func (t *tokenizer) next() (token, error) {
	if t.pos >= len(t.input) {
		return token{kind: tokEOF}, nil
	}
	if t.input[t.pos] != '<' {
		return t.scanText(), nil
	}
	// Markup constructs beginning with '<'.
	rest := t.input[t.pos:]
	switch {
	case strings.HasPrefix(rest, "<!--"):
		return t.scanDelimited(tokComment, "-->")
	case strings.HasPrefix(rest, "<![CDATA["):
		return t.scanDelimited(tokCDATA, "]]>")
	case strings.HasPrefix(rest, "<!"):
		// DOCTYPE or other declaration; scan to matching '>'.
		return t.scanDoctype()
	case strings.HasPrefix(rest, "<?"):
		return t.scanDelimited(tokPI, "?>")
	case strings.HasPrefix(rest, "</"):
		return t.scanEndTag()
	default:
		return t.scanStartTag()
	}
}

// scanText scans character data up to the next '<' (or end of input).
func (t *tokenizer) scanText() token {
	start := t.pos
	idx := strings.IndexByte(t.input[t.pos:], '<')
	if idx < 0 {
		t.pos = len(t.input)
	} else {
		t.pos += idx
	}
	return token{kind: tokText, raw: t.input[start:t.pos]}
}

// scanDelimited scans a construct from the current '<' to the first occurrence
// of the closing delimiter (inclusive).
func (t *tokenizer) scanDelimited(kind tokKind, closer string) (token, error) {
	start := t.pos
	idx := strings.Index(t.input[t.pos:], closer)
	if idx < 0 {
		return token{}, fmt.Errorf("resx tokenizer: unterminated %q starting at offset %d", closer, start)
	}
	t.pos += idx + len(closer)
	return token{kind: kind, raw: t.input[start:t.pos]}, nil
}

// scanDoctype scans a <!...> declaration honouring an internal subset
// ("[ ... ]") so nested '>' inside it does not terminate prematurely.
func (t *tokenizer) scanDoctype() (token, error) {
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
				return token{kind: tokDoctype, raw: t.input[start:t.pos]}, nil
			}
		}
		i++
	}
	return token{}, fmt.Errorf("resx tokenizer: unterminated declaration starting at offset %d", start)
}

// scanEndTag scans a </name> token.
func (t *tokenizer) scanEndTag() (token, error) {
	start := t.pos
	idx := strings.IndexByte(t.input[t.pos:], '>')
	if idx < 0 {
		return token{}, fmt.Errorf("resx tokenizer: unterminated end tag at offset %d", start)
	}
	t.pos += idx + 1
	raw := t.input[start:t.pos]
	name := strings.TrimSpace(raw[2 : len(raw)-1]) // between "</" and ">"
	return token{kind: tokEndTag, raw: raw, name: name}, nil
}

// scanStartTag scans a <name ...> or <name ... /> token, parsing attributes.
func (t *tokenizer) scanStartTag() (token, error) {
	start := t.pos
	// Find the closing '>' while skipping quoted attribute values, which may
	// legitimately contain '>'.
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
			kind := tokStartTag
			inner := raw[1 : len(raw)-1] // strip '<' and '>'
			if strings.HasSuffix(inner, "/") {
				kind = tokSelfClose
				inner = inner[:len(inner)-1]
			}
			name, attrs := parseTagInner(inner)
			return token{kind: kind, raw: raw, name: name, attrs: attrs}, nil
		}
		i++
	}
	return token{}, fmt.Errorf("resx tokenizer: unterminated start tag at offset %d", start)
}

// parseTagInner splits a start-tag interior (the text between '<' and '>',
// minus any trailing '/') into the element name and its attributes.
func parseTagInner(inner string) (string, []attr) {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return "", nil
	}
	// Element name runs up to the first whitespace.
	name := inner
	rest := ""
	if idx := strings.IndexAny(inner, " \t\r\n"); idx >= 0 {
		name = inner[:idx]
		rest = inner[idx:]
	}
	return name, parseAttrs(rest)
}

// parseAttrs parses an attribute list of the form ` k="v" k2='v2'`.
func parseAttrs(s string) []attr {
	var attrs []attr
	i := 0
	for i < len(s) {
		// Skip whitespace.
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		// Attribute name up to '=' or whitespace.
		nameStart := i
		for i < len(s) && s[i] != '=' && !isSpace(s[i]) {
			i++
		}
		aname := s[nameStart:i]
		// Skip whitespace before '='.
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			// Valueless attribute (rare in RESX) — record empty value.
			if aname != "" {
				attrs = append(attrs, attr{name: aname})
			}
			continue
		}
		i++ // consume '='
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		quote := s[i]
		if quote != '"' && quote != '\'' {
			continue
		}
		i++
		valStart := i
		for i < len(s) && s[i] != quote {
			i++
		}
		raw := s[valStart:i]
		if i < len(s) {
			i++ // consume closing quote
		}
		attrs = append(attrs, attr{name: aname, value: decodeEntities(raw)})
	}
	return attrs
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
