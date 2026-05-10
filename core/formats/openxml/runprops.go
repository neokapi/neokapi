package openxml

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// runProps holds normalized run properties extracted from <w:rPr>.
type runProps struct {
	bold      bool
	italic    bool
	underline string // "single", "double", etc. — empty means none
	strike    bool
	vertAlign string // "superscript", "subscript", or ""
	vanish    bool   // hidden text
	// vanishExplicit is true when the source rPr carried an explicit
	// <w:vanish.../> element, regardless of its toggle value (true,
	// false, off, 0). This distinguishes "direct rPr overrides
	// inherited vanish" from "no direct vanish at all" so that
	// inheritance from pStyle/docDefaults can be applied only when the
	// run did not specify vanish itself. Mirrors upstream Okapi's
	// minified() preCombined.contains(p) rule
	// (RunProperties.java:497-540): a directly specified property is
	// kept iff it is not already in the inherited hierarchy with the
	// SAME value — an override (e.g. directly false against inherited
	// true) is preserved.
	vanishExplicit bool
	fontName       string // primary font name from <w:rFonts> (ascii or hAnsi)
	fontNameCS     string // complex script font name
	fontNameEA     string // East Asian font name
	otherXML       string // serialized non-formatting properties we preserve but don't compare
	// rPrChildren is the ordered list of <w:rPr> child element
	// serializations as they appeared on the source <w:r>, used to
	// preserve per-run rPr through the writer (#592).
	//
	// Each entry is a fully-formed XML fragment for one rPr child
	// (e.g. `<w:rStyle w:val="Emphasis"/>`,
	// `<w:color w:val="FF0000"/>`, `<w:sz w:val="24"/>`). The toggle
	// children that the writer reconstructs from PcOpen/PcClose runs
	// (b, i, u, strike, vertAlign, vanish) are EXCLUDED from this
	// list to avoid double-emission. Bowrain Issue #592 + ECMA-376-1
	// §17.3.2.30.
	rPrChildren []rPrChild
}

// rPrChild captures one <w:rPr> child element by its local name and
// raw XML serialization. Identity for "common rPr across runs"
// detection is by exact xml-string equality (matching upstream Okapi
// StyleOptimisation.commonRunPropertiesOf, which compares Property
// instances by Object.equals on serialized form — see
// StyleOptimisation.java lines 204-237 of the openxml-filter source).
//
// The xml field stores the element with WML's "w:" prefix (NOT the
// full namespace URI Go's encoding/xml hands back via Name.Space).
// This is required so the writer can re-emit the element directly into
// document.xml, which uses the "w:" prefix throughout.
type rPrChild struct {
	name string // local element name, e.g. "rStyle", "color", "sz"
	xml  string // raw XML serialization, e.g. `<w:color w:val="FF0000"/>`
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
//
// In addition to the normalised toggle/font fields, parseRunProps captures
// the FULL list of rPr child elements as they appeared in source order
// (props.rPrChildren). This is the materials needed by the writer to emit
// per-source-run rPr on output (Bowrain Issue #592 — ECMA-376-1 §17.3.2.30).
//
// rPrChildren EXCLUDES:
//   - the per-run rsid* attributes that aggressive cleanup strips
//   - the toggle elements (b, i, u, strike, vertAlign, vanish) — those are
//     reconstructed by the writer from PcOpen/PcClose Runs in the model.
//
// rPrChildren INCLUDES non-toggle children (rStyle, color, sz, szCs,
// rFonts, lang, highlight, bCs, iCs, …). Mirroring upstream Okapi
// RunBuilder.java (lines 73-188): every rPr child not classified as a
// toggle by RunSkippableElements becomes a tracked Property on the run.
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

			// Mirror upstream RunSkippableElements (RunSkippableElements.java
			// lines 50-62 of okapi/filters/openxml). The reader strips
			// these from <w:rPr> so they don't influence common-rPr
			// detection in WordStyleOptimisation and never leak into the
			// writer's per-run rPr output:
			//   - RUN_PROPERTY_LANGUAGE        (<w:lang>)
			//   - RUN_PROPERTY_NO_SPELLING_OR_GRAMMAR (<w:noProof>)
			//   - RUN_PROPERTIES_CHANGE        (<w:rPrChange>) — revision
			//
			// Without this skip, a paragraph whose only rPr difference is
			// <w:lang/> would get a synthesised pStyle by the WSO post-pass
			// even though Okapi keeps the paragraph rPr-less (the lang is
			// stripped by the writer's stripWMLSkippableElements). #592.
			if local == "lang" || local == "noProof" || local == "rPrChange" {
				skip = true
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
				// Complex script bold — preserve verbatim for the writer
				// (#592). The model has no separate complex-script bold
				// toggle; bCs travels with the run's full rPr serialization.
				raw, err := serializeRPrChildElement(d, t)
				if err != nil {
					return props, err
				}
				props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
			case local == "i":
				props.italic = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "iCs":
				raw, err := serializeRPrChildElement(d, t)
				if err != nil {
					return props, err
				}
				props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
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
				props.vanishExplicit = true
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
				// Capture both forms: otherXML keeps the legacy
				// full-namespace serialisation (used only by the
				// internal otherXML field, which never reaches the
				// writer), and rPrChildren keeps the writer-friendly
				// "w:"-prefixed form (#592). Two separate calls are
				// needed because each consumes the element subtree.
				raw, err := serializeWithCapture(d, t)
				if err != nil {
					return props, err
				}
				otherParts = append(otherParts, raw.legacy)
				props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw.wmlPrefixed})
			default:
				// Preserve unknown properties as raw XML in both forms.
				raw, err := serializeWithCapture(d, t)
				if err != nil {
					return props, err
				}
				otherParts = append(otherParts, raw.legacy)
				props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw.wmlPrefixed})
			}

		case xml.EndElement:
			// End of rPr
			if len(otherParts) > 0 {
				slices.Sort(otherParts)
				props.otherXML = strings.Join(otherParts, "")
			}
			// Apply RunProperties.minified() — strip default-valued
			// run-property entries from props.rPrChildren before the
			// downstream WSO/source-rPr passes see them. Mirrors upstream
			// RunPropertiesParser → RunProperties.minified(combined) in
			// RunParser.java:280-294 + RunProperties.java:497-540.
			//
			// Native lacks pStyle/docDefault inheritance ("combined" is
			// effectively empty), so only the no-op-default branch of
			// upstream's minified() applies here: a directly-specified
			// property with a clearing-formatting value (false/0/none/
			// nil/etc.) drops out because it would not have been there
			// anyway in the inherited hierarchy. The "drop p when
			// preCombined.contains(p)" branch is a no-op here.
			//
			// Without this, redundant `<w:rtl w:val="0"/>`,
			// `<w:vanish w:val="false"/>`, etc. round-trip into
			// synthesised pStyles via WSO and diverge from the upstream
			// reference (reordered-zip.docx).
			props.rPrChildren = minifyRPrChildren(props.rPrChildren)
			return props, nil
		}
	}
}

// minifyRPrChildren drops rPr child entries that carry a default-valued
// no-op formatting toggle / property. Mirrors upstream Okapi
// RunProperties.Default.minified() in
// okapi/filters/openxml/RunProperties.java:497-540 plus the omitted-
// default constant tables on RunProperties.java:370-402.
//
// The native parser never reaches this function with toggle children
// for the "model" toggles (b, i, u, strike, vertAlign, vanish) — those
// are normalised into runProps fields and never enter rPrChildren.
// Other WPML toggle properties (rtl, caps, smallCaps, dstrike, outline,
// shadow, emboss, imprint, webHidden, cs, specVanish, snapToGrid, oMath)
// land in rPrChildren via the default branch of parseRunProps, so this
// is where they have to be filtered.
//
// References:
//   - ECMA-376-1 §17.3.2 toggle properties default to "true" (an
//     attribute-less element means on). Explicit `w:val="0"` /
//     `w:val="false"` / `w:val="off"` is a no-op when no parent style
//     turns the toggle on.
//   - okapi/filters/openxml/RunPropertyFactory.java:201-222 enumerates
//     the WpmlToggleRunProperty set; those names trigger the
//     `WpmlToggleRunProperty && !getToggleValue()` branch on
//     RunProperties.java:506-510.
//   - RunProperties.java:370-402 lists the value-defaulted run
//     properties (none/nil for u/highlight/em/effect/brd, 0 for kern/
//     position, baseline for vertAlign, …). The matching branches on
//     RunProperties.java:511-525 strip those when the value matches the
//     documented default.
func minifyRPrChildren(children []rPrChild) []rPrChild {
	if len(children) == 0 {
		return children
	}
	out := children[:0]
	for _, c := range children {
		if isDefaultValuedRPrChild(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// wpmlToggleNames lists the WPML run-property toggle element local names
// that mirror upstream Okapi's RunPropertyFactory.WpmlTogglePropertyName
// enum (RunPropertyFactory.java:201-222). Each toggle defaults to "true"
// per ECMA-376-1 §17.3.2, so an explicit `w:val="0"` / `"false"` / `"off"`
// is a no-op and gets stripped by RunProperties.minified().
//
// The members b, i, u (handled separately), strike, vertAlign, vanish are
// excluded from this set in native because parseRunProps maps them onto
// runProps struct fields rather than into rPrChildren — they never reach
// minifyRPrChildren.
var wpmlToggleNames = map[string]bool{
	"caps":       true,
	"smallCaps":  true,
	"dstrike":    true,
	"outline":    true,
	"shadow":     true,
	"emboss":     true,
	"imprint":    true,
	"webHidden":  true,
	"specVanish": true,
	"noProof":    true,
	"snapToGrid": true,
	"oMath":      true,
	"cs":         true,
	"rtl":        true,
	"bCs":        true,
	"iCs":        true,
}

// rPrOmittedWithNoneOrNil mirrors upstream
// RunProperties.OMITTED_WITH_NONE_OR_NIL (RunProperties.java:370-380):
// these properties are omitted when their `val` attribute is "none" or
// "nil". Limited here to the WPML names since the native rPr parser only
// sees WPML rPr children.
var rPrOmittedWithNoneOrNil = map[string]bool{
	"brd":       true,
	"effect":    true,
	"em":        true,
	"highlight": true,
	"u":         true, // ALSO present in the toggle path, but value-defaulted
}

// rPrOmittedWithZero mirrors upstream RunProperties.OMITTED_WITH_ZERO
// (RunProperties.java:382-390): these properties are omitted when their
// `val` attribute equals "0". Limited to WPML members.
var rPrOmittedWithZero = map[string]bool{
	"kern":     true,
	"position": true,
}

// rPrOmittedWithHundred mirrors upstream RunProperties.OMITTED_WITH_HUNDRED
// (RunProperties.java:391-394).
var rPrOmittedWithHundred = map[string]bool{
	"w": true,
}

// rPrOmittedWithBaseline mirrors upstream RunProperties.OMITTED_WITH_BASELINE
// (RunProperties.java:395-398).
var rPrOmittedWithBaseline = map[string]bool{
	"vertAlign": true,
}

// isDefaultValuedRPrChild returns true when c is a run-property element
// whose value matches its documented no-op default per upstream Okapi's
// RunProperties.minified() rules.
func isDefaultValuedRPrChild(c rPrChild) bool {
	val, hasVal := parseRPrChildVal(c.xml)
	// WPML toggles default to true: any explicit false-equivalent value
	// drops the entry.
	if wpmlToggleNames[c.name] {
		if !hasVal {
			return false // bare element ≡ val="true" → not a default no-op
		}
		switch val {
		case "0", "false", "off":
			return true
		}
		return false
	}
	if rPrOmittedWithNoneOrNil[c.name] && hasVal {
		if val == "none" || val == "nil" {
			return true
		}
	}
	if rPrOmittedWithZero[c.name] && hasVal && val == "0" {
		return true
	}
	if rPrOmittedWithHundred[c.name] && hasVal && val == "100" {
		return true
	}
	if rPrOmittedWithBaseline[c.name] && hasVal && val == "baseline" {
		return true
	}
	return false
}

// parseRPrChildVal extracts the `w:val` (or bare `val`) attribute value
// from a serialised single rPr child element XML fragment, e.g.
// `<w:rtl w:val="0"/>` → ("0", true). Returns ("", false) when no `val`
// attribute is present (a bare `<w:rtl/>` is "val=true" by default and
// should NOT be minified).
//
// The fragment is always one element with no nested children, produced by
// serializeRPrChildElement / serializeWithCapture in this same file, so a
// scoped string scan is sufficient — no need to wrap-and-decode through
// encoding/xml.
func parseRPrChildVal(elemXML string) (string, bool) {
	// Find the end of the start tag (`>` or `/>`); we only ever need
	// attributes from the opening tag.
	end := strings.IndexAny(elemXML, ">")
	if end < 0 {
		return "", false
	}
	head := elemXML[:end]
	for _, key := range []string{` w:val="`, ` val="`} {
		if i := strings.Index(head, key); i >= 0 {
			rest := head[i+len(key):]
			j := strings.IndexByte(rest, '"')
			if j < 0 {
				return "", false
			}
			return rest[:j], true
		}
	}
	return "", false
}

// parseRunPropsFromRaw re-parses an already-captured <w:rPr>...</w:rPr>
// blob (as produced by captureRawElement) for typed properties. Used by
// callers that need to keep the raw rPr around for opaque emission yet
// also need the strongly-typed runProps view (e.g. complex-field run
// capture, where the entire <w:r> is preserved verbatim AND the typed
// props feed downstream merging / style-optimisation passes).
//
// The captured blob uses the bare "w:" prefix (the writer's canonical
// prefix table — captureRawElement → writeElementName) but carries no
// xmlns binding, so encoding/xml would otherwise leave the prefix
// unbound and downstream serialisation via prefixForNamespace would
// drop the prefix entirely. Wrap the blob in a synthetic root element
// that declares the standard WML namespaces so the decoder hydrates
// the same Name.Space URIs the on-the-fly path produces (compare
// runprops.go:484 writeStartTag, which keys off prefixForNamespace).
func parseRunPropsFromRaw(rPrXML string, aggressive bool) (runProps, error) {
	wrapped := `<root xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
		` xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"` +
		` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
		` xmlns="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		rPrXML + `</root>`
	d := xml.NewDecoder(strings.NewReader(wrapped))
	// Drain past <root> and the inner <w:rPr> start tag so
	// parseRunProps sees the rPr children, matching the original
	// on-the-fly call shape (which is invoked already positioned past
	// the <w:rPr> start element by the parent <w:r> loop).
	startsSeen := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return runProps{}, err
		}
		if _, ok := tok.(xml.StartElement); ok {
			startsSeen++
			if startsSeen == 2 {
				break
			}
		}
	}
	return parseRunProps(d, aggressive)
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

// serializedRPrChild bundles the two serialisations parseRunProps
// needs in lockstep: the legacy full-namespace-URI form (for
// otherXML, kept for backwards compatibility with code paths that
// never write it back) and the WML-prefixed form (for the writer's
// per-source-run rPr re-emission, #592). Built in a single subtree
// walk so the decoder is consumed exactly once per element.
type serializedRPrChild struct {
	legacy      string // "<http://schemas..../wordprocessingml/2006/main:rStyle ...>"
	wmlPrefixed string // "<w:rStyle ...>" — what the writer needs
}

// serializeWithCapture walks the start element's subtree and returns
// both serialisation forms.
func serializeWithCapture(d *xml.Decoder, start xml.StartElement) (serializedRPrChild, error) {
	var legacy, wml strings.Builder
	writeStartTagLegacy(&legacy, start)
	writeStartTag(&wml, start)

	depth := 1
	var legacyInner, wmlInner strings.Builder
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return serializedRPrChild{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			writeStartTagLegacy(&legacyInner, t)
			legacyInner.WriteString(">")
			writeStartTag(&wmlInner, t)
			wmlInner.WriteString(">")
		case xml.EndElement:
			depth--
			if depth > 0 {
				legacyInner.WriteString("</")
				if t.Name.Space != "" {
					legacyInner.WriteString(t.Name.Space)
					legacyInner.WriteString(":")
				}
				legacyInner.WriteString(t.Name.Local)
				legacyInner.WriteString(">")
				wmlInner.WriteString("</")
				wmlInner.WriteString(prefixForNamespace(t.Name.Space))
				wmlInner.WriteString(t.Name.Local)
				wmlInner.WriteString(">")
			}
		case xml.CharData:
			legacyInner.Write(t)
			wmlInner.Write(t)
		}
	}

	if legacyInner.Len() == 0 {
		legacy.WriteString("/>")
	} else {
		legacy.WriteString(">")
		legacy.WriteString(legacyInner.String())
		legacy.WriteString("</")
		if start.Name.Space != "" {
			legacy.WriteString(start.Name.Space)
			legacy.WriteString(":")
		}
		legacy.WriteString(start.Name.Local)
		legacy.WriteString(">")
	}
	if wmlInner.Len() == 0 {
		wml.WriteString("/>")
	} else {
		wml.WriteString(">")
		wml.WriteString(wmlInner.String())
		wml.WriteString("</")
		wml.WriteString(prefixForNamespace(start.Name.Space))
		wml.WriteString(start.Name.Local)
		wml.WriteString(">")
	}
	return serializedRPrChild{legacy: legacy.String(), wmlPrefixed: wml.String()}, nil
}

// writeStartTagLegacy writes an opening tag using the full namespace
// URI as the prefix (encoding/xml's natural format). Mirrors the
// serializeElement loop pre-#592 for compatibility with the otherXML
// field that some tests assert against.
func writeStartTagLegacy(buf *strings.Builder, start xml.StartElement) {
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
}

// serializeRPrChildElement captures an <w:rPr> child element and its
// content as a raw XML string, using WordprocessingML's "w:" element
// prefix and "w:" attribute prefix everywhere.
//
// Unlike the generic serializeElement (which writes Name.Space verbatim
// — Go's encoding/xml expands the prefix to the full namespace URI in
// Name.Space — the result of which is "<http://schemas..../wordprocessingml/2006/main:rStyle>"
// rather than "<w:rStyle>"), this writer is dedicated to rPr children
// where the natural target prefix is "w:" and any non-WML attribute
// (xml:space, w14:foo, …) keeps its original prefix as best we can
// reconstruct from the namespace URI.
//
// Used by parseRunProps to populate rPrChildren so the writer can re-emit
// each preserved child verbatim into document.xml. #592.
func serializeRPrChildElement(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	writeStartTag(&buf, start)

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
			writeStartTag(&inner, t)
			inner.WriteString(">")
		case xml.EndElement:
			depth--
			if depth > 0 {
				inner.WriteString("</")
				inner.WriteString(prefixForNamespace(t.Name.Space))
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
		buf.WriteString(prefixForNamespace(start.Name.Space))
		buf.WriteString(start.Name.Local)
		buf.WriteString(">")
	}
	return buf.String(), nil
}

// writeStartTag writes the opening "<prefix:name attr1=\"v1\" ...>" portion
// of an element (no terminator) using prefixForNamespace to map xml.Name
// namespaces back to their compact WML/MC/W14 prefix.
func writeStartTag(buf *strings.Builder, start xml.StartElement) {
	buf.WriteString("<")
	buf.WriteString(prefixForNamespace(start.Name.Space))
	buf.WriteString(start.Name.Local)
	for _, attr := range start.Attr {
		// xmlns declarations from the source are already implicit on
		// the document element of the writer's output; redundant
		// xmlns="..." or xmlns:foo="..." attributes here would muddy
		// canonical comparisons. Skip them.
		if attr.Name.Local == "xmlns" || attr.Name.Space == "xmlns" {
			continue
		}
		buf.WriteString(" ")
		buf.WriteString(prefixForNamespace(attr.Name.Space))
		buf.WriteString(attr.Name.Local)
		buf.WriteString("=\"")
		buf.WriteString(escapeAttrVal(attr.Value))
		buf.WriteString("\"")
	}
}

// prefixForNamespace maps the most common OOXML namespace URIs back to
// their compact prefix as used by document.xml. Returns "" (empty
// prefix, becomes a bare local-name attribute or element) for unknown
// namespaces — fixture corpus is dominated by the WML/MC/W14 set so a
// fixed table is sufficient. The trailing colon is included.
//
// Mirrors okapi/filters/openxml/Namespaces.java which keeps a similar
// fixed-prefix table on the writer side.
func prefixForNamespace(ns string) string {
	switch ns {
	case "":
		return ""
	case "http://schemas.openxmlformats.org/wordprocessingml/2006/main":
		return "w:"
	case "http://schemas.openxmlformats.org/markup-compatibility/2006":
		return "mc:"
	case "http://schemas.microsoft.com/office/word/2010/wordml":
		return "w14:"
	case "http://schemas.microsoft.com/office/word/2012/wordml":
		return "w15:"
	case "http://schemas.microsoft.com/office/word/2006/wordml":
		return "wne:"
	case "http://www.w3.org/XML/1998/namespace":
		return "xml:"
	case "http://schemas.openxmlformats.org/officeDocument/2006/relationships":
		return "r:"
	case "http://schemas.openxmlformats.org/drawingml/2006/main":
		return "a:"
	case "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":
		return "wp:"
	case "http://schemas.openxmlformats.org/drawingml/2006/picture":
		return "pic:"
	case "urn:schemas-microsoft-com:vml":
		return "v:"
	case "urn:schemas-microsoft-com:office:office":
		return "o:"
	}
	// Unknown — emit bare local-name (the canonical comparator
	// handles namespace-prefixed/un-prefixed equivalence at a higher
	// layer; rPr children rarely sit outside the WML namespace).
	return ""
}

// escapeAttrVal performs the minimal XML attribute-value escaping
// required to round-trip an attribute value through serialisation.
// Mirrors the canonical attribute-value escaping pattern
// (RFC 7049 — XML 1.0 §3.3.3 attribute-value normalisation).
func escapeAttrVal(s string) string {
	if !strings.ContainsAny(s, `<>&"`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
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
