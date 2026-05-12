package openxml

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strings"
)

// runProps holds normalized run properties extracted from <w:rPr>.
type runProps struct {
	bold bool
	// boldXML preserves the source serialisation of an explicit-on
	// `<w:b ...>` toggle (e.g. `<w:b w:val="1"/>`). Empty means the
	// source authored the bare on-form `<w:b/>` (or the run has no
	// bold). Per ECMA-376-1 §17.3.2.1 (CT_OnOff <w:b>) the bare
	// element and val="1"/"true"/"on" are equivalent ON states, but
	// upstream Okapi preserves the source form across the
	// round-trip (RunProperties.minified() retains the captured
	// RunProperty's exact QName + attributes; RunProperties.java:
	// 497-540). 830-2.docx and 830-6.docx are the canonical
	// fixtures: hyperlink-display runs author `<w:b w:val="1"/>`,
	// reference output preserves it.
	boldXML string
	italic  bool
	// italicXML mirrors boldXML for <w:i> per ECMA-376-1 §17.3.2.13
	// (CT_OnOff <w:i>).
	italicXML string
	underline string // "single", "double", etc. — empty means none
	strike    bool
	vertAlign string // "superscript", "subscript", or ""
	vanish    bool   // hidden text
	// vanishXML mirrors boldXML/italicXML for `<w:vanish>` per
	// ECMA-376-1 §17.3.2.42 (CT_OnOff). When the source authored an
	// explicit-on form (e.g. `<w:vanish w:val="on"/>` /
	// `<w:vanish w:val="1"/>` / `<w:vanish w:val="true"/>`), this
	// preserves the source serialisation so the writer can re-emit
	// the original form. Empty means the source authored the bare
	// `<w:vanish/>` (or vanish=false). Per ECMA-376-1 §17.3.2.1
	// (CT_OnOff) the bare element and val="1"/"true"/"on" are
	// equivalent ON states, but upstream Okapi's RunProperties.
	// minified() preserves the source RunProperty's exact QName +
	// attributes (RunProperties.java:497-540). Fixture
	// HiddenTablesApachePoi.docx authors `<w:vanish w:val="on"/>`
	// on the hidden table runs; the WSO post-pass lifts vanish into
	// a synthesised pStyle and the bridge preserves the explicit-on
	// form there.
	vanishXML string
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


// canBeMergedWithTexts is the text-aware variant of canBeMergedWith.
// When either side's text is whitespace-only (or empty), per upstream
// Okapi RunFonts.canContentCategoriesBeMerged (RunFonts.java:211-230)
// the whitespace run has no detected content categories and its
// rFonts attributes are "irrelevant" for any script category — the
// shared-attribute byte-equality requirement relaxes per category.
// Concretely: if A's text is Latin-only and B's text is whitespace,
// B's `w:ascii` value can differ from A's `w:ascii` because B has no
// Latin chars that "use" the ascii font category. The merged result
// keeps A's value for the category (mergeRFontsXMLTextAware below).
//
// rText / otherText are the source text bodies of the two runs (the
// raw `<w:t>` payload, NOT the post-merge concatenation). Sentinels
// (tab, image, …) and line-breaks should be filtered out by the
// caller — they don't pass through the rFonts merge anyway because
// mergeRuns refuses to fuse them with text runs.
//
// Mirrors upstream Okapi RunFonts.canBeMerged + RunFonts.merge
// (RunFonts.java:190-315) where the per-category gate consults
// `containsDetected(category)` on each side; an "undetected" run
// on either side allows the merge for that category.
//
// Used by mergeRuns to fuse adjacent runs where one is a
// whitespace-only spacer between two text bodies sharing the same
// rPr otherwise (1200-1.docx canonical: a Times-ascii space run
// between two Georgia-ascii text runs both carrying b/bCs +
// matching color/sz/lang).
func (rp runProps) canBeMergedWithTexts(other runProps, rText, otherText string) bool {
	if !rp.equalTextAware(other, rText, otherText) {
		return false
	}
	return rPrChildrenMergeableTexts(rp.rPrChildren, other.rPrChildren, rText, otherText)
}

// equalTextAware reports whether two runProps have equal toggle state
// AND compatible fontName (per the script-detection relaxation). Toggles
// (bold/italic/underline/strike/vertAlign/vanish) must match — they
// affect every character regardless of script. The fontName check
// follows the same whitespace-only relaxation as rFontsMergeableTexts:
// when one side's text is undetected (whitespace/empty), its fontName
// is "irrelevant" — upstream Okapi RunFonts.merge resolves to the
// detected side's value (RunFonts.java:267-315).
//
// Used by canBeMergedWithTexts to allow merging a text+space sequence
// where the space's rFonts ascii/hAnsi differs from the surrounding
// text (1200-1.docx canonical case).
func (rp runProps) equalTextAware(other runProps, rText, otherText string) bool {
	if rp.bold != other.bold ||
		rp.italic != other.italic ||
		rp.underline != other.underline ||
		rp.strike != other.strike ||
		rp.vertAlign != other.vertAlign ||
		rp.vanish != other.vanish {
		return false
	}
	if rp.fontName == other.fontName {
		return true
	}
	// fontName differs: allow when at least one side is undetected.
	if isUndetectedRunText(rText) || isUndetectedRunText(otherText) {
		return true
	}
	// Theme/direct cross-equivalence: when one side authors a direct
	// `ascii=X` and the other authors `asciiTheme=X` (or any other
	// theme-direct pair) for the same content category, the effective
	// fonts resolve to the same value per ECMA-376-1 §17.3.2.26
	// (CT_Fonts). fontName here is a parse-time shortcut populated
	// from `ascii` (then `hAnsi`); it does NOT consider asciiTheme.
	// Defer the strict-fontName gate to the full rFontsMergeableTexts
	// per-category check below — that's the authoritative compatibility
	// test and already handles theme/direct equivalence. Returning true
	// here lets canBeMergedWithTexts proceed to rPrChildrenMergeableTexts
	// which will either confirm or refuse the merge.
	//
	// FontThemeOverFont.docx canonical: R1 fontName="minorHAnsi" (from
	// direct ascii), R3 fontName="Times New Roman" (from direct ascii)
	// — but R3 also carries asciiTheme="minorHAnsi" which dominates per
	// ECMA-376-1 §17.3.2.26, so the effective Latin font is the same on
	// both sides and upstream Okapi merges the runs.
	return rp.fontNameThemeDirectCompatible(other)
}

// fontNameThemeDirectCompatible reports whether two runProps with
// differing fontName values are theme/direct compatible — i.e. one
// run's direct font matches the other's theme alternative for the
// same content category. Approximates upstream Okapi's per-category
// effective-value comparison (RunFonts.canContentCategoriesBeMerged,
// RunFonts.java:211-230) for the Latin/ASCII content category that
// fontName tracks. Returns true when:
//   - One side has rFonts asciiTheme equal to the other side's
//     fontName (direct ascii), OR vice versa.
//   - Both sides' rFonts effective ASCII category values agree even
//     when the literal `ascii` attribute differs.
//
// Falls back to false when no rPrChildren rFonts is available to
// reason about — the caller's per-attribute gate (rFontsMergeableTexts)
// is the authoritative check; this method only relaxes equalTextAware.
func (rp runProps) fontNameThemeDirectCompatible(other runProps) bool {
	aRFonts := rp.findRFontsXML()
	bRFonts := other.findRFontsXML()
	if aRFonts == "" || bRFonts == "" {
		return false
	}
	aAttrs, ok := parseRFontsAttrs(aRFonts)
	if !ok {
		return false
	}
	bAttrs, ok := parseRFontsAttrs(bRFonts)
	if !ok {
		return false
	}
	aMap := make(map[string]string, len(aAttrs))
	for _, a := range aAttrs {
		aMap[a.name] = a.value
	}
	bMap := make(map[string]string, len(bAttrs))
	for _, a := range bAttrs {
		bMap[a.name] = a.value
	}
	// Effective ASCII category value: asciiTheme dominates ascii.
	aEff := aMap["w:asciiTheme"]
	if aEff == "" {
		aEff = aMap["w:ascii"]
	}
	bEff := bMap["w:asciiTheme"]
	if bEff == "" {
		bEff = bMap["w:ascii"]
	}
	if aEff != "" && bEff != "" && aEff == bEff {
		return true
	}
	// Same for HIGH_ANSI (hAnsiTheme/hAnsi) — fontName falls back to
	// hAnsi when ascii is absent (see parseRunProps RunProperty rFonts).
	aHAnsiEff := aMap["w:hAnsiTheme"]
	if aHAnsiEff == "" {
		aHAnsiEff = aMap["w:hAnsi"]
	}
	bHAnsiEff := bMap["w:hAnsiTheme"]
	if bHAnsiEff == "" {
		bHAnsiEff = bMap["w:hAnsi"]
	}
	if aHAnsiEff != "" && bHAnsiEff != "" && aHAnsiEff == bHAnsiEff {
		return true
	}
	return false
}

// findRFontsXML returns the raw XML of the run's <w:rFonts> child or
// "" when absent. Helper for fontNameThemeDirectCompatible.
func (rp runProps) findRFontsXML() string {
	for k := range rp.rPrChildren {
		c := &rp.rPrChildren[k]
		if c.name == "rFonts" {
			return c.xml
		}
	}
	return ""
}

// rPrChildrenMergeableTexts is the text-aware variant of
// rPrChildrenMergeable. Same contract for non-rFonts children, but
// the rFonts compatibility check passes the run texts so the
// per-attribute gate can allow undetected-category divergence per
// upstream Okapi RunFonts.canContentCategoriesBeMerged
// (RunFonts.java:211-230). See canBeMergedWithTexts for the
// rationale.
func rPrChildrenMergeableTexts(a, b []rPrChild, aText, bText string) bool {
	if len(a) != len(b) {
		return false
	}
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
	if aFonts == nil || bFonts == nil {
		return true
	}
	return rFontsMergeableTexts(aFonts.xml, bFonts.xml, aText, bText)
}

// rFontsMergeableTexts is the text-aware variant of rFontsMergeable.
// When a shared attribute has different values across the two runs,
// the merge is allowed iff at least one side has NO text in the
// content category the attribute addresses (per ECMA-376-1
// §17.3.2.26: ascii→Latin/Basic, hAnsi→High ANSI, cs→Complex Script,
// eastAsia→East Asian).
//
// Native lacks the full ContentCategoriesDetection state machine, so
// the approximation is: a run whose text is empty or whitespace-only
// has no detected content category for ANY script — the rFonts
// attributes from the other side carry through. Mirrors upstream
// Okapi RunFonts.canContentCategoriesBeMerged (RunFonts.java:211-230)
// where `containsDetected(category)` is false on a whitespace-only
// run (whitespace is not "in" any script range).
//
// For richer runs (mixed scripts), the per-attribute byte-equality
// path applies unchanged — over-conservative compared to upstream's
// full content-category detection but never invents a merge that
// upstream wouldn't.
func rFontsMergeableTexts(aXML, bXML, aText, bText string) bool {
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
	}
	aUndetected := isUndetectedRunText(aText)
	bUndetected := isUndetectedRunText(bText)
	// Per-content-category mergeability — mirrors upstream Okapi
	// RunFonts.canContentCategoriesBeMerged (RunFonts.java:211-230).
	// For each content category C ∈ {ASCII, HIGH_ANSI, COMPLEX_SCRIPT,
	// EAST_ASIAN} the effective rFonts value is themeC when present,
	// else directC. Two runs can merge on category C when:
	//   - neither side has any attribute for C (no conflict), OR
	//   - the effective values are equal (themeC vs themeC, themeC vs
	//     directC, directC vs themeC, or directC vs directC), OR
	//   - at least one side is "undetected" (whitespace/empty text)
	//     so its attribute is moot for that category.
	// The theme/direct cross-equivalence is required by FontThemeOverFont.docx:
	// run "Hello" has `ascii=minorHAnsi`; run "World" has
	// `ascii="Times New Roman", asciiTheme=minorHAnsi`. Per
	// ECMA-376-1 §17.3.2.26 (CT_Fonts) `asciiTheme` resolves to the
	// active theme's `minorHAnsi` font, taking precedence over the
	// direct `ascii` attribute. Effective ASCII values are equal
	// (both resolve to minorHAnsi) so upstream merges the runs.
	for _, pair := range rFontsThemePairs {
		aDirect, aHasDirect := aMap[pair.direct]
		aTheme, aHasTheme := aMap[pair.theme]
		bDirect, bHasDirect := bMap[pair.direct]
		bTheme, bHasTheme := bMap[pair.theme]
		// Effective value: theme dominates direct.
		var aEff, bEff string
		var aHas, bHas bool
		switch {
		case aHasTheme:
			aEff = aTheme
			aHas = true
		case aHasDirect:
			aEff = aDirect
			aHas = true
		}
		switch {
		case bHasTheme:
			bEff = bTheme
			bHas = true
		case bHasDirect:
			bEff = bDirect
			bHas = true
		}
		if !aHas || !bHas {
			continue
		}
		if aEff == bEff {
			continue
		}
		// Effective category values differ.
		if !aUndetected && !bUndetected {
			return false
		}
	}
	// Non-font rFonts attributes (everything except ascii/hAnsi/cs/
	// eastAsia and their theme siblings + hint, which is handled
	// below). These have no theme/direct equivalence; differing values
	// are blockers unless one side is undetected.
	categoryAttrs := rFontsCategoryAttrSet()
	for name, aV := range aMap {
		if name == "w:hint" || name == "hint" {
			continue
		}
		if categoryAttrs[name] {
			continue
		}
		bV, has := bMap[name]
		if !has {
			continue
		}
		if aV == bV {
			continue
		}
		if !aUndetected && !bUndetected {
			return false
		}
	}
	// Hint compatibility: only relax the one-sided-hint blocker
	// when the SIDE THAT CARRIES THE HINT is undetected (so its
	// hint references no real script category in the run text).
	// Mirrors upstream Okapi RunFonts.canHintsBeMerged
	// (RunFonts.java:232-248): when one side has hint=X and the
	// other doesn't, the merge is allowed iff the hint-bearing
	// side does NOT contain text in the script category X. For
	// native, we approximate "doesn't contain text in any script"
	// as "whitespace-only or empty" — sufficient for the spacer-
	// run cases (1200-1.docx) without breaking Arabic-with-hint vs
	// space-no-hint adjacent fixtures (1385-whitespace-styles.docx)
	// where the hint-bearing side carries genuine CS text.
	aHasHint := hasHintAttr(aMap)
	bHasHint := hasHintAttr(bMap)
	if aHasHint != bHasHint {
		// Content-category-aware hint merging — mirrors upstream
		// Okapi RunFonts.canHintsBeMerged (RunFonts.java:232-248).
		// When one side has hint=X and the other doesn't, the merge
		// is allowed iff the hint-bearing side does NOT contain a
		// content category corresponding to X.
		// `contentCategoriesByHints` mapping (RunFonts.java:73-78):
		//   hint=eastAsia → check {EAST_ASIAN_THEME, EAST_ASIAN}
		//   hint=cs       → check {COMPLEX_SCRIPT_THEME, COMPLEX_SCRIPT}
		//   hint=default  → check {HIGH_ANSI_THEME, HIGH_ANSI}
		//
		// Two relaxations apply:
		//   - The hint-bearing side's text is undetected
		//     (whitespace/empty) — the hint references no script
		//     category in any of its text.
		//   - The hint-bearing side has NO font attribute in the
		//     hint's content-category slot (per
		//     containsContentCategoryFor at RunFonts.java:250-257).
		//     E.g. hint="eastAsia" + ascii=X but no eastAsia/
		//     eastAsiaTheme — the hint is moot because no
		//     East-Asian-category font is asserted.
		//
		// Fixture document-with-run-fonts-variations.docx: R0 has
		// `<w:rFonts ascii="Courier New" hint="eastAsia"/>` (no
		// eastAsia direct/theme attribute), R1 has
		// `<w:rFonts ascii="Courier New"/>`. The hint references
		// EAST_ASIAN but R0 has no East-Asian font slot — upstream
		// canHintsBeMerged returns true.
		hintSideMap := aMap
		hintSideUndetected := aUndetected
		if bHasHint {
			hintSideMap = bMap
			hintSideUndetected = bUndetected
		}
		if hintSideUndetected {
			return true
		}
		if !hintSideHasContentCategorySlot(hintSideMap) {
			return true
		}
		return false
	}
	return true
}

// hintSideHasContentCategorySlot reports whether the rFonts attribute
// map carries a font attribute in the content-category slot that the
// `hint` attribute references. Mirrors upstream Okapi
// RunFonts.containsContentCategoryFor (RunFonts.java:250-257) + the
// contentCategoriesByHints mapping (RunFonts.java:73-78):
//
//	hint="eastAsia" → check eastAsia / eastAsiaTheme
//	hint="cs"       → check cs / cstheme
//	hint="default"  → check hAnsi / hAnsiTheme
//
// Returns false when the hint attribute is absent (the caller should
// have gated on hasHintAttr before reaching here).
func hintSideHasContentCategorySlot(m map[string]string) bool {
	hint := m["w:hint"]
	if hint == "" {
		hint = m["hint"]
	}
	if hint == "" {
		return false
	}
	switch hint {
	case "eastAsia":
		return mapHasAny(m, "w:eastAsia", "w:eastAsiaTheme", "eastAsia", "eastAsiaTheme")
	case "cs":
		return mapHasAny(m, "w:cs", "w:cstheme", "cs", "cstheme")
	case "default":
		return mapHasAny(m, "w:hAnsi", "w:hAnsiTheme", "hAnsi", "hAnsiTheme")
	}
	// Unknown hint value — conservative: assume it's relevant.
	return true
}

// mapHasAny reports whether m contains any of the listed keys with a
// non-empty value.
func mapHasAny(m map[string]string, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != "" {
			return true
		}
	}
	return false
}

// isUndetectedRunText reports whether s carries no characters in any
// detectable script content category. Used as a native approximation
// of upstream Okapi RunFonts.containsDetected returning false for ALL
// categories. The approximation is "empty or whitespace-only" —
// whitespace (ASCII space, tab, newline, NBSP, narrow NBSP, …) is not
// in any script range per Unicode general categories and is treated
// as undetected by upstream's RunFonts.containsContentCategoryFor +
// ContentCategoriesDetection.java:134-138.
//
// Sentinel/PUA runs (.. internal markers) are not
// expected to reach this path because mergeRuns refuses to fuse them
// with regular text via the isSentinel guard.
func isUndetectedRunText(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', ' ', ' ', ' ', ' ', ' ':
			// Common whitespace forms: ASCII space/tab/newline/CR,
			// NBSP, narrow NBSP, figure space, thin space, hair space.
			continue
		}
		// Non-whitespace character → at least one script category
		// may be detected. Conservative: treat as detected.
		return false
	}
	return true
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

// rFontsCategoryAttrSet returns the set of rFonts attribute names that
// address a per-content-category font (ASCII / HIGH_ANSI / COMPLEX_SCRIPT
// / EAST_ASIAN) — covered by the theme/direct pair-aware effective-value
// comparison in rFontsMergeableTexts. Used to exclude these attributes
// from the generic per-attribute equality loop so the per-category
// comparison is the sole authority.
func rFontsCategoryAttrSet() map[string]bool {
	out := make(map[string]bool, 2*len(rFontsThemePairs))
	for _, p := range rFontsThemePairs {
		out[p.direct] = true
		out[p.theme] = true
	}
	return out
}

// mergeRPrChildrenTexts returns the merged rPrChildren of two
// mergeable runs (text-aware: when one side's text is undetected —
// empty or whitespace-only via isUndetectedRunText — shared rFonts
// attributes that differ in value resolve to the DETECTED side's
// value; mirrors upstream Okapi RunFonts.merge at RunFonts.java:267-
// 315 which prefers the detected content category's value).
//
// Non-rFonts children are taken from `a` (byte-equal to `b`'s per the
// rPrChildrenMergeableTexts contract). rFonts is the per-attribute
// intersection (or detected-side preference when one side is
// undetected). An empty rFonts intersection is dropped rather than
// emitted as a bare `<w:rFonts/>` — an attribute-less rFonts carries
// no formatting and only noises up the per-run rPr sidecar.
//
// Used by mergeRuns when canBeMergedWithTexts allowed the merge via
// the whitespace relaxation. The caller must have already gated on
// rPrChildrenMergeableTexts.
func mergeRPrChildrenTexts(a, b []rPrChild, aText, bText string) []rPrChild {
	_, aFonts := splitRFonts(a)
	_, bFonts := splitRFonts(b)
	aUndetected := isUndetectedRunText(aText)
	bUndetected := isUndetectedRunText(bText)
	merged := mergeRFontsXMLTextAware(aFonts, bFonts, aUndetected, bUndetected)
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

// mergeRFontsXMLTextAware merges two rFonts entries with awareness of
// each side's content-category detection (approximated as "undetected
// when text is whitespace-only"). The merge rules:
//
//  1. Both sides detected → per-attribute intersection (only shared
//     attrs with equal values are kept). Same as mergeRFontsXML.
//  2. A detected, B undetected → keep A's attributes verbatim
//     (B has no "opinion" on font choice).
//  3. A undetected, B detected → keep B's attributes verbatim.
//  4. Both undetected → per-attribute intersection (no detection
//     signal either way).
//
// Per upstream Okapi RunFonts.merge (RunFonts.java:267-315 +
// mergeContentCategories at 299-315): the merged value for a content
// category is the DETECTED side's value when only one is detected;
// when both are detected with equal values, that value is kept;
// when both are detected with different values, the merge would have
// been refused by canBeMerged upstream (so we don't reach this path
// unless the byte-equality / undetected-relaxation gate passed).
func mergeRFontsXMLTextAware(a, b *rPrChild, aUndetected, bUndetected bool) rPrChild {
	if a == nil && b == nil {
		return rPrChild{name: "rFonts"}
	}
	if a == nil {
		return *b
	}
	if b == nil {
		return *a
	}
	if aUndetected && !bUndetected {
		// A's attributes carry no content-category signal — defer to
		// B's choices for the merged result.
		return *b
	}
	if !aUndetected && bUndetected {
		return *a
	}
	// Both detected (or both undetected): per-content-category union
	// with theme/direct cross-equivalence. Mirrors upstream Okapi
	// RunFonts.merge + mergeContentCategories (RunFonts.java:267-315)
	// which iterates EVERY ContentCategory and emits a value per
	// (theme, direct) attribute when either run carries one, even
	// when the per-attribute byte-equal intersection would drop it.
	//
	// Concretely, FontThemeOverFont.docx merges:
	//   A: ascii=minorHAnsi, eastAsia=minorHAnsi, hAnsi=minorHAnsi
	//   B: ascii=Times, asciiTheme=minorHAnsi, eastAsia=minorHAnsi,
	//      hAnsi=minorHAnsi
	// Effective ASCII category resolves to minorHAnsi on both sides
	// (A via ascii, B via asciiTheme dominating direct). Upstream's
	// merge keeps ascii=minorHAnsi (from A's detected side) AND
	// asciiTheme=minorHAnsi (from B's detected side) as separate
	// attributes — both attribute slots are populated whenever
	// either source carried the corresponding slot.
	return mergeRFontsXMLCategoryUnion(a, b)
}

// mergeRFontsXMLCategoryUnion is the per-content-category union of two
// rFonts entries with theme/direct cross-equivalence. For each
// (direct, theme) pair plus the non-category attributes, the merged
// result keeps an attribute when the contributing runs agree on the
// effective category value. Mirrors upstream Okapi RunFonts.merge
// + RunFonts.mergeContentCategories (RunFonts.java:267-315).
//
// Algorithm:
//  1. For each (direct, theme) pair in rFontsThemePairs:
//     - Compute the effective category value: theme if present, else
//       direct.
//     - If both sides have an effective value AND they agree, emit
//       every attribute that any side carried for the pair (direct,
//       theme).
//     - If only one side has an effective value, emit that side's
//       attributes for the pair.
//     - If neither side has the pair, skip.
//  2. For non-category attributes (hint, …), take the byte-equal
//     intersection — upstream's behavior for the hint slot.
//
// Attribute order: walk a's attribute order first, then append any
// theme/non-theme slot from b not already emitted. Preserves the
// "a's order" stability used by the byte-equality intersection path.
func mergeRFontsXMLCategoryUnion(a, b *rPrChild) rPrChild {
	aAttrs, _ := parseRFontsAttrs(a.xml)
	bAttrs, _ := parseRFontsAttrs(b.xml)
	aMap := make(map[string]string, len(aAttrs))
	for _, x := range aAttrs {
		aMap[x.name] = x.value
	}
	bMap := make(map[string]string, len(bAttrs))
	for _, x := range bAttrs {
		bMap[x.name] = x.value
	}
	// Per-content-category emit decision. For each (direct, theme)
	// pair, we honour the theme/direct cross-equivalence rule of
	// upstream Okapi RunFonts.mergeContentCategories (RunFonts.java:
	// 299-315) under a conservative native approximation:
	//
	//   - Both sides assert a category value (via direct or theme):
	//     emit the corresponding attribute when the effective values
	//     match. When A's direct value equals B's theme value (or
	//     vice versa), the cross-effective values are equivalent; we
	//     emit BOTH source attributes (direct from A, theme from B)
	//     to preserve each side's content-category-detection signal —
	//     mirroring upstream's mergeContentCategories iterating every
	//     ContentCategory.
	//   - Only one side asserts the category: drop the attribute
	//     (intersection semantics). Upstream's mergeContentCategories
	//     would emit the asserted value iff the asserting side has the
	//     content category detected; with no script-detection signal
	//     native conservatively drops it. This preserves 1411-mergable
	//     -runs.docx's intersection where R2 lacks hAnsi.
	emit := make(map[string]string, 8)
	for _, pair := range rFontsThemePairs {
		aDirect, aHasDirect := aMap[pair.direct]
		aTheme, aHasTheme := aMap[pair.theme]
		bDirect, bHasDirect := bMap[pair.direct]
		bTheme, bHasTheme := bMap[pair.theme]
		// Effective category value (theme dominates direct).
		var aEff, bEff string
		aHas := aHasDirect || aHasTheme
		bHas := bHasDirect || bHasTheme
		switch {
		case aHasTheme:
			aEff = aTheme
		case aHasDirect:
			aEff = aDirect
		}
		switch {
		case bHasTheme:
			bEff = bTheme
		case bHasDirect:
			bEff = bDirect
		}
		if !aHas || !bHas {
			// Only one side has this category. Intersect: emit only
			// when the direct attribute is byte-equal on both sides
			// (no asymmetric carry-through — matches the prior
			// mergeRFontsXML behaviour for the side-with-attr/side
			// -without case).
			continue
		}
		if aEff != bEff {
			// Effective values disagree — would have been refused by
			// canBeMerged. Drop the pair (intersection fallback).
			continue
		}
		// Effective values agree. Emit every attribute that any side
		// carried for the pair. When A authored direct and B authored
		// theme (or vice versa) we emit both — each side's
		// content-category-detection signal is preserved on the wire.
		// Prefer "theme-less direct" for the direct slot: that's the
		// detected-direct value per upstream's mergeContentCategories
		// (RunFonts.java:299-315) which keeps the value of the side
		// whose detection actually used the direct attribute.
		anyDirect := aHasDirect || bHasDirect
		anyTheme := aHasTheme || bHasTheme
		if anyDirect {
			switch {
			case aHasDirect && !aHasTheme:
				emit[pair.direct] = aDirect
			case bHasDirect && !bHasTheme:
				emit[pair.direct] = bDirect
			case aHasDirect:
				emit[pair.direct] = aDirect
			case bHasDirect:
				emit[pair.direct] = bDirect
			}
		}
		if anyTheme {
			switch {
			case aHasTheme:
				emit[pair.theme] = aTheme
			case bHasTheme:
				emit[pair.theme] = bTheme
			}
		}
	}
	categoryAttrs := rFontsCategoryAttrSet()
	// Hint attribute: prefer the side that has it (A wins ties).
	// Mirrors upstream Okapi RunFonts.mergeContentCategories HINT
	// branch (RunFonts.java:300-304):
	//   if (null == fonts.get(HINT)) return runFonts.fonts.get(HINT);
	//   else return fonts.get(HINT);
	// The gate (rFontsMergeableTexts) already verified the hint can
	// be merged via canHintsBeMerged content-category-aware check, so
	// we simply preserve the hint-bearing side's value here.
	if v, ok := aMap["w:hint"]; ok {
		emit["w:hint"] = v
	} else if v, ok := bMap["w:hint"]; ok {
		emit["w:hint"] = v
	}
	if v, ok := aMap["hint"]; ok {
		emit["hint"] = v
	} else if v, ok := bMap["hint"]; ok {
		emit["hint"] = v
	}
	// Non-category, non-hint attributes: byte-equal intersection.
	for _, x := range aAttrs {
		if categoryAttrs[x.name] {
			continue
		}
		if x.name == "w:hint" || x.name == "hint" {
			continue
		}
		if v, ok := bMap[x.name]; ok && v == x.value {
			emit[x.name] = x.value
		}
	}
	// Build XML preserving a's attribute order, then append b-only
	// category attributes in b's order. Matches mergeRFontsAcrossRuns'
	// "a's order wins" stability.
	prefix := extractRFontsElemNameFromXML(a.xml)
	if prefix == "" {
		prefix = extractRFontsElemNameFromXML(b.xml)
	}
	if prefix == "" {
		prefix = "w:rFonts"
	}
	var b1 strings.Builder
	b1.WriteByte('<')
	b1.WriteString(prefix)
	seen := make(map[string]bool, len(emit))
	for _, x := range aAttrs {
		v, ok := emit[x.name]
		if !ok || seen[x.name] {
			continue
		}
		b1.WriteByte(' ')
		b1.WriteString(x.name)
		b1.WriteString(`="`)
		b1.WriteString(escapeAttrVal(v))
		b1.WriteByte('"')
		seen[x.name] = true
	}
	for _, x := range bAttrs {
		v, ok := emit[x.name]
		if !ok || seen[x.name] {
			continue
		}
		b1.WriteByte(' ')
		b1.WriteString(x.name)
		b1.WriteString(`="`)
		b1.WriteString(escapeAttrVal(v))
		b1.WriteByte('"')
		seen[x.name] = true
	}
	b1.WriteString("/>")
	return rPrChild{name: "rFonts", xml: b1.String()}
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

// boldOnXML returns the rPr child serialisation to emit for an
// ON-state bold toggle: the source's preserved explicit-on form when
// captured (e.g. `<w:b w:val="1"/>`), otherwise the canonical bare
// `<w:b/>`. ECMA-376-1 §17.3.2.1 (CT_OnOff <w:b>) treats the bare
// element and val="1"/"true"/"on" as equivalent ON states.
func boldOnXML(rp runProps) string {
	if rp.boldXML != "" {
		return rp.boldXML
	}
	return "<w:b/>"
}

// italicOnXML mirrors boldOnXML for `<w:i>`. ECMA-376-1 §17.3.2.13.
func italicOnXML(rp runProps) string {
	if rp.italicXML != "" {
		return rp.italicXML
	}
	return "<w:i/>"
}

// vanishOnXML mirrors boldOnXML for `<w:vanish>`. ECMA-376-1 §17.3.2.42.
func vanishOnXML(rp runProps) string {
	if rp.vanishXML != "" {
		return rp.vanishXML
	}
	return "<w:vanish/>"
}

// appendOpeningRuns emits PcOpen runs for this run's formatting.
func (rp runProps) appendOpeningRuns(b *runBuilder, idCounter *int) {
	emit := func(typ, subType, data string) {
		*idCounter++
		b.AddPcOpen(idStr(*idCounter), typ, subType, data, "", "", true, true, true)
	}
	if rp.bold {
		// Use the captured explicit-on form (e.g.
		// `<w:b w:val="1"/>`) when the source authored one;
		// otherwise emit the canonical bare `<w:b/>`. ECMA-376-1
		// §17.3.2.1 (CT_OnOff <w:b>).
		emit(TypeBold, SubTypeBold, boldOnXML(rp))
	}
	if rp.italic {
		// Same treatment as bold per ECMA-376-1 §17.3.2.13.
		emit(TypeItalic, SubTypeItalic, italicOnXML(rp))
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
					// Preserve the explicit-on form (e.g.
					// `<w:b w:val="1"/>`) so the writer can re-emit
					// the source authoring form. The bare `<w:b/>`
					// is the canonical default and stays in
					// boldXML="" so callers fall through to the
					// fixed `<w:b/>` literal. Only capture when the
					// element carried any attributes (rsid* etc are
					// already pre-stripped by stripFieldRPrSkippables
					// in the field-markup capture path; the
					// translatable rPr path here sees a bare element
					// without any rsids on a `<w:b/>` toggle in
					// well-formed WPML). Per ECMA-376-1 §17.3.2.1
					// (CT_OnOff <w:b>), val="1" / "true" / "on" are
					// equivalent ON states; upstream Okapi preserves
					// the source RunProperty's exact QName +
					// attributes (RunProperties.java:497-540).
					if props.bold && len(t.Attr) > 0 {
						raw, err := serializeRPrChildElement(d, t)
						if err != nil {
							return props, err
						}
						props.boldXML = raw
					} else if err := skipElement(d); err != nil {
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
					// Mirror the bold path: capture the explicit-on
					// form (e.g. `<w:i w:val="1"/>`) so the writer
					// can re-emit the source's exact serialisation.
					// ECMA-376-1 §17.3.2.13 (CT_OnOff <w:i>).
					if props.italic && len(t.Attr) > 0 {
						raw, err := serializeRPrChildElement(d, t)
						if err != nil {
							return props, err
						}
						props.italicXML = raw
					} else if err := skipElement(d); err != nil {
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
					if err := skipElement(d); err != nil {
						return props, err
					}
				} else {
					// Clearing form (`<w:u w:val="none"/>` or
					// `<w:u w:val="none" w:color="..."/>`) per ECMA-376-1
					// §17.3.2.40 (CT_Underline) — explicit no-underline,
					// the override that suppresses an inherited underline
					// from the resolved style chain. Preserve verbatim in
					// rPrChildren so it survives the per-run rPr sidecar.
					// Mirrors upstream Okapi RunProperties.minified()
					// preCombined.contains check (RunProperties.java
					// :497-540) which keeps the clearing form when the
					// style chain authors `<w:u>` by name. Fixture
					// 992.docx footer1.xml is the canonical case: every
					// hyperlink-wrapped text run carries
					// `<w:u w:val="none" w:color="808080"/>` to suppress
					// the inherited Hyperlink-style underline; without
					// preservation the round-trip emits the inherited
					// underline on those runs.
					raw, err := serializeRPrChildElement(d, t)
					if err != nil {
						return props, err
					}
					props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
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
				// Toggle property per ECMA-376-1 §17.3.2.42 (<w:vanish>).
				// A bare element turns hide-text ON; an explicit
				// `val="0"` / `"false"` / `"off"` is the clearing form.
				// The bare-on path normalises into runProps.vanish so
				// downstream consumers (allHidden, writer toggle emit)
				// can read it efficiently. The clearing form is preserved
				// verbatim in rPrChildren so it survives into the per-run
				// rPr sidecar — upstream Okapi's RunProperties.minified()
				// preserves a clearing-value toggle when the inherited
				// style chain carries that property by name
				// (RunProperties.java:497-540, the
				// `!preCombined.contains(p.getName())` condition).
				// Mirrors the local == "b" / local == "i" clearing-form
				// branches above. Without this, lang.docx's
				// `editform`-styled space run loses its
				// `<w:vanish w:val="0"/>` clearing override on round-trip
				// (the source uses it to suppress a `vanish` toggle that
				// would otherwise be inherited at the document layer; the
				// minifyRPrChildren default-strip path drops it because
				// the immediate Normal style chain has no vanish).
				off := hasAttrVal(t, "val", "0") || hasAttrVal(t, "val", "false") || hasAttrVal(t, "val", "off")
				props.vanish = !off
				props.vanishExplicit = true
				if off {
					raw, err := serializeRPrChildElement(d, t)
					if err != nil {
						return props, err
					}
					props.rPrChildren = append(props.rPrChildren, rPrChild{name: local, xml: raw})
				} else {
					// Preserve the explicit-on form (e.g.
					// `<w:vanish w:val="on"/>` /
					// `<w:vanish w:val="1"/>`) so the writer can re-emit
					// the source authoring form. The bare `<w:vanish/>`
					// is the canonical default and stays in
					// vanishXML="" so callers fall through to the
					// fixed `<w:vanish/>` literal. Mirrors the bold/
					// italic explicit-on capture above. Per ECMA-376-1
					// §17.3.2.42 (CT_OnOff <w:vanish>), val="1" /
					// "true" / "on" are equivalent ON states; upstream
					// Okapi preserves the source RunProperty's exact
					// QName + attributes (RunProperties.java:497-540).
					// HiddenTablesApachePoi.docx is the canonical
					// fixture: hidden table runs author
					// `<w:vanish w:val="on"/>`; the WSO post-pass lifts
					// vanish into a synthesised pStyle and the bridge
					// preserves the explicit-on form there.
					if len(t.Attr) > 0 {
						raw, err := serializeRPrChildElement(d, t)
						if err != nil {
							return props, err
						}
						props.vanishXML = raw
					} else if err := skipElement(d); err != nil {
						return props, err
					}
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
			// Defer the strip of default-valued entries that depend on
			// the rStyle chain. wml.go runs minifyRPrChildren again
			// per-run with the merged paraChain ∪ rStyleChain — entries
			// that need the rStyle context to be preserved (e.g.
			// `<w:u w:val="none"/>` against an rStyle-supplied
			// `<w:u w:val="single"/>`) survive this parse-time pass.
			// 834.docx footnotes is the canonical fixture (rStyle="a6"
			// / Hyperlink). Strict mode (paired-toggle bCs/iCs
			// preservation, rtl-with-sibling preservation) still runs.
			props.rPrChildren = minifyRPrChildrenDeferred(props.rPrChildren, styleChainNames)
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
	return minifyRPrChildrenWithMode(children, styleChainNames, false)
}

// minifyRPrChildrenDeferred behaves like minifyRPrChildren but DEFERS
// the "strip default-valued entries whose name is absent from the
// chain" rule. Used at parse time inside parseRunProps where the
// supplied chain only covers the paragraph's pStyle — the rStyle
// chain is unknown until the wml.go run-loop applies subtractProps +
// late minify. Without deferral, a run that authors
// `<w:u w:val="none"/>` to clear an inherited Hyperlink-style
// underline gets stripped at parse time (paragraph chain has no `u`)
// and the late minify with the merged chain (which DOES include `u`
// from the rStyle) has no entry left to preserve.
//
// 834.docx footnotes runs (rStyle="a6" / Hyperlink) are the canonical
// fixture: nested SDT runs carry `<w:u w:val="none"/>` to clear the
// inherited Hyperlink underline; without deferral the merged
// "śďţ 2" run loses its clearing form.
//
// Strict mode (paired-toggle bCs/iCs preservation, rtl-with-sibling
// preservation) still runs — those rules don't depend on rStyle
// chain context.
func minifyRPrChildrenDeferred(children []rPrChild, styleChainNames map[string]bool) []rPrChild {
	return minifyRPrChildrenWithMode(children, styleChainNames, true)
}

// minifyRPrChildrenWithMode is the internal worker. When deferDefault
// is true, default-valued entries that would be stripped because the
// chain doesn't carry the name are PRESERVED — the caller must run
// minifyRPrChildren again with the augmented chain.
func minifyRPrChildrenWithMode(children []rPrChild, styleChainNames map[string]bool, deferDefault bool) []rPrChild {
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
	// Empirical Okapi behavior: <w:rtl w:val="0"/> is preserved in run
	// rPr whenever the rPr also carries other formatting that survives
	// minified() — it's only stripped when the rPr would otherwise be
	// emptied entirely (the reordered-zip.docx case where rtl=0 is the
	// SOLE child of <w:rPr> and the upstream writer drops the empty
	// rPr after stripping).
	//
	// Reading RunProperties.minified() (RunProperties.java:497-540) at
	// face value, the `<w:rtl w:val="0"/>` toggle should land in the
	// drop branch (WpmlToggleRunProperty && !getToggleValue() &&
	// !preCombined.contains("rtl")) for any rPr whose paragraph-style
	// chain doesn't author rtl. But empirically, running the bridge
	// against 830-2.docx KEEPS the entry on every text-bearing run AND
	// on the empty placeholder run. Two equally-shaped runs in 830-2
	// vs reordered-zip — same source rPr `<w:rPr><w:rtl w:val="0"/>
	// </w:rPr>` — diverge: reordered-zip's rPr is emptied (because rtl
	// is the only child) while 830-2's is preserved (because the run
	// either has no text body or has text alongside other rPr
	// children). The most parsimonious model is "Okapi only collapses
	// rtl=0 when the post-strip rPr would itself collapse" — possibly
	// realised through some downstream re-emit path we haven't located
	// in the source. Mirror that effective behaviour with a sibling
	// check: keep rtl=0 when the rPr has any non-default-valued
	// sibling, drop it otherwise so reordered-zip.docx still emits
	// `<w:r><w:t>` without an empty `<w:rPr/>`.
	hasRtlPreservingSibling := func() bool {
		for _, c := range children {
			if c.name == "rtl" {
				continue
			}
			if !isDefaultValuedRPrChild(c) {
				return true
			}
		}
		return false
	}()
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
			if c.name == "rtl" && hasRtlPreservingSibling {
				out = append(out, c)
				continue
			}
			if styleChainNames == nil || !styleChainNames[c.name] {
				// Defer the strip ONLY when the caller provided a
				// chain that doesn't carry `c.name`. With a nil chain
				// (no style context — caller disabled style optimisation
				// OR a unit test that does not load styles.xml), keep
				// the legacy "always strip default-valued entries"
				// behaviour so style-context-free callers stay byte-
				// equivalent (TestParseRunProps_StripsDefaultValuedRtl
				// and the reordered-zip.docx fixture path rely on this).
				if deferDefault && styleChainNames != nil {
					// Caller (parseRunProps) only sees the paragraph
					// chain; the rStyle chain may add `c.name` and
					// flip this strip into a "preserve as clearing
					// override". Keep verbatim and let the late
					// minify in wml.go (with paraChain ∪ rStyleChain)
					// decide. Fixture 834.docx footnotes: rStyle="a6"
					// (Hyperlink) chain has `<w:u w:val="single"/>`,
					// so a per-run `<w:u w:val="none"/>` clearing
					// form must round-trip; without deferral the
					// parse-time minify (paragraph chain only) drops
					// it and the late minify has nothing left.
					out = append(out, c)
					continue
				}
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

// stripExplicitOffVanish returns children with any
// `<w:vanish w:val="0"/>` / `<w:vanish w:val="false"/>` /
// `<w:vanish w:val="off"/>` entry removed. Used by the wml.go
// run-loop AFTER folding the rStyle chain into the chain-name set,
// so the strip only fires when the merged chain doesn't author
// vanish by name. Vanish is intentionally excluded from
// `wpmlToggleNames` so the parse-time minify pass (which only sees
// the paragraph's chain) doesn't strip the clearing form
// prematurely. Fixture lang.docx is the canonical case: the editform
// rStyle chain authors `<w:vanish/>`, so the per-run
// `<w:vanish w:val="0"/>` clearing override must round-trip; without
// the deferred-strip split, the parse-time minify (which only sees
// the empty pStyle chain) drops it before wml.go can decide.
func stripExplicitOffVanish(children []rPrChild) []rPrChild {
	out := children[:0]
	for _, c := range children {
		if c.name == "vanish" {
			val, ok := parseRPrChildVal(c.xml)
			if ok && (val == "0" || val == "false" || val == "off") {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// stripChainAbsentSzCs returns children with `<w:szCs .../>` removed
// when the merged style chain (paragraph style ∪ rStyle, with
// docDefaults folded in) does NOT author szCs by name. Mirrors the
// `else { v = true }` branch of upstream Okapi's
// RunParser.canBeSkipped (RunParser.java:236-250): when
// `preCombinedRunProperties.contains(name)` is false the property is
// unconditionally skippable, which feeds the no-CS-text strip at
// RunParser.java:226-228 (RUN_PROPERTY_COMPLEX_SCRIPT_FONT_SIZE
// added to skippableProperties when
// !runFonts.containsDetectedComplexScriptContentCategories — the
// upstream gate is text-driven, the caller is responsible for that
// guard).
//
// Per ECMA-376-1 §17.3.2.39 (CT_HpsMeasure szCs — complex-script
// font size), szCs is the paired complex-script side of `<w:sz>`
// (§17.3.2.38). When neither the chain nor the run text is
// complex-script bearing the property is a no-op duplicate of
// `<w:sz>` and Okapi drops it at parse time.
//
// The OTHER half of canBeSkipped — chain HAS szCs and the run's
// szCs xml byte-equals the chain's szCs xml — is already handled by
// the chain-XML-match strip in wml.go (the loop that consults
// `effectiveRPrChildXML`). This helper closes the missing branch.
//
// Caller contract: invoke only when the run's text is
// non-complex-script (containsComplexScriptText == false) AND the
// merged chainNames map does NOT contain "szCs". The two-fold gate
// keeps the strip semantically equivalent to upstream and avoids
// dropping a legitimate value override for paragraphs whose chain
// inherits a different szCs (947-non-cs.docx counterexample:
// docDefaults declares `<w:szCs val="24"/>`, so chainNames["szCs"]
// is true — the strip is gated off and the run's `<w:szCs val="28"/>`
// override survives into the synth common rPr).
//
// MissingPara.docx canonical case: no style or docDefaults declares
// szCs by name; every translatable paragraph's runs carry
// `<w:rPr><w:szCs val="…"/></w:rPr>` (often with rFonts/sz too) on
// ASCII text. Without this strip native lifts szCs into a synth
// `Normal2` paragraph style, diverging from upstream which strips it
// at parse time and never lifts it.
func stripChainAbsentSzCs(children []rPrChild) []rPrChild {
	out := children[:0]
	for _, c := range children {
		if c.name == "szCs" {
			continue
		}
		out = append(out, c)
	}
	return out
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
