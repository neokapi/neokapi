package format

import "strings"

// StartTag is a markup start tag (HTML or XML) located in the bytes that spell
// it, so a writer can change one attribute value and keep every other byte.
type StartTag struct {
	// Name is the element name as spelled.
	Name string
	// Attrs are the attributes in document order.
	Attrs []TagAttr
	// InsertAt is the offset just after the last attribute (or the name, when
	// there is none), where a new attribute is written.
	InsertAt int
	// End is the offset just past the tag's closing '>'.
	End int
}

// TagAttr is one attribute of a StartTag.
type TagAttr struct {
	// Name is the attribute name as spelled.
	Name string
	// ValueStart and ValueEnd are the byte range of the value, inside its
	// quotes. An attribute written without a value has both at the end of
	// its name, and HasValue false.
	ValueStart, ValueEnd int
	// Quote is the quote around the value, '"' or '\'', or 0 when the value
	// is unquoted or absent.
	Quote    byte
	HasValue bool
}

// Attr returns the attribute named name, compared as fold says, and whether
// the tag holds it.
func (t StartTag) Attr(name string, fold bool) (TagAttr, bool) {
	for _, a := range t.Attrs {
		if a.Name == name || (fold && strings.EqualFold(a.Name, name)) {
			return a, true
		}
	}
	return TagAttr{}, false
}

// ParseStartTag reads the start tag that begins s: '<', a name, attributes
// with double-quoted, single-quoted, unquoted or no values, and '>' or '/>'.
// It reports false when s does not begin with a complete start tag.
func ParseStartTag(s string) (StartTag, bool) {
	if len(s) < 2 || s[0] != '<' || !tagNameStart(s[1]) {
		return StartTag{}, false
	}
	i := 1
	for i < len(s) && !tagSpace(s[i]) && s[i] != '>' && s[i] != '/' {
		i++
	}
	t := StartTag{Name: s[1:i], InsertAt: i}
	for {
		for i < len(s) && tagSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			return StartTag{}, false
		}
		switch {
		case s[i] == '>':
			t.End = i + 1
			return t, true
		case s[i] == '/' && i+1 < len(s) && s[i+1] == '>':
			t.End = i + 2
			return t, true
		case s[i] == '/':
			i++
			continue
		}
		start := i
		for i < len(s) && !tagSpace(s[i]) && s[i] != '>' && s[i] != '=' && !(s[i] == '/' && i+1 < len(s) && s[i+1] == '>') {
			i++
		}
		a := TagAttr{Name: s[start:i], ValueStart: i, ValueEnd: i}
		j := i
		for j < len(s) && tagSpace(s[j]) {
			j++
		}
		if j < len(s) && s[j] == '=' {
			j++
			for j < len(s) && tagSpace(s[j]) {
				j++
			}
			if j >= len(s) {
				return StartTag{}, false
			}
			a.HasValue = true
			switch q := s[j]; q {
			case '"', '\'':
				end := strings.IndexByte(s[j+1:], q)
				if end < 0 {
					return StartTag{}, false
				}
				a.Quote = q
				a.ValueStart, a.ValueEnd = j+1, j+1+end
				i = a.ValueEnd + 1
			default:
				k := j
				for k < len(s) && !tagSpace(s[k]) && s[k] != '>' {
					k++
				}
				a.ValueStart, a.ValueEnd = j, k
				i = k
			}
		}
		t.Attrs = append(t.Attrs, a)
		t.InsertAt = i
	}
}

func tagNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == ':' || c >= 0x80
}

func tagSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}
