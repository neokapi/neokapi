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
	// boldClear/italicClear/strikeClear are set when a style entry
	// explicitly authored a clearing-form toggle — `<w:b w:val="0"/>`,
	// `<w:i w:val="false"/>`, `<w:strike w:val="off"/>` etc. — so
	// mergeProps can apply the clear during basedOn-chain resolution.
	// Without this, a child style's explicit-off toggle never overrides
	// a parent style's bare-on toggle (mergeProps only sets bool fields
	// when src is true; it had no signal to clear them). Mirrors
	// upstream Okapi RunProperties.minified() chain semantics
	// (RunProperties.java:497-540) and ECMA-376-1 §17.3.2.1
	// (CT_OnOff: explicit val="0"/false/off CLEARS the toggle in the
	// resolved style chain). Fixture `document-style-definitions.docx`
	// is the canonical case: `Normal1` basedOn `Style1` (bold=true)
	// authors `<w:b w:val="0"/>` to clear the inherited bold; without
	// the clear-tracking the resolved Normal1 chain incorrectly carries
	// bold=true and `subtractProps` strips the per-run `<w:b/>` from
	// any direct override on a Normal1-styled paragraph's middle run.
	//
	// These fields are populated only by parseStyles for style entries.
	// Per-run rPr keeps the existing explicit-off path: parseRunProps
	// preserves the clearing form in rPrChildren so it survives into
	// the per-run sidecar, and the writer emits it verbatim.
	boldClear   bool
	italicClear bool
	strikeClear bool
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

// equal returns true if two runProps produce the same visual formatting
// at the level of toggle props + primary font name. Font names are
// compared when set (for font mapping merging).
//
// equal INTENTIONALLY ignores rPrChildren (rStyle, color, sz, lang,
// highlight, etc.) because its callers — the open/close PcOpen/PcClose
// emitter loops in wml.go (~line 2198), sml.go (~line 376), and
// dml.go (~line 276) — only emit toggle markers. Non-toggle rPr
// children are carried through the per-source-run rPr sidecar (#592),
// not through the toggle Spans.
//
// For the stricter "should two runs be coalesced into one merged run"
// check used by mergeRuns, see equalIncludingChildren below — that
// mirrors upstream Okapi RunMerger.canRunPropertiesBeMerged
// (RunMerger.java:156-229) which compares EVERY RunProperty, not just
// toggles.
func (rp runProps) equal(other runProps) bool {
	return rp.bold == other.bold &&
		rp.italic == other.italic &&
		rp.underline == other.underline &&
		rp.strike == other.strike &&
		rp.vertAlign == other.vertAlign &&
		rp.vanish == other.vanish &&
		rp.fontName == other.fontName
}

// equalIncludingChildren reports whether two runProps are equivalent
// across the FULL rPr — toggles AND non-toggle children (rStyle, color,
// sz, lang, highlight, …).
//
// Mirrors upstream Okapi RunMerger.canRunPropertiesBeMerged
// (RunMerger.java:156-229): if any RunProperty differs (toggle OR
// non-toggle), runs cannot be coalesced. Per ECMA-376-1 §17.3.2,
// heterogeneous rPr means heterogeneous runs.
//
// This is the gate used by mergeRuns to keep the per-source-run rPr
// sidecar's run-count aligned with the merged-run count: when source
// runs carried different rPrChildren, mergeRuns must NOT collapse them
// into one merged run, otherwise the sidecar count and the merged-run
// count drift apart and Phase 2's alignment guard nils the sidecar
// (regression-free fallback).
//
// rPrChildren is compared as an ordered list of (name, xml) pairs.
// Order matters because parseRunProps preserves the source rPr child
// order, and upstream RunBuilder also keeps RunProperty order — two
// runs whose children differ only by ordering are unusual in
// well-formed WPML but, when they occur, upstream's filteredBy/contains
// also implicitly respects the property identity so order-equality is
// a safe overconstraint.
func (rp runProps) equalIncludingChildren(other runProps) bool {
	if !rp.equal(other) {
		return false
	}
	if len(rp.rPrChildren) != len(other.rPrChildren) {
		return false
	}
	for i, c := range rp.rPrChildren {
		oc := other.rPrChildren[i]
		if c.name != oc.name || c.xml != oc.xml {
			return false
		}
	}
	return true
}

// canBeMergedWith reports whether two runProps are mergeable per upstream
// Okapi RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229) —
// stricter than byte-equality but looser than equalIncludingChildren:
// rFonts is merged per-attribute (no contradictory values per shared
// content category — RunFonts.canBeMerged at RunFonts.java:190-247),
// every other rPr child must be byte-equal. Per ECMA-376-1 §17.3.2.26
// rFonts attributes (ascii, hAnsi, cs, eastAsia, *Theme, hint) are
// independent and an rFonts may carry any subset, so an rFonts that
// only specifies (ascii, cs) is compatible with one that specifies
// (ascii, hAnsi, cs) when their shared attributes agree.
//
// This is the gate used by mergeRuns. When two runs are mergeable but
// not byte-equal, mergeRPrChildren computes the merged rPrChildren so
// the surviving textRun carries the rFonts that satisfies both source
// runs.
func (rp runProps) canBeMergedWith(other runProps) bool {
	if !rp.equal(other) {
		return false
	}
	return rPrChildrenMergeable(rp.rPrChildren, other.rPrChildren)
}

// rPrChildrenMergeable reports whether two ordered rPrChildren slices
// can be merged per upstream Okapi RunMerger.canRunPropertiesBeMerged
// (RunMerger.java:156-229): every non-rFonts child must be byte-equal,
// and rFonts (if both have it) must be per-attribute compatible. An
// rFonts present on only one side is also mergeable (the merged result
// keeps it).
//
// The native rPrChildren list omits the toggle children
// (b/i/u/strike/vertAlign/vanish) — those flow through runProps's
// toggle fields. rPrChildren contains the non-toggle children that
// upstream RunBuilder tracks as Properties (rStyle, rFonts, color, sz,
// szCs, highlight, bCs, iCs, …).
func rPrChildrenMergeable(a, b []rPrChild) bool {
	// Build maps from name → xml for non-rFonts children.
	// Multiple children with the same name are unusual in well-formed
	// WPML; fall back to ordered byte comparison in that case.
	aOther, aFonts := splitRFonts(a)
	bOther, bFonts := splitRFonts(b)
	if len(aOther) != len(bOther) {
		return false
	}
	for i, c := range aOther {
		oc := bOther[i]
		if c.name != oc.name || c.xml != oc.xml {
			return false
		}
	}
	// rFonts: if both absent or only one side has it, mergeable.
	if aFonts == nil || bFonts == nil {
		return true
	}
	return rFontsMergeable(aFonts.xml, bFonts.xml)
}

// splitRFonts partitions rPrChildren into (non-rFonts entries, rFonts
// entry). The non-rFonts slice preserves source order. The rFonts
// pointer is nil when the slice carries no rFonts. When the slice
// carries multiple rFonts (malformed source), only the first is
// returned; the rest are kept in the non-rFonts slice so byte-equality
// at those slots still gates merging.
func splitRFonts(children []rPrChild) ([]rPrChild, *rPrChild) {
	var fonts *rPrChild
	out := make([]rPrChild, 0, len(children))
	for i := range children {
		if children[i].name == "rFonts" && fonts == nil {
			fonts = &children[i]
			continue
		}
		out = append(out, children[i])
	}
	return out, fonts
}

// rFontsMergeable reports whether two <w:rFonts ...> elements are
// per-attribute compatible: shared attribute names must have equal
// values; font-name attributes (ascii, hAnsi, cs, eastAsia, *Theme)
// may be present on only one side (the merged result drops attributes
// not shared by both — see mergeRFontsXML for the intersection rule
// matching upstream's "undetected category drops out" branch in
// RunFonts.mergeContentCategories at RunFonts.java:299-315).
//
// Per ECMA-376-1 §17.3.2.26 each content category (Latin/ASCII,
// HighAnsi, ComplexScript, EastAsian) has a direct font-name attribute
// (ascii / hAnsi / cs / eastAsia) AND a theme-reference alternative
// (asciiTheme / hAnsiTheme / cstheme / eastAsiaTheme). The two
// alternatives address the SAME content category — a run that asserts
// `ascii="Times New Roman"` cannot be merged with a run that asserts
// `asciiTheme="minorHAnsi"` because they describe different fonts for
// the same character range. Upstream Okapi's RunFonts.canBeMerged
// (RunFonts.java:190-247) walks ContentCategory enum members and uses
// `containsDetected` for both the direct and the theme alternative
// (`fontThemeContentCategories.get(contentCategory)`) — when both
// runs detect the category and the values differ (whether direct vs
// theme or direct vs direct), the merge fails. Native has no script
// detection, so the safe over-constraint is to treat ANY divergence
// across a theme-pair as a merge blocker — see fixture 1312-fonts-info*
// where Run 1 has `asciiTheme="minorHAnsi"` and Run 2 has
// `ascii="Times New Roman"`: upstream keeps the two runs separate, so
// native must too.
//
// The `hint` attribute is treated differently: it must be byte-equal
// on both sides OR ABSENT on both. Mirrors upstream Okapi
// RunFonts.canHintsBeMerged (RunFonts.java:232-248) without access
// to detected-content-category state — native cannot tell whether the
// other run "uses" the script category the hint addresses, so the
// safe over-constraint is to refuse merge whenever one run carries a
// hint and the other does not. This preserves the boundary between
// e.g. `<w:rFonts w:cs="Arial" w:hint="cs"/>` (complex-script text)
// and `<w:rFonts w:cs="Arial"/>` (whitespace span) so 1385-style
// paragraphs round-trip with all runs intact.
//
// Mirrors upstream Okapi RunFonts.canBeMerged (RunFonts.java:190-247).
func rFontsMergeable(aXML, bXML string) bool {
	aAttrs, ok := parseRFontsAttrs(aXML)
	if !ok {
		return false
	}
	bAttrs, ok := parseRFontsAttrs(bXML)
	if !ok {
		return false
	}
	aMap := make(map[string]string, len(aAttrs))
	for _, a := range aAttrs {
		aMap[a.name] = a.value
	}
	bMap := make(map[string]string, len(bAttrs))
	for _, b := range bAttrs {
		bMap[b.name] = b.value
		if v, ok := aMap[b.name]; ok && v != b.value {
			return false
		}
	}
	// Theme/direct-pair conflict: each content category (Latin/HighAnsi/
	// ComplexScript/EastAsian) has a direct attribute AND a theme
	// alternative, and asserting one on one run while the other asserts
	// the alternative for the same category is a merge blocker. Per
	// upstream RunFonts.canContentCategoriesBeMerged (RunFonts.java:211-
	// 230), the comparison walks `containsDetected(fontThemeCategory)`
	// for both the direct and theme alternatives. Without script
	// detection, treat any divergence on a theme-pair as a blocker.
	for _, pair := range rFontsThemePairs {
		_, aHasDirect := aMap[pair.direct]
		_, aHasTheme := aMap[pair.theme]
		_, bHasDirect := bMap[pair.direct]
		_, bHasTheme := bMap[pair.theme]
		// A asserts direct, B asserts theme (or vice versa) → conflict.
		// Both-direct or both-theme divergences are already caught by
		// the byte-equality loop above (same attribute name).
		if aHasDirect && bHasTheme {
			return false
		}
		if aHasTheme && bHasDirect {
			return false
		}
	}
	// Hint compatibility: refuse merge when only one side carries a
	// hint. Upstream's canHintsBeMerged also rejects most of those
	// cases (the rare exception is when the other run has no font in
	// the category the hint addresses — we cannot detect that here).
	aHasHint := hasHintAttr(aMap)
	bHasHint := hasHintAttr(bMap)
	return aHasHint == bHasHint
}

// rFontsThemePairs lists the (direct, theme) attribute pairs that
// address the same content category per ECMA-376-1 §17.3.2.26.
// Used by rFontsMergeable to detect mergeability conflicts across
// the direct/theme alternatives. Both prefixed and unprefixed forms
// are listed because the captured rFonts attribute strings use the
// "w:" prefix form when the source was in the WML namespace (the
// standard case) but downstream callers may also feed un-prefixed
// attribute names through the legacy otherXML capture path.
var rFontsThemePairs = []struct {
	direct, theme string
}{
	{"w:ascii", "w:asciiTheme"},
	{"w:hAnsi", "w:hAnsiTheme"},
	{"w:cs", "w:cstheme"},
	{"w:eastAsia", "w:eastAsiaTheme"},
	{"ascii", "asciiTheme"},
	{"hAnsi", "hAnsiTheme"},
	{"cs", "cstheme"},
	{"eastAsia", "eastAsiaTheme"},
}

// hasHintAttr returns true if the rFonts attribute map carries a
// `w:hint` (or bare `hint`) attribute.
func hasHintAttr(m map[string]string) bool {
	if _, ok := m["w:hint"]; ok {
		return true
	}
	_, ok := m["hint"]
	return ok
}

// mergeRPrChildren returns the merged rPrChildren of two mergeable
// runs. Non-rFonts children are taken from a (byte-equal to b's by
// the rPrChildrenMergeable contract). rFonts is the per-attribute
// intersection (shared attribute names with equal values) — matching
// the per-paragraph rFonts consensus computed by
// mergeRFontsAcrossRuns (source_rpr.go) for the WSO common-rPr lift.
//
// When the rFonts intersection is empty, the rFonts entry is dropped
// from the merged rPrChildren rather than emitted as an empty
// `<w:rFonts/>` — an attribute-less rFonts carries no formatting and
// would only noise up the per-run rPr sidecar.
//
// The caller must have established mergeability via
// rPrChildrenMergeable; this function does NOT re-check compatibility.
func mergeRPrChildren(a, b []rPrChild) []rPrChild {
	_, aFonts := splitRFonts(a)
	_, bFonts := splitRFonts(b)
	merged := mergeRFontsXML(aFonts, bFonts)
	mergedHasAttrs := rFontsHasAttrs(merged.xml)
	out := make([]rPrChild, 0, len(a)+1)
	rFontsEmitted := false
	for _, p := range a {
		if p.name == "rFonts" {
			if !rFontsEmitted && mergedHasAttrs {
				out = append(out, merged)
				rFontsEmitted = true
			}
			continue
		}
		out = append(out, p)
	}
	if !rFontsEmitted && aFonts == nil && bFonts != nil && mergedHasAttrs {
		out = append(out, merged)
	}
	return out
}

// rFontsHasAttrs reports whether a serialised `<w:rFonts .../>`
// fragment carries at least one attribute. Used to drop attribute-less
// rFonts from merged rPrChildren.
func rFontsHasAttrs(xmlStr string) bool {
	// Look for any name=" or name=' inside the start tag.
	end := strings.IndexAny(xmlStr, "/>")
	if end < 0 {
		return false
	}
	head := xmlStr[:end]
	return strings.ContainsAny(head, "=")
}

// mergeRFontsXML returns the per-attribute intersection of two rFonts
// entries — an attribute is kept iff BOTH sides carry it with equal
// values. This approximates upstream Okapi RunFonts.merge
// (RunFonts.java:267-315) in the absence of script-detection
// categories: upstream drops attributes whose category is not
// "detected" on either side, and undetected-equal-on-both also yields
// the shared value. Native has no detection signal, so the safe
// approximation is INTERSECTION — never invent an attribute that
// wasn't on every contributing run. This matches the per-paragraph
// rFonts consensus computed by mergeRFontsAcrossRuns (source_rpr.go)
// for the WSO common-rPr lift, so the post-merge per-run rPr stays in
// step with the lifted pStyle.
//
// Either input may be nil; nil means "no rFonts at all" on that side,
// which is treated identically to "rFonts with no attributes" — the
// intersection is empty unless the non-nil side also has zero
// attributes, in which case the result is an empty-element rFonts
// (`<w:rFonts/>`). When both are nil the result carries an empty xml
// field. The element name is taken from the first non-nil source.
func mergeRFontsXML(a, b *rPrChild) rPrChild {
	if a == nil && b == nil {
		return rPrChild{name: "rFonts"}
	}
	if a == nil || b == nil {
		// An rFonts on only one side has no intersection partner —
		// upstream would drop every undetected attribute. With no
		// detection signal in native, drop them all.
		var prefix string
		if a != nil {
			prefix = extractRFontsElemNameFromXML(a.xml)
		} else {
			prefix = extractRFontsElemNameFromXML(b.xml)
		}
		if prefix == "" {
			prefix = "w:rFonts"
		}
		return rPrChild{name: "rFonts", xml: "<" + prefix + "/>"}
	}
	aAttrs, _ := parseRFontsAttrs(a.xml)
	bAttrs, _ := parseRFontsAttrs(b.xml)
	bMap := make(map[string]string, len(bAttrs))
	for _, x := range bAttrs {
		bMap[x.name] = x.value
	}
	// Walk a's order; keep iff b has the same name with the same
	// value. This matches mergeRFontsAcrossRuns' iteration order.
	var kept []rfontsAttr
	for _, x := range aAttrs {
		if v, ok := bMap[x.name]; ok && v == x.value {
			kept = append(kept, x)
		}
	}
	prefix := extractRFontsElemNameFromXML(a.xml)
	if prefix == "" {
		prefix = extractRFontsElemNameFromXML(b.xml)
	}
	if prefix == "" {
		prefix = "w:rFonts"
	}
	var buf strings.Builder
	buf.WriteByte('<')
	buf.WriteString(prefix)
	for _, x := range kept {
		buf.WriteByte(' ')
		buf.WriteString(x.name)
		buf.WriteByte('=')
		q := x.quote
		if q == 0 {
			q = '"'
		}
		buf.WriteByte(q)
		buf.WriteString(x.value)
		buf.WriteByte(q)
	}
	buf.WriteString("/>")
	return rPrChild{name: "rFonts", xml: buf.String()}
}

// extractRFontsElemNameFromXML extracts the prefixed element name from
// a single rFonts XML fragment, e.g. "<w:rFonts .../>" → "w:rFonts".
// Returns "" when the input is malformed.
func extractRFontsElemNameFromXML(xmlStr string) string {
	if len(xmlStr) < 2 || xmlStr[0] != '<' {
		return ""
	}
	end := strings.IndexAny(xmlStr[1:], " \t\n\r/>")
	if end < 0 {
		return ""
	}
	return xmlStr[1 : 1+end]
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
// styleChainNames (when non-nil) is the set of WordprocessingML rPr
// child element local names contributed by docDefaults + the
// paragraph style's resolved basedOn chain (see
// styleMap.effectiveRPrChildNames). Passed through to
// minifyRPrChildren so explicit-off WPML toggles can be preserved
// when they clear a style-chain toggle by name (RunProperties.java
// :497-540, `preCombined.contains(p.getName())` branch). When nil
// (style optimisation disabled, or caller has no style context), the
// minifier falls back to its current behaviour (always strip default-
// valued WPML toggles); this matches the documented "lacks
// pStyle/docDefault inheritance" comment on minifyRPrChildren.
func parseRunProps(d *xml.Decoder, aggressive bool, styleChainNames map[string]bool) (runProps, error) {
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

			// Skip rsid and proof attributes in aggressive cleanup.
			// noProof's strip is gated on the transitional WPML
			// namespace for the same reason as the dedicated noProof
			// strip below: upstream Okapi's
			// SkippableElement.RUN_PROPERTY_NO_SPELLING_OR_GRAMMAR
			// QName binds to the transitional URI only
			// (SkippableElement.java:207). For Strict OOXML documents
			// `<w:noProof>` must be preserved on the run rPr —
			// 859.docx is the canonical fixture.
			if aggressive {
				if strings.HasPrefix(local, "rsid") ||
					local == "proofErr" ||
					local == "lastRenderedPageBreak" {
					skip = true
				}
				if local == "noProof" && t.Name.Space != wmlStrictNamespace {
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
			//
			// The lang skip is GATED on the transitional WPML namespace.
			// Upstream's RUN_PROPERTY_LANGUAGE QName binds to
			// Namespaces.WordProcessingML — the transitional URI
			// "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
			// (Namespaces.java:26). For Strict OOXML documents using
			// "http://purl.oclc.org/ooxml/wordprocessingml/main" (e.g.
			// 858.docx — Word's "Save As → Strict Open XML Document"
			// output), the QName does NOT match upstream's skippable
			// set, so <w:lang> is preserved in run rPr. Keeping it in
			// rPrChildren lets the WSO post-pass lift it into the
			// synthesised paragraph style — matching the upstream emit
			// shape for 858.docx where the common run-rPr lang is
			// promoted into the new <w:style>'s rPr and stripped from
			// each <w:r><w:rPr>. ECMA-376-1 / ISO/IEC 29500-1 §A.1.
			if local == "lang" && t.Name.Space != wmlStrictNamespace {
				skip = true
			}
			// The noProof skip is GATED on the transitional WPML namespace
			// for the same reason as <w:lang> above. Upstream's
			// SkippableElement.RunProperty.RUN_PROPERTY_NO_SPELLING_OR_GRAMMAR
			// (SkippableElement.java:207) binds to
			// Namespaces.WordProcessingML.getQName("noProof") — the
			// transitional URI. For Strict OOXML documents the QName
			// does NOT match upstream's skippable set, so <w:noProof>
			// is preserved in run rPr (RunSkippableElements does not
			// strip it on read). 859.docx is the canonical fixture: its
			// drawing-bearing run carries
			// `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>`
			// and both children must round-trip on the wire.
			//
			// rPrChange is a revision-tracking element (ECMA-376-1
			// §17.13.5.31 CT_RPrChange) — stripped under
			// auto-accept-revisions semantics regardless of namespace.
			if local == "noProof" && t.Name.Space != wmlStrictNamespace {
				skip = true
			}
			if local == "rPrChange" {
				skip = true
			}

			switch {
			case skip:
				if err := skipElement(d); err != nil {
					return props, err
				}
			case local == "b":
				// Toggle property per ECMA-376-1 §17.3.2.1 (<w:b>). A
				// bare element turns bold ON; an explicit `val="0"` /
				// `"false"` / `"off"` is the clearing form. The bare-on
				// path normalises into runProps.bold so the writer can
				// reconstruct it from the PcOpen/PcClose toggle codes.
				//
				// The clearing form is preserved verbatim in rPrChildren
				// so it survives into the per-run rPr sidecar (#592) —
				// upstream Okapi's RunProperties.minified() preserves a
				// clearing-value toggle when the inherited style chain
				// carries that property by name
				// (RunProperties.java:497-540, the
				// `!preCombined.contains(p.getName())` condition).
				// Native does not have the full preCombined view at
				// parse time, so the conservative choice — keep the
				// explicit-off so it round-trips on paragraphs whose
				// pStyle inherits bold (e.g. 1311.docx Heading2 with
				// `<w:b/>` + `<w:bCs/>` in the resolved chain). The
				// `bold=false` set keeps the toggle field semantics
				// consistent with the bare-off-equivalent state so the
				// writer's PcOpen/PcClose path correctly emits NO
				// `<w:b/>` for this run.
				if isExplicitOffBIToggle(t) {
					raw, err := serializeRPrChildElement(d, t)
					if err != nil {
						return props, err
					}
					props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
					props.bold = false
				} else {
					props.bold = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
					if err := skipElement(d); err != nil {
						return props, err
					}
				}
			case local == "bCs":
				// Complex-script bold per ECMA-376-1 §17.3.2.16. The
				// model has no separate complex-script bold toggle; bCs
				// travels with the run's full rPr serialization through
				// rPrChildren (#592). Both the bare-on form and the
				// explicit-off form are preserved verbatim — the
				// downstream minifyRPrChildren rule EXEMPTS bCs from the
				// strip-on-explicit-false default-toggle path (see
				// wpmlToggleNames) so the clearing form survives for
				// paragraphs whose inherited style chain turns bCs ON
				// (1311.docx Heading2).
				raw, err := serializeRPrChildElement(d, t)
				if err != nil {
					return props, err
				}
				props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
			case local == "i":
				// Toggle property per ECMA-376-1 §17.3.2.13 (<w:i>).
				// Same handling as <w:b> above — the explicit-off form
				// is preserved verbatim in rPrChildren so it survives
				// for paragraphs whose inherited style chain turns
				// italic ON.
				if isExplicitOffBIToggle(t) {
					raw, err := serializeRPrChildElement(d, t)
					if err != nil {
						return props, err
					}
					props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
					props.italic = false
				} else {
					props.italic = !hasAttrVal(t, "val", "0") && !hasAttrVal(t, "val", "false")
					if err := skipElement(d); err != nil {
						return props, err
					}
				}
			case local == "iCs":
				// Complex-script italic per ECMA-376-1 §17.3.2.17. Same
				// preservation as bCs above.
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
			props.rPrChildren = minifyRPrChildren(props.rPrChildren, styleChainNames)
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
// styleChainNames (when non-nil) is the set of WordprocessingML rPr
// child element local names that appear in the resolved style chain
// of the run's paragraph (docDefaults + basedOn chain). Mirrors
// upstream Okapi RunProperties.minified()'s
// `preCombined.contains(p.getName())` guard (RunProperties.java:527):
// when an rPr child is a WPML toggle with an explicit-off value AND
// the style chain has the property by name, the explicit-off form is
// preserved because it functions as a clearing override against the
// inherited toggle. A nil map preserves the legacy behaviour
// (always strip default-valued toggles), matching native's pre-#xxx
// pStyle-inheritance-absent baseline.
func minifyRPrChildren(children []rPrChild, styleChainNames map[string]bool) []rPrChild {
	if len(children) == 0 {
		return children
	}
	// Paired-toggle preservation signal for `bCs` / `iCs`. When the
	// SAME rPr also carries an explicit-off `<w:b>` / `<w:i>`, the
	// authoring tool authored the complex-script clearing override
	// alongside the Latin clearing override — a strong indicator that
	// the inherited style chain has the complex-script toggle ON (the
	// only case where upstream Okapi's
	// RunProperties.minified() preserves the clearing form per
	// `preCombined.contains("bCs")` in RunProperties.java:497-540).
	// Fixture 1311.docx is the canonical case: a `Heading2`-styled
	// paragraph whose direct rPr clears both <w:b> and <w:bCs>.
	// Native has no preCombined view at minify time but DOES see
	// the b-bCs / i-iCs pairing in rPrChildren (because the explicit-
	// off forms of <w:b> and <w:i> are now preserved in rPrChildren
	// — see the parseRunProps `case local == "b"` / `case local ==
	// "i"` clearing-form branches). Keeping the paired bCs/iCs
	// clearing form mirrors upstream's effective behaviour for
	// fixtures that author both halves of the pair together; the
	// default no-op-strip path still applies when the pair is
	// absent (so isolated `<w:bCs w:val="false"/>` on a paragraph
	// with no bCs in its style chain continues to be stripped).
	//
	// Per ECMA-376-1 §17.3.2.16 (<w:bCs>) and §17.3.2.17 (<w:iCs>)
	// these are independent toggle properties. Per §17.3.2.1 (<w:b>)
	// and §17.3.2.13 (<w:i>) the Latin halves are independent too.
	// The pairing-as-signal heuristic only fires when the source
	// authors both clearing forms together, so it cannot falsely
	// promote a lone <w:bCs w:val="false"/>.
	keepBCs := hasExplicitOffByName(children, "b")
	keepICs := hasExplicitOffByName(children, "i")
	out := children[:0]
	for _, c := range children {
		if keepBCs && c.name == "bCs" {
			out = append(out, c)
			continue
		}
		if keepICs && c.name == "iCs" {
			out = append(out, c)
			continue
		}
		if isDefaultValuedRPrChild(c) {
			// Mirror upstream Okapi RunProperties.minified()'s
			// `preCombined.contains(p.getName())` guard
			// (RunProperties.java:527). An explicit-off WPML toggle
			// (e.g. <w:rtl w:val="0"/>) is a CLEARING override when
			// the run's resolved style chain carries the toggle by
			// name — dropping it would let the inherited toggle leak
			// through and change effective formatting. Fixture
			// 899.docx (Normal style has <w:rtl/>) is the canonical
			// case: each run carries <w:rtl w:val="0"/> to suppress
			// the inherited <w:rtl/>; upstream Okapi keeps the
			// clearing form so the synthesised paragraph styles emit
			// <w:rtl w:val="0"/> in their rPr.
			//
			// When styleChainNames is nil (no style context — caller
			// disabled style optimisation OR is a unit test that does
			// not load styles.xml), fall through to the unconditional
			// strip to preserve the legacy behaviour for the
			// reordered-zip.docx-style fixtures whose source Normal
			// style has no rtl property.
			if styleChainNames == nil || !styleChainNames[c.name] {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// hasExplicitOffByName reports whether children contain an rPr child
// element with the given local name whose `val` attribute equals "0",
// "false", or "off" — the WPML clearing-form signature per ECMA-376-1
// §17.3.2. Used by minifyRPrChildren to detect paired-toggle
// preservation signals (b ↔ bCs / i ↔ iCs).
func hasExplicitOffByName(children []rPrChild, name string) bool {
	for _, c := range children {
		if c.name != name {
			continue
		}
		val, ok := parseRPrChildVal(c.xml)
		if !ok {
			continue
		}
		if val == "0" || val == "false" || val == "off" {
			return true
		}
	}
	return false
}

// wpmlToggleNames lists the WPML run-property toggle element local names
// that mirror upstream Okapi's RunPropertyFactory.WpmlTogglePropertyName
// enum (RunPropertyFactory.java:201-222). Each toggle defaults to "true"
// per ECMA-376-1 §17.3.2, so an explicit `w:val="0"` / `"false"` / `"off"`
// is a no-op and gets stripped by RunProperties.minified() WHEN the
// inherited style chain does not also carry the property by name.
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
// "nil". The upstream set is matched purely by `localPart`
// (RunProperties.java:512 calls `p.getName().getLocalPart()`), so the
// DML-flagged "cap" entry also drops a WPML `<w:cap w:val="none"/>` —
// see fixture 1440-default-formatting.docx whose translatable run carries
// `<w:cap w:val="none"/>` next to other clearing-value toggles.
var rPrOmittedWithNoneOrNil = map[string]bool{
	"brd":       true,
	"effect":    true,
	"em":        true,
	"highlight": true,
	"u":         true, // ALSO present in the toggle path, but value-defaulted
	"cap":       true, // DML by upstream comment, matched by local-name only
	"scheme":    true, // SML by upstream comment, matched by local-name only
}

// rPrOmittedWithZero mirrors upstream RunProperties.OMITTED_WITH_ZERO
// (RunProperties.java:382-390): these properties are omitted when their
// `val` attribute equals "0". Upstream matches by `localPart` only so
// DML members (`baseline`, `spc`) also fire on WPML elements that happen
// to share the local name.
var rPrOmittedWithZero = map[string]bool{
	"kern":     true,
	"position": true,
	"baseline": true, // DML by upstream comment, matched by local-name only
	"spc":      true, // DML by upstream comment, matched by local-name only
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

// rPrOmittedWithNoStrike mirrors upstream RunProperties.OMITTED_WITH_NO_STRIKE
// (RunProperties.java:399-402). Upstream matches by `localPart` only; the
// DML `strike` element accepts a `noStrike` value to mean "no
// strikethrough". The native parser would otherwise have already lifted a
// `<w:strike>` into runProps.strike, but a DML/SML element sharing the
// local name "strike" lands in rPrChildren via the default branch.
var rPrOmittedWithNoStrike = map[string]bool{
	"strike": true,
}

// rPrOmittedWithBlack lists run-property elements whose `val="000000"` is
// the implicit document-default per upstream Okapi WordStyleDefinition
// .DocumentDefaults.addExplicitDefaults() (WordStyleDefinition.java:164-228).
// When a docDefault `<w:rPrDefault>` lacks an explicit `<w:color>` child,
// upstream injects a synthetic `<w:color w:val="000000"/>` into the
// pre-combined run properties so any directly-specified `<w:color w:val=
// "000000"/>` on a run becomes a no-op duplicate of the precombined
// default and is dropped by RunProperties.minified()'s
// `preCombined.contains(p)` branch (RunProperties.java:504).
//
// Native does not materialise a precombined view, so we mirror the
// effective behaviour: drop `<w:color w:val="000000"/>` from rPrChildren
// unconditionally when the inherited style chain does not carry a
// `<w:color>` (the styleChainNames guard in minifyRPrChildren). When the
// style chain DOES carry a non-000000 color, the directly-specified
// 000000 is a real clearing override and must be preserved — that case
// goes through the `styleChainNames[c.name]` branch in minifyRPrChildren.
//
// References:
//   - WordStyleDefinition.java:122 explicitDefaultColorPropertyValue =
//     systemColorValues.valueFor(Color.System.DEFAULT_FOREGROUND_NAME).asRgb()
//     where DEFAULT_FOREGROUND_NAME = "windowText" → "000000".
//   - WordStyleDefinition.java:192-227 addExplicitDefaults() — adds the
//     synthetic ColorRunProperty with val=000000 to the docDefaults rPr.
//   - RunProperties.java:497-540 minified() — drops properties already
//     present (by equality) in the preCombined chain.
//   - ECMA-376-1 §17.3.2.6 (`<w:color>`).
//
// Fixtures: 830-7.docx (run-level redundant `<w:color w:val="000000"/>`),
// 1335-doc-properties.docx (synthesised paragraph style picking up the
// runs' redundant color).
var rPrOmittedWithBlack = map[string]bool{
	"color": true,
}

// rPrExplicitOffBINames lists the run-property elements whose explicit-off
// form (val="0"|"false"|"off") is a no-op when the inherited style chain
// does not carry the property by name. The `b` and `i` toggles are
// "model" toggles in native (parseRunProps lifts the bare-on form into
// runProps.bold / runProps.italic and discards the element), but the
// EXPLICIT-OFF branch in parseRunProps preserves the clearing form in
// rPrChildren so it can survive when the resolved style chain turns the
// toggle ON (1311.docx Heading2 case). When the style chain does NOT
// carry the toggle, the clearing form is a no-op duplicate of the
// implicit default-off and must be stripped — otherwise it leaks into
// synthesised paragraph styles via WSO and diverges from upstream
// (1335-doc-properties.docx is the canonical case: every run carries
// `<w:b w:val="0"/>` + `<w:i w:val="0"/>` against a Normal-based pStyle
// whose chain has no b/i, so upstream RunProperties.minified() drops
// both before WSO computes commonRunProperties).
//
// Per ECMA-376-1 §17.3.2.1 (`<w:b>`) and §17.3.2.13 (`<w:i>`) these
// toggles default to OFF in the absence of an inherited override, so the
// explicit-off form is a no-op. Upstream Okapi handles these via the
// `WpmlToggleRunProperty && !getToggleValue()` branch on
// RunProperties.java:506-510, gated by the same
// `!preCombined.contains(p.getName())` guard
// (RunProperties.java:527) that `styleChainNames[c.name]` mirrors.
var rPrExplicitOffBINames = map[string]bool{
	"b": true,
	"i": true,
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
	// b / i model toggles default to OFF: an explicit-off value is a
	// no-op duplicate of the implicit default. The clearing form
	// reaches rPrChildren via the explicit-off branches in parseRunProps
	// (lines 676-688 for `b`, 711-723 for `i`); the styleChainNames
	// guard in minifyRPrChildren preserves it when the inherited style
	// chain turns the toggle ON.
	if rPrExplicitOffBINames[c.name] && hasVal {
		switch val {
		case "0", "false", "off":
			return true
		}
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
	if rPrOmittedWithNoStrike[c.name] && hasVal && val == "noStrike" {
		return true
	}
	if rPrOmittedWithBlack[c.name] && hasVal && val == "000000" {
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
//
// The strict parameter selects which WML namespace the "w" prefix is
// bound to: when true, the bare-"w:" elements re-hydrate into the
// Strict OOXML URI (Names.Space == wmlStrictNamespace), which the
// parseRunProps lang-skip gate uses to mirror upstream Okapi's
// namespace-keyed RUN_PROPERTY_LANGUAGE QName behaviour
// (Namespaces.java:26-27). Without this, raw-captured rPr from a
// Strict OOXML document would re-hydrate as transitional WPML and
// parseRunProps would incorrectly strip <w:lang> from rPrChildren.
// styleChainNames (when non-nil) is forwarded to parseRunProps →
// minifyRPrChildren so explicit-off WPML toggles can be preserved
// as style-chain clearing overrides. See parseRunProps for the
// upstream-Okapi citation.
func parseRunPropsFromRaw(rPrXML string, aggressive bool, strict bool, styleChainNames map[string]bool) (runProps, error) {
	wNS := wmlNamespace
	if strict {
		wNS = wmlStrictNamespace
	}
	wrapped := `<root xmlns:w="` + wNS + `"` +
		` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
		` xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"` +
		` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"` +
		` xmlns="` + wNS + `">` +
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
	return parseRunProps(d, aggressive, styleChainNames)
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
	case "http://purl.oclc.org/ooxml/wordprocessingml/main":
		// Strict OOXML WPML namespace (ISO/IEC 29500-1 §A.1). Same
		// `w:` prefix as transitional WPML — the writer always emits
		// the `w:` prefix regardless of which URI the document binds
		// it to, mirroring upstream Okapi's prefix-preservation
		// policy on round-trip.
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

// isExplicitOffBIToggle reports whether a WPML <w:b> / <w:bCs> / <w:i>
// / <w:iCs> element carries an explicit clearing `val` attribute. Per
// ECMA-376-1 §17.3.2 (toggle properties), a bare element (no attribute)
// defaults to "true" — bold/italic ON — and an explicit `val="0"`,
// `"false"`, or `"off"` clears the toggle. The explicit-off form is
// preserved by parseRunProps in rPrChildren so paragraphs whose
// inherited style turns the toggle ON round-trip with the clearing
// override intact (1311.docx Heading2 → `<w:b/>` + `<w:bCs/>`).
func isExplicitOffBIToggle(t xml.StartElement) bool {
	return hasAttrVal(t, "val", "0") ||
		hasAttrVal(t, "val", "false") ||
		hasAttrVal(t, "val", "off")
}
