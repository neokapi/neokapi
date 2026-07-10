package resx

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// streamTokenizer is the bounded-memory twin of tokenizer: it produces the
// identical token sequence (same kind/raw/name/attrs) but reads incrementally
// from an io.Reader through a bufio window instead of indexing a whole-document
// string. Each token owns its raw bytes, so a streaming walk can emit skeleton
// and discard as it goes without ever materialising the document. It is used
// only by the reader's streaming path; the string tokenizer still backs the
// writer and the buffered reader path (which retains resx.original).
type streamTokenizer struct {
	r   *bufio.Reader
	buf []byte // scratch reused across tokens
}

func newStreamTokenizer(r *bufio.Reader) *streamTokenizer {
	return &streamTokenizer{r: r}
}

// next returns the next token. At end of input it returns tokEOF.
func (t *streamTokenizer) next() (token, error) {
	b, err := t.r.ReadByte()
	if err == io.EOF {
		return token{kind: tokEOF}, nil
	}
	if err != nil {
		return token{}, err
	}
	if b != '<' {
		_ = t.r.UnreadByte()
		return t.scanText()
	}
	// Classify the '<'-led construct by peeking (without consuming) enough
	// bytes to disambiguate `<![CDATA[` (the longest prefix, 9 bytes).
	_ = t.r.UnreadByte()
	head, _ := t.r.Peek(9)
	switch {
	case hasPrefix(head, "<!--"):
		return t.scanDelimited(tokComment, "-->")
	case hasPrefix(head, "<![CDATA["):
		return t.scanDelimited(tokCDATA, "]]>")
	case hasPrefix(head, "<!"):
		return t.scanDoctype()
	case hasPrefix(head, "<?"):
		return t.scanDelimited(tokPI, "?>")
	case hasPrefix(head, "</"):
		return t.scanEndTag()
	default:
		return t.scanStartTag()
	}
}

func hasPrefix(b []byte, s string) bool {
	return len(b) >= len(s) && string(b[:len(s)]) == s
}

// scanText reads character data up to the next '<' (or EOF).
func (t *streamTokenizer) scanText() (token, error) {
	t.buf = t.buf[:0]
	for {
		b, err := t.r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return token{}, err
		}
		if b == '<' {
			_ = t.r.UnreadByte()
			break
		}
		t.buf = append(t.buf, b)
	}
	return token{kind: tokText, raw: string(t.buf)}, nil
}

// scanDelimited reads from the current '<' to the first occurrence of closer
// (inclusive), matching tokenizer.scanDelimited.
func (t *streamTokenizer) scanDelimited(kind tokKind, closer string) (token, error) {
	t.buf = t.buf[:0]
	for {
		b, err := t.r.ReadByte()
		if err == io.EOF {
			return token{}, fmt.Errorf("resx tokenizer: unterminated %q", closer)
		}
		if err != nil {
			return token{}, err
		}
		t.buf = append(t.buf, b)
		if len(t.buf) >= len(closer) && string(t.buf[len(t.buf)-len(closer):]) == closer {
			return token{kind: kind, raw: string(t.buf)}, nil
		}
	}
}

// scanDoctype reads a <!...> declaration, honouring an internal subset so a
// nested '>' does not terminate early (matches tokenizer.scanDoctype).
func (t *streamTokenizer) scanDoctype() (token, error) {
	t.buf = t.buf[:0]
	depth := 0
	for {
		b, err := t.r.ReadByte()
		if err == io.EOF {
			return token{}, errors.New("resx tokenizer: unterminated declaration")
		}
		if err != nil {
			return token{}, err
		}
		t.buf = append(t.buf, b)
		switch b {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '>':
			if depth == 0 {
				return token{kind: tokDoctype, raw: string(t.buf)}, nil
			}
		}
	}
}

// scanEndTag reads a </name> token (matches tokenizer.scanEndTag).
func (t *streamTokenizer) scanEndTag() (token, error) {
	t.buf = t.buf[:0]
	for {
		b, err := t.r.ReadByte()
		if err == io.EOF {
			return token{}, errors.New("resx tokenizer: unterminated end tag")
		}
		if err != nil {
			return token{}, err
		}
		t.buf = append(t.buf, b)
		if b == '>' {
			raw := string(t.buf)
			name := strings.TrimSpace(raw[2 : len(raw)-1]) // between "</" and ">"
			return token{kind: tokEndTag, raw: raw, name: name}, nil
		}
	}
}

// scanStartTag reads a <name ...> or <name ... /> token, skipping quoted
// attribute values that may contain '>' (matches tokenizer.scanStartTag).
func (t *streamTokenizer) scanStartTag() (token, error) {
	t.buf = t.buf[:0]
	var inQuote byte
	for {
		b, err := t.r.ReadByte()
		if err == io.EOF {
			return token{}, errors.New("resx tokenizer: unterminated start tag")
		}
		if err != nil {
			return token{}, err
		}
		t.buf = append(t.buf, b)
		if inQuote != 0 {
			if b == inQuote {
				inQuote = 0
			}
			continue
		}
		switch b {
		case '"', '\'':
			inQuote = b
		case '>':
			raw := string(t.buf)
			kind := tokStartTag
			inner := raw[1 : len(raw)-1] // strip '<' and '>'
			if strings.HasSuffix(inner, "/") {
				kind = tokSelfClose
				inner = inner[:len(inner)-1]
			}
			name, attrs := parseTagInner(inner)
			return token{kind: kind, raw: raw, name: name, attrs: attrs}, nil
		}
	}
}
