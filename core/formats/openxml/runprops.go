package openxml

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// runProps holds normalized run properties extracted from <w:rPr>.
type runProps struct {
	bold       bool
	italic     bool
	underline  string // "single", "double", etc. — empty means none
	strike     bool
	vertAlign  string // "superscript", "subscript", or ""
	vanish     bool   // hidden text
	fontName   string // primary font name from <w:rFonts> (ascii or hAnsi)
	fontNameCS string // complex script font name
	fontNameEA string // East Asian font name
	otherXML   string // serialized non-formatting properties we preserve but don't compare
}

// equal returns true if two runProps produce the same visual formatting.
// Font names are compared when set (for font mapping merging).
func (rp runProps) equal(other runProps) bool {
	return rp.bold == other.bold &&
		rp.italic == other.italic &&
		rp.underline == other.underline &&
		rp.strike == other.strike &&
		rp.vertAlign == other.vertAlign &&
		rp.vanish == other.vanish &&
		rp.fontName == other.fontName
}

// isEmpty returns true if no formatting properties are set.
func (rp runProps) isEmpty() bool {
	return !rp.bold && !rp.italic && rp.underline == "" &&
		!rp.strike && rp.vertAlign == "" && !rp.vanish
}

// appendOpeningRuns emits PcOpen runs for this run's formatting.
func (rp runProps) appendOpeningRuns(b *runBuilder, idCounter *int) {
	emit := func(typ, subType, data string) {
		*idCounter++
		b.AddPcOpen(idStr(*idCounter), typ, subType, data, "", "", true, true, true)
	}
	if rp.bold {
		emit(TypeBold, SubTypeBold, "<w:b/>")
	}
	if rp.italic {
		emit(TypeItalic, SubTypeItalic, "<w:i/>")
	}
	if rp.underline != "" {
		emit(TypeUnderline, SubTypeUnderline, "<w:u w:val=\""+rp.underline+"\"/>")
	}
	if rp.strike {
		emit(TypeStrikethrough, SubTypeStrikethrough, "<w:strike/>")
	}
	if rp.vertAlign == "superscript" {
		emit(TypeSuperscript, SubTypeSuperscript, "<w:vertAlign w:val=\"superscript\"/>")
	}
	if rp.vertAlign == "subscript" {
		emit(TypeSubscript, SubTypeSubscript, "<w:vertAlign w:val=\"subscript\"/>")
	}
}

// appendClosingRuns emits PcClose runs for this run's formatting in
// reverse order.
func (rp runProps) appendClosingRuns(b *runBuilder, idCounter *int) {
	emit := func(typ, subType, data string) {
		*idCounter++
		b.AddPcClose(idStr(*idCounter), typ, subType, data, "")
	}
	if rp.vertAlign == "subscript" {
		emit(TypeSubscript, SubTypeSubscript, "</w:vertAlign>")
	}
	if rp.vertAlign == "superscript" {
		emit(TypeSuperscript, SubTypeSuperscript, "</w:vertAlign>")
	}
	if rp.strike {
		emit(TypeStrikethrough, SubTypeStrikethrough, "</w:strike>")
	}
	if rp.underline != "" {
		emit(TypeUnderline, SubTypeUnderline, "</w:u>")
	}
	if rp.italic {
		emit(TypeItalic, SubTypeItalic, "</w:i>")
	}
	if rp.bold {
		emit(TypeBold, SubTypeBold, "</w:b>")
	}
}

func idStr(n int) string {
	return fmt.Sprintf("c%d", n)
}

// parseRunProps extracts run properties from a <w:rPr> element.
// The decoder should be positioned just after reading the w:rPr start element.
func parseRunProps(d *xml.Decoder, aggressive bool) (runProps, error) {
	var props runProps
	var otherParts []string

	for {
		tok, err := d.Token()
		if err != nil {
			return props, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			skip := false

			// Skip rsid and proof attributes in aggressive cleanup
			if aggressive {
				if strings.HasPrefix(local, "rsid") ||
					local == "proofErr" ||
					local == "lastRenderedPageBreak" ||
					local == "noProof" {
					skip = true
				}
			}

			switch {
			case skip:
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "b":
				props.bold = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "bCs":
				// Complex script bold — skip
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "i":
				props.italic = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "iCs":
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "u":
				val := attrVal(t, "val")
				if val != "" && val != "none" {
					props.underline = val
				}
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "strike":
				props.strike = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "vertAlign":
				props.vertAlign = attrVal(t, "val")
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "vanish":
				props.vanish = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "rFonts":
				// Capture font names: ascii/hAnsi for Latin, cs for complex script, eastAsia for EA
				if v := attrVal(t, "ascii"); v != "" {
					props.fontName = v
				} else if v := attrVal(t, "hAnsi"); v != "" {
					props.fontName = v
				}
				props.fontNameCS = attrVal(t, "cs")
				props.fontNameEA = attrVal(t, "eastAsia")
				// Still preserve as raw XML for roundtrip
				raw, err := serializeElement(d, t)
				if err != nil {
					return props, err
				}
				otherParts = append(otherParts, raw)
			default:
				// Preserve unknown properties as raw XML
				raw, err := serializeElement(d, t)
				if err != nil {
					return props, err
				}
				otherParts = append(otherParts, raw)
			}

		case xml.EndElement:
			// End of rPr
			if len(otherParts) > 0 {
				slices.Sort(otherParts)
				props.otherXML = strings.Join(otherParts, "")
			}
			return props, nil
		}
	}
}

// skipElement skips past the current element and all its children.
func skipElement(d *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// serializeElement captures an element and its contents as a raw XML string.
func serializeElement(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	if start.Name.Space != "" {
		buf.WriteString(start.Name.Space)
		buf.WriteString(":")
	}
	buf.WriteString(start.Name.Local)
	for _, attr := range start.Attr {
		buf.WriteString(" ")
		if attr.Name.Space != "" {
			buf.WriteString(attr.Name.Space)
			buf.WriteString(":")
		}
		buf.WriteString(attr.Name.Local)
		buf.WriteString("=\"")
		buf.WriteString(attr.Value)
		buf.WriteString("\"")
	}

	// Check if immediately closed
	depth := 1
	var inner strings.Builder
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			inner.WriteString("<")
			if t.Name.Space != "" {
				inner.WriteString(t.Name.Space)
				inner.WriteString(":")
			}
			inner.WriteString(t.Name.Local)
			for _, attr := range t.Attr {
				inner.WriteString(" ")
				if attr.Name.Space != "" {
					inner.WriteString(attr.Name.Space)
					inner.WriteString(":")
				}
				inner.WriteString(attr.Name.Local)
				inner.WriteString("=\"")
				inner.WriteString(attr.Value)
				inner.WriteString("\"")
			}
			inner.WriteString(">")
		case xml.EndElement:
			depth--
			if depth > 0 {
				inner.WriteString("</")
				if t.Name.Space != "" {
					inner.WriteString(t.Name.Space)
					inner.WriteString(":")
				}
				inner.WriteString(t.Name.Local)
				inner.WriteString(">")
			}
		case xml.CharData:
			inner.Write(t)
		}
	}

	content := inner.String()
	if content == "" {
		buf.WriteString("/>")
	} else {
		buf.WriteString(">")
		buf.WriteString(content)
		buf.WriteString("</")
		if start.Name.Space != "" {
			buf.WriteString(start.Name.Space)
			buf.WriteString(":")
		}
		buf.WriteString(start.Name.Local)
		buf.WriteString(">")
	}
	return buf.String(), nil
}

// attrVal returns the value of a named attribute (local name match with w: namespace).
func attrVal(el xml.StartElement, localName string) string {
	for _, a := range el.Attr {
		if a.Name.Local == localName {
			return a.Value
		}
	}
	return ""
}

// hasAttrVal returns true if the element has an attribute with the given local name and value.
func hasAttrVal(el xml.StartElement, localName, value string) bool {
	return attrVal(el, localName) == value
}
