// Raw XML capture shared by the OpenXML readers: reading an element's character
// data, capturing a whole element (or an mc:AlternateContent choice) verbatim,
// and re-serialising start/end elements with their source prefixes.

package openxml

import (
	"encoding/xml"
	"strings"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
)

// writeRawStartElementTo emits an XML start element to a strings.Builder,
// preserving namespace prefixes via the package nsPrefixMap and
// registering any new xmlns declarations on the element.
func writeRawStartElementTo(out *strings.Builder, t xml.StartElement) {
	registerNamespaces(t.Attr)
	out.WriteString("<")
	writeElementName(out, t.Name)
	for _, a := range t.Attr {
		out.WriteString(" ")
		writeAttrName(out, a.Name)
		out.WriteString(`="`)
		out.WriteString(xmlesc.Attr(a.Value))
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// writeRawEndElementTo emits an XML end element to a strings.Builder.
func writeRawEndElementTo(out *strings.Builder, t xml.EndElement) {
	out.WriteString("</")
	writeElementName(out, t.Name)
	out.WriteString(">")
}

func startElementToRaw(start xml.StartElement) string {
	var b strings.Builder
	b.WriteString("<")
	writeElementName(&b, start.Name)
	for _, a := range start.Attr {
		b.WriteString(" ")
		writeAttrName(&b, a.Name)
		b.WriteString(`="`)
		b.WriteString(xmlesc.Attr(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	return b.String()
}

// readCharData reads character data content of a simple element and consumes its end tag.
func readCharData(d *xml.Decoder) (string, error) {
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			// Unexpected nested element — skip it
			if err := skipElement(d); err != nil {
				return "", err
			}
		}
	}
}

// captureRawElement captures an entire element (start to end) as raw XML.
func captureRawElement(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlesc.Attr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			buf.WriteString("<")
			writeElementName(&buf, t.Name)
			for _, a := range t.Attr {
				buf.WriteString(" ")
				writeAttrName(&buf, a.Name)
				buf.WriteString(`="`)
				buf.WriteString(xmlesc.Attr(a.Value))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlesc.Text(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
	return buf.String(), nil
}

// captureAlternateContent serializes an <mc:AlternateContent> element,
// preserving the wrapper plus the selected branch but dropping
// <mc:Fallback>. Per ECMA-376 Part 3 / ISO/IEC 29500-3 §10 (Markup
// Compatibility and Extensibility) the consumer must select the first
// <mc:Choice Requires="..."> whose required namespaces are all
// supported, otherwise the <mc:Fallback>. Okapi's reference filter
// always selects the first Choice and unconditionally strips Fallback
// (SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK at line
// 56 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/SkippableElement.java; gold fixture
// gold/parts/block/document-alternate-content.xml shows
// <mc:AlternateContent><mc:Choice Requires="wps">...</mc:Choice></
// mc:AlternateContent> surviving the round-trip with Fallback gone).
// We mirror that policy: keep the wrapper, keep every Choice, drop
// every Fallback. The wrapper element name (mc:AlternateContent) and
// child Choice/Fallback names are matched by local-name regardless of
// prefix so documents that bind the markup-compatibility namespace to
// a non-default prefix still work.
func captureAlternateContent(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlesc.Attr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Fallback":
				// Drop the Fallback subtree entirely. Skip without
				// emitting anything — matches Okapi's
				// SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK
				// behaviour described above.
				if err := skipElement(d); err != nil {
					return "", err
				}
			case "Choice":
				// Keep the Choice element verbatim, including its
				// Requires attribute and full subtree. Per the MCE
				// spec a Choice consumer MAY select the first
				// supported Choice — Okapi simply preserves every
				// Choice and lets the rendering pipeline decide,
				// which is byte-faithful to the source for any
				// document that already had its wrapper survive a
				// Word save/load round-trip.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return "", err
				}
				buf.WriteString(raw)
			default:
				// Defensive: unknown child of mc:AlternateContent
				// (the schema only allows Choice and Fallback).
				// Preserve it verbatim so unusual documents don't
				// regress silently.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return "", err
				}
				buf.WriteString(raw)
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				buf.WriteString("</")
				writeElementName(&buf, t.Name)
				buf.WriteString(">")
				return buf.String(), nil
			}
			// Should not happen for a well-formed document, but
			// emit the close tag defensively.
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlesc.Text(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
}

// xmlEscapeRune writes a single rune to a string builder, XML-escaping if
// needed. Kept local rather than folded into xmlesc: it streams one rune
// straight into a caller-owned Builder instead of returning a string, so the
// per-rune writers here avoid an intermediate allocation per run.
func xmlEscapeRune(buf *strings.Builder, r rune) {
	switch r {
	case '&':
		buf.WriteString("&amp;")
	case '<':
		buf.WriteString("&lt;")
	case '>':
		buf.WriteString("&gt;")
	case '"':
		buf.WriteString("&quot;")
	default:
		buf.WriteRune(r)
	}
}
