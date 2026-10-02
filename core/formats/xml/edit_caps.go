package xml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/its"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/xmlesc"
)

// An inline element's start tag is the native data of its code, and the
// reader records on the code the attributes a set_attribute can change, so a
// read shows them. The writer changes the value inside the tag and keeps
// every other byte. It writes the attributes an element spells and adds none,
// because the format has no schema saying which attributes an element may
// carry. Some attributes the tag spells are not attributes of the code:
// a namespace declaration; an attribute the reader surfaces as a block of its
// own (a translatable attribute); and an attribute whose value decides how
// the document reads (xml:lang, xml:space, the ITS attributes, and any
// attribute the reader's configured rules or the document's ITS rules read),
// since a change to it would change what the next read extracts. The
// operations matrix proves the declaration and each refusal
// (core/formats/opsmatrix_capabilities_test.go).
var _ format.AttrWriter = (*Writer)(nil)

// WritableAttrs declares that every attribute an inline element's code
// records is writable, whatever the element.
func (w *Writer) WritableAttrs() map[string][]string {
	return map[string][]string{format.AnyCodeType: {format.AnyAttr}}
}

// WriteAttr sets the value of an attribute the start tag of an inline element
// spells and its code records, escaped for its quotes so an XML parser reads
// back the value given.
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
	_, recorded := r.PcOpen.Attrs[name]
	switch {
	case !found:
		return nil, fmt.Errorf("the element spells no %s attribute, and the writer adds none", name)
	case a.Quote == 0:
		return nil, fmt.Errorf("the %s attribute has no quoted value", name)
	case strings.ContainsRune(data[a.ValueStart:a.ValueEnd], inlineAttrRefOpen):
		return nil, fmt.Errorf("the %s attribute is content of a block of its own; edit that block", name)
	case !recorded:
		return nil, fmt.Errorf("the reader reads the %s attribute as an instruction (an xml: or ITS attribute, or one a rule of the format's configuration or the document's ITS rules names), so a change to it would change what the next read extracts", name)
	}
	if err := xmlesc.CheckText(value); err != nil {
		return nil, err
	}
	open := *r.PcOpen
	open.Data = data[:a.ValueStart] + escapeAttrValue(value, a.Quote) + data[a.ValueEnd:]
	open.Attrs = maps.Clone(open.Attrs)
	open.Attrs[name] = value
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

// inlineCodeAttrs returns the attributes of an inline element's start tag that
// its code records: each attribute data spells with a value, by its name as
// spelled, with the value an XML parser reads (decodeAttrValue; the decoder
// keeps literal line breaks that attribute-value normalization reads as
// spaces). It leaves out a namespace
// declaration, a translatable attribute (its value is a block's marker), and
// an attribute whose value decides how the document reads (interpretsAttr).
// data is the tag as the document spells it and t the tag as the decoder
// read it, with its attributes in the same order. It returns nil when there
// are none, or when data and t do not hold the same attributes.
func (s *xmlParseState) inlineCodeAttrs(t xml.StartElement, data string) map[string]string {
	tag, ok := format.ParseStartTag(data)
	if !ok || len(tag.Attrs) != len(t.Attr) {
		return nil
	}
	var out map[string]string
	for i, a := range tag.Attrs {
		xa := t.Attr[i]
		if !a.HasValue || isNamespaceDecl(a.Name) || strings.ContainsRune(data[a.ValueStart:a.ValueEnd], inlineAttrRefOpen) || s.interpretsAttr(xa.Name) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[a.Name] = decodeAttrValue(data[a.ValueStart:a.ValueEnd])
	}
	return out
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

// itsExtensionsNamespaceURI is the namespace of the ITS extension attributes
// (itsx:), which the reader reads like ITS attributes.
const itsExtensionsNamespaceURI = "http://www.w3.org/2008/12/its-extensions"

// interpretsAttr reports whether the reader reads an attribute's value as an
// instruction about the document rather than carrying it: an attribute in the
// xml, ITS or ITS extensions namespace, one a rule of the reader's
// configuration names, and one the document's ITS rules select, test or point
// at.
func (s *xmlParseState) interpretsAttr(name xml.Name) bool {
	switch name.Space {
	case "xml", its.XMLNamespaceURI, its.NamespaceURI, itsExtensionsNamespaceURI:
		return true
	}
	key := name.Local
	if name.Space != "" {
		key = name.Space + ":" + name.Local
	}
	if s.reader.cfg.namesAttribute(key, name.Local) {
		return true
	}
	return s.itsResolver.MentionsAttribute(name.Local)
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
