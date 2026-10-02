package xml

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/xmlesc"
)

// An inline element's start tag is the native data of its code, and the
// reader records the tag's attributes on the code, so a read shows them and a
// set_attribute can change one. The writer changes the value inside the tag
// and keeps every other byte. It writes the attributes an element spells and
// adds none, because the format has no schema saying which attributes an
// element may carry. A namespace declaration, and an attribute the reader
// surfaces as a block of its own (a translatable attribute), are not
// attributes of the code. The operations matrix proves the declaration
// (core/formats/opsmatrix_capabilities_test.go).
var _ format.AttrWriter = (*Writer)(nil)

// WritableAttrs declares that every attribute an inline element's start tag
// spells is writable, whatever the element.
func (w *Writer) WritableAttrs() map[string][]string {
	return map[string][]string{format.AnyCodeType: {format.AnyAttr}}
}

// WriteAttr sets the value of an attribute the start tag of an inline element
// spells, escaped for its quotes so an XML parser reads back the value given.
func (w *Writer) WriteAttr(seq []model.Run, at int, name, value string) ([]model.Run, error) {
	if at < 0 || at >= len(seq) {
		return nil, fmt.Errorf("no code at run %d", at)
	}
	r := seq[at]
	if r.PcOpen == nil {
		return nil, errors.New("the code is not an inline element's start tag")
	}
	data := r.PcOpen.Data
	tag, ok := format.ParseStartTag(data)
	if !ok || tag.End != len(data) {
		return nil, errors.New("the code's markup is not a single start tag")
	}
	if isNamespaceDecl(name) {
		return nil, errors.New("a namespace declaration is not an attribute of the element")
	}
	a, found := tag.Attr(name, false)
	switch {
	case !found:
		return nil, fmt.Errorf("the element spells no %s attribute, and the writer adds none", name)
	case a.Quote == 0:
		return nil, fmt.Errorf("the %s attribute has no quoted value", name)
	case strings.ContainsRune(data[a.ValueStart:a.ValueEnd], inlineAttrRefOpen):
		return nil, fmt.Errorf("the %s attribute is content of a block of its own; edit that block", name)
	}
	if err := xmlesc.CheckText(value); err != nil {
		return nil, err
	}
	next := data[:a.ValueStart] + escapeAttrValue(value, a.Quote) + data[a.ValueEnd:]
	open := *r.PcOpen
	open.Data = next
	open.Attrs = inlineTagAttrs(next)
	out := slices.Clone(seq)
	out[at] = model.Run{PcOpen: &open}
	return out, nil
}

// inlineAttrRefOpen opens the marker the reader puts in place of a
// translatable attribute's value (inlineAttrRefMarker).
const inlineAttrRefOpen = '\x01'

func isNamespaceDecl(name string) bool {
	return name == "xmlns" || strings.HasPrefix(name, "xmlns:")
}

// inlineTagAttrs returns the attributes an inline element's start tag spells,
// by name as spelled, with references decoded and whitespace normalized as an
// XML parser reads them. Namespace declarations and translatable attributes
// (read as blocks of their own) are left out. It returns nil for a tag with
// none.
func inlineTagAttrs(data string) map[string]string {
	tag, ok := format.ParseStartTag(data)
	if !ok {
		return nil
	}
	var out map[string]string
	for _, a := range tag.Attrs {
		raw := data[a.ValueStart:a.ValueEnd]
		if !a.HasValue || isNamespaceDecl(a.Name) || strings.ContainsRune(raw, inlineAttrRefOpen) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[a.Name] = decodeAttrValue(raw)
	}
	return out
}

// escapeAttrValue escapes v for an attribute value quoted with quote: '&', '<'
// and the quote as references, and tab, line feed and carriage return as
// character references, which attribute-value normalization would otherwise
// read as spaces.
func escapeAttrValue(v string, quote byte) string {
	var b strings.Builder
	b.Grow(len(v) + 8)
	for i := range len(v) {
		switch c := v[i]; {
		case c == '&':
			b.WriteString("&amp;")
		case c == '<':
			b.WriteString("&lt;")
		case c == '"' && quote == '"':
			b.WriteString("&quot;")
		case c == '\'' && quote == '\'':
			b.WriteString("&apos;")
		case c == '\t':
			b.WriteString("&#9;")
		case c == '\n':
			b.WriteString("&#10;")
		case c == '\r':
			b.WriteString("&#13;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// decodeAttrValue decodes an attribute value as an XML parser reads it:
// literal tab, line feed and carriage return become spaces, then the
// predefined entities and character references are resolved. An unknown
// reference is kept as written.
func decodeAttrValue(raw string) string {
	normalized := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, raw)
	if !strings.Contains(normalized, "&") {
		return normalized
	}
	var b strings.Builder
	for i := 0; i < len(normalized); i++ {
		if normalized[i] != '&' {
			b.WriteByte(normalized[i])
			continue
		}
		end := strings.IndexByte(normalized[i:], ';')
		if end < 0 {
			b.WriteString(normalized[i:])
			break
		}
		ref := normalized[i+1 : i+end]
		if s, ok := predefinedEntities[ref]; ok {
			b.WriteString(s)
		} else if r, ok := charRef(ref); ok {
			b.WriteRune(r)
		} else {
			b.WriteString(normalized[i : i+end+1])
		}
		i += end
	}
	return b.String()
}

var predefinedEntities = map[string]string{"amp": "&", "lt": "<", "gt": ">", "quot": `"`, "apos": "'"}

// charRef resolves a numeric character reference's body (#65, #x41).
func charRef(ref string) (rune, bool) {
	if !strings.HasPrefix(ref, "#") {
		return 0, false
	}
	base, digits := 10, ref[1:]
	if strings.HasPrefix(digits, "x") || strings.HasPrefix(digits, "X") {
		base, digits = 16, digits[1:]
	}
	n, err := strconv.ParseInt(digits, base, 32)
	if err != nil || n < 0 || !xmlesc.ValidChar(rune(n)) {
		return 0, false
	}
	return rune(n), true
}
